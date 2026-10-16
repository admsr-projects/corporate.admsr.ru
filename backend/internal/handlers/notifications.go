package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"corporate.admsr.ru/backend/internal/auth"
	"corporate.admsr.ru/backend/internal/httpx"
)

// Notifications — колокольчик в шапке (/api/notifications.php). Сотрудник видит
// и отмечает прочитанными только свои уведомления; личность — только из сессии.
//
// Опроса по таймеру нет намеренно: каждый авторизованный запрос продлевает
// last_activity, и опрос раз в минуту отменил бы выход по бездействию.
// Фронт запрашивает уведомления при загрузке, переходах и открытии колокольчика.
type Notifications struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
}

const (
	notifyKindWallPost         = "wall_post"
	notifyKindBirthdayGreeting = "birthday_greeting"
	notifyKindCommentReply     = "comment_reply"  // ответ на ваш комментарий к новости (V18)
	notifyKindEventReminder    = "event_reminder" // завтра личное событие или мероприятие, куда записались (V19)
	notifyKindDevblog          = "devblog"        // опубликован девблог — всем активным сотрудникам (V20)

	notificationsPageDefault = 20
	notificationsPageMax     = 50
	notificationExcerptRunes = 140
)

// notifyWallPost — уведомление владельцу стены о новой записи. Ошибка не
// отменяет саму запись: запись уже опубликована, без уведомления она переживёт.
func notifyWallPost(ctx context.Context, pool *pgxpool.Pool, ownerID, authorID, postID int64, kind string) {
	if ownerID == authorID {
		return
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.notifications (user_id, actor_id, kind, post_id)
		VALUES ($1, $2, $3, $4)`, ownerID, authorID, kind, postID); err != nil {
		log.Printf("notifications: insert %s for user %d: %v", kind, ownerID, err)
	}
}

func notificationsFailDB(w http.ResponseWriter, op string, err error) {
	log.Printf("notifications: %s: %v", op, err)
	if wallTableMissing(err) {
		httpx.Fail(w, http.StatusServiceUnavailable, "Уведомления ещё не включены на сервере: нужна миграция V17__birthday_greetings_notifications.sql")
		return
	}
	if isUndefinedColumn(err) {
		httpx.Fail(w, http.StatusServiceUnavailable, "Уведомления не обновлены на сервере: нужны миграции V18–V23 (V18__news_comments.sql, V19__calendar_colors_reminders.sql, V20__devblog.sql, V22__events_schedule_limit.sql, V23__event_slots.sql)")
		return
	}
	httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
}

func (h *Notifications) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cur, ok := requireUser(w, r, h.Auth)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.list(w, r, cur)
	case http.MethodPost:
		var body map[string]any
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Некорректный JSON")
			return
		}
		switch strVal(body["action"]) {
		case "read":
			h.markRead(w, r, cur, body)
		case "read_all":
			h.markAllRead(w, r, cur)
		default:
			httpx.Fail(w, http.StatusBadRequest, "Неизвестное действие")
		}
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Notifications) unreadCount(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := h.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM public.notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// sqlNotificationsList: $1 — получатель, $2 — limit.
const sqlNotificationsList = `
		SELECT n.id, n.kind, n.post_id, n.comment_id, COALESCE(nc.news_id, n.news_id), n.created_at, n.read_at, n.actor_id,
			COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.firstname, u.surname)), ''), 'Сотрудник'),
			COALESCE(u.avatar_url, ''),
			COALESCE(p.content, nc.content, ce.title, ev.title, nw.title, ''),
			n.calendar_entry_id, n.event_id, to_char(n.event_date, 'YYYY-MM-DD'),
			COALESCE(ce.time_start, es.time_start, ''), COALESCE(ce.location, ev.location, ''), COALESCE(ce.color, ''),
			COALESCE(ce.time_end, es.time_end, '')
		FROM public.notifications n
		LEFT JOIN public.user_info u ON u.id = n.actor_id
		LEFT JOIN public.wall_posts p ON p.id = n.post_id
		LEFT JOIN public.news_comments nc ON nc.id = n.comment_id
		LEFT JOIN public.calendar_entries ce ON ce.id = n.calendar_entry_id
		LEFT JOIN public.events ev ON ev.id = n.event_id
		-- время мероприятия — то, на которое записан получатель (V23)
		LEFT JOIN public.event_rsvps er ON er.event_id = n.event_id AND er.user_id = n.user_id
		LEFT JOIN public.event_slots es ON es.id = er.slot_id
		LEFT JOIN public.news nw ON nw.id = n.news_id
		WHERE n.user_id = $1
		ORDER BY n.created_at DESC, n.id DESC
		LIMIT $2`

// Напоминания накануне (V19). Планировщика нет: напоминания создаются, когда
// сотрудник открывает портал (запрос уведомлений), — то есть «с утра накануне»,
// как только он зашёл. 'event_reminder' в SQL — литералом: ON CONFLICT должен
// совпасть с условием частичного индекса notifications_event_reminder_uidx.

// sqlDropStaleReminders: $1 — сотрудник. Событие перенесли на другой день или
// запись на мероприятие отменили — напоминание больше не про то событие.
const sqlDropStaleReminders = `
	DELETE FROM public.notifications n
	WHERE n.user_id = $1 AND n.kind = 'event_reminder' AND (
		(n.calendar_entry_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.calendar_entries ce
			WHERE ce.id = n.calendar_entry_id AND ce.date_key = n.event_date))
		OR (n.event_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.event_rsvps r JOIN public.events e ON e.id = r.event_id
			WHERE r.user_id = n.user_id AND r.event_id = n.event_id AND e.date = n.event_date)))`

// sqlRemindEntries: $1 — сотрудник, $2 — завтра (YYYY-MM-DD). Личные события и встречи.
const sqlRemindEntries = `
	INSERT INTO public.notifications (user_id, kind, calendar_entry_id, event_date)
	SELECT ce.user_id, 'event_reminder', ce.id, ce.date_key
	FROM public.calendar_entries ce
	WHERE ce.user_id = $1 AND ce.date_key = $2::date
	ON CONFLICT (user_id, COALESCE(calendar_entry_id, 0), COALESCE(event_id, 0), event_date)
		WHERE kind = 'event_reminder' DO NOTHING`

// sqlRemindEvents: $1 — сотрудник, $2 — завтра. Мероприятия, куда он записался.
const sqlRemindEvents = `
	INSERT INTO public.notifications (user_id, kind, event_id, event_date)
	SELECT r.user_id, 'event_reminder', r.event_id, e.date
	FROM public.event_rsvps r JOIN public.events e ON e.id = r.event_id
	WHERE r.user_id = $1 AND e.date = $2::date
	ON CONFLICT (user_id, COALESCE(calendar_entry_id, 0), COALESCE(event_id, 0), event_date)
		WHERE kind = 'event_reminder' DO NOTHING`

// generateReminders — до списка и счётчика: напоминания о завтрашних событиях.
// Сбой не роняет колокольчик — остальные уведомления важнее.
func (h *Notifications) generateReminders(ctx context.Context, userID int64) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := h.Pool.Exec(ctx, sqlDropStaleReminders, userID); err != nil {
		log.Printf("notifications: drop stale reminders for user %d: %v", userID, err)
		return
	}
	for name, sql := range map[string]string{"entries": sqlRemindEntries, "events": sqlRemindEvents} {
		if _, err := h.Pool.Exec(ctx, sql, userID, tomorrow); err != nil {
			log.Printf("notifications: remind %s for user %d: %v", name, userID, err)
		}
	}
}

func (h *Notifications) list(w http.ResponseWriter, r *http.Request, cur *auth.User) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = notificationsPageDefault
	}
	if limit > notificationsPageMax {
		limit = notificationsPageMax
	}
	h.generateReminders(r.Context(), cur.ID)
	unread, err := h.unreadCount(r.Context(), cur.ID)
	if err != nil {
		notificationsFailDB(w, "count", err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), sqlNotificationsList, cur.ID, limit)
	if err != nil {
		notificationsFailDB(w, "list", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id                       int64
			actorID                  *int64
			postID, commentID        *int64
			newsID, entryID, eventID *int64
			eventDate                *string
			kind, name               string
			avatar, content          string
			timeStart, loc, color    string
			timeEnd                  string
			createdAt                time.Time
			readAt                   *time.Time
		)
		if err := rows.Scan(&id, &kind, &postID, &commentID, &newsID, &createdAt, &readAt, &actorID, &name, &avatar, &content,
			&entryID, &eventID, &eventDate, &timeStart, &loc, &color, &timeEnd); err != nil {
			notificationsFailDB(w, "scan", err)
			return
		}
		item := map[string]any{
			"id":        id,
			"kind":      kind,
			"postId":    postID,
			"commentId": commentID,
			"newsId":    newsID,
			"createdAt": createdAt.Format(time.RFC3339),
			"read":      readAt != nil,
			"actor":     nil,
			"excerpt":   truncateRunes(content, notificationExcerptRunes),
			"reminder":  nil,
		}
		if actorID != nil {
			item["actor"] = map[string]any{"id": *actorID, "name": name, "avatar_url": avatar}
		}
		if kind == notifyKindEventReminder {
			item["reminder"] = map[string]any{
				"title":           content,
				"date":            eventDate,
				"timeStart":       timeStart,
				"timeEnd":         timeEnd,
				"location":        loc,
				"color":           color,
				"calendarEntryId": entryID,
				"eventId":         eventID,
			}
		}
		items = append(items, item)
	}
	httpx.OK(w, map[string]any{"unread": unread, "items": items}, "OK")
}

func (h *Notifications) markRead(w http.ResponseWriter, r *http.Request, cur *auth.User, body map[string]any) {
	raw, _ := body["ids"].([]any)
	ids := make([]int64, 0, len(raw))
	for _, v := range raw {
		if id := int64Of(v); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "Не переданы уведомления")
		return
	}
	// user_id в условии: чужие уведомления отметить нельзя, даже зная их id.
	if _, err := h.Pool.Exec(r.Context(), `
		UPDATE public.notifications SET read_at = now()
		WHERE user_id = $1 AND id = ANY($2) AND read_at IS NULL`, cur.ID, ids); err != nil {
		notificationsFailDB(w, "read", err)
		return
	}
	h.respondUnread(w, r, cur)
}

func (h *Notifications) markAllRead(w http.ResponseWriter, r *http.Request, cur *auth.User) {
	if _, err := h.Pool.Exec(r.Context(), `
		UPDATE public.notifications SET read_at = now()
		WHERE user_id = $1 AND read_at IS NULL`, cur.ID); err != nil {
		notificationsFailDB(w, "read all", err)
		return
	}
	h.respondUnread(w, r, cur)
}

func (h *Notifications) respondUnread(w http.ResponseWriter, r *http.Request, cur *auth.User) {
	unread, err := h.unreadCount(r.Context(), cur.ID)
	if err != nil {
		notificationsFailDB(w, "count", err)
		return
	}
	httpx.OK(w, map[string]any{"unread": unread}, "OK")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
