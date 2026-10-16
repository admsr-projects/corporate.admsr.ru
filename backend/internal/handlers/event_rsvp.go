package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"corporate.admsr.ru/backend/internal/auth"
	"corporate.admsr.ru/backend/internal/httpx"
)

// ── /api/event_rsvp.php ──────────────────────────────────────────────────────

// EventRSVP — запись на мероприятия (V16), на конкретное время (V23, ADR-056).
//
//	GET            — свои записи: {eventIds, items: [{eventId, slotId}]}
//	GET ?eventId=  — кто записался, по временам (только организаторы, ADR-054)
//	POST {eventId, slotId?, joined} — записаться на время / перезаписаться на другое /
//	               отменить запись. slotId не передан — первое время мероприятия.
//
// Сотрудник записан на одно время мероприятия (PK event_rsvps (event_id, user_id)).
// Лимит времени проверяется под блокировкой строки времени: два одновременных
// «Записаться» на последнее место не пройдут оба.
type EventRSVP struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
}

func (h *EventRSVP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}
	if r.Method == http.MethodGet && r.URL.Query().Has("eventId") {
		h.participants(w, r)
		return
	}
	cur, ok := requireUser(w, r, h.Auth)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		h.list(w, r, cur)
		return
	}
	var body map[string]any
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный JSON")
		return
	}
	eventID := int64Of(body["eventId"])
	if eventID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "Не передан eventId")
		return
	}
	joined, _ := body["joined"].(bool)
	var slotID int64
	if joined {
		var ok bool
		if slotID, ok = h.join(w, r, eventID, int64Of(body["slotId"]), cur.ID); !ok {
			return
		}
	} else if _, err := h.Pool.Exec(r.Context(),
		`DELETE FROM public.event_rsvps WHERE event_id = $1 AND user_id = $2`, eventID, cur.ID); err != nil {
		calendarFailDB(w, "rsvp remove", err)
		return
	}
	resp, err := h.counts(r.Context(), eventID)
	if err != nil {
		calendarFailDB(w, "rsvp count", err)
		return
	}
	resp["eventId"] = eventID
	resp["joined"] = joined
	if joined {
		resp["slotId"] = slotID
	}
	httpx.OK(w, resp, "")
}

// counts — число записавшихся всего и по каждому времени (для обновления карточки).
func (h *EventRSVP) counts(ctx context.Context, eventID int64) (map[string]any, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT s.id, (SELECT COUNT(*) FROM public.event_rsvps r WHERE r.slot_id = s.id)
		FROM public.event_slots s WHERE s.event_id = $1
		ORDER BY s.position, s.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	slots := []map[string]any{}
	var total int64
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		total += n
		slots = append(slots, map[string]any{"id": id, "participantsCount": n})
	}
	return map[string]any{"participantsCount": total, "slots": slots}, rows.Err()
}

// join — записать на время (или перезаписать с другого) с проверкой лимита в
// транзакции. Возвращает id времени; false — ответ уже записан.
func (h *EventRSVP) join(w http.ResponseWriter, r *http.Request, eventID, slotID, userID int64) (int64, bool) {
	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		calendarFailDB(w, "rsvp begin", err)
		return 0, false
	}
	defer tx.Rollback(ctx)

	// Время этого мероприятия (не передано — первое), под блокировкой.
	var limit *int32
	var timeStart, timeEnd *string
	q := `SELECT id, max_participants, time_start, time_end FROM public.event_slots
		WHERE event_id = $1 AND ($2::bigint = 0 OR id = $2::bigint)
		ORDER BY position, id LIMIT 1 FOR UPDATE`
	if err := tx.QueryRow(ctx, q, eventID, slotID).Scan(&slotID, &limit, &timeStart, &timeEnd); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "Мероприятие или время не найдено")
			return 0, false
		}
		if isUndefinedTable(err) {
			httpx.Fail(w, http.StatusServiceUnavailable, "Мероприятия не обновлены на сервере: нужна миграция V23__event_slots.sql")
			return 0, false
		}
		calendarFailDB(w, "rsvp slot", err)
		return 0, false
	}

	var currentSlot *int64
	err = tx.QueryRow(ctx,
		`SELECT slot_id FROM public.event_rsvps WHERE event_id = $1 AND user_id = $2`, eventID, userID).Scan(&currentSlot)
	already := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		calendarFailDB(w, "rsvp current", err)
		return 0, false
	}
	if already && currentSlot != nil && *currentSlot == slotID {
		return slotID, true // уже записан на это время
	}

	if limit != nil {
		var count int64
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM public.event_rsvps WHERE slot_id = $1`, slotID).Scan(&count); err != nil {
			calendarFailDB(w, "rsvp count", err)
			return 0, false
		}
		if count >= int64(*limit) {
			label := "это время"
			if timeStart != nil {
				label = *timeStart
				if timeEnd != nil {
					label += "–" + *timeEnd
				}
			}
			httpx.Fail(w, http.StatusConflict, "На "+label+" мест больше нет: записалось "+
				strconv.FormatInt(count, 10)+" из "+strconv.Itoa(int(*limit)))
			return 0, false
		}
	}

	if already {
		// Перезапись на другое время: прежнее место освобождается.
		_, err = tx.Exec(ctx, `
			UPDATE public.event_rsvps SET slot_id = $3, created_at = now()
			WHERE event_id = $1 AND user_id = $2`, eventID, userID, slotID)
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO public.event_rsvps (event_id, user_id, slot_id) VALUES ($1, $2, $3)
			ON CONFLICT (event_id, user_id) DO UPDATE SET slot_id = EXCLUDED.slot_id`, eventID, userID, slotID)
	}
	if err != nil {
		calendarFailDB(w, "rsvp add", err)
		return 0, false
	}
	if err := tx.Commit(ctx); err != nil {
		calendarFailDB(w, "rsvp commit", err)
		return 0, false
	}
	return slotID, true
}

// participants — кто записался, с временем записи; порядок — по временам, затем по
// моменту записи. Только организаторам.
func (h *EventRSVP) participants(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSection(w, r, h.Pool, h.Auth, "events"); !ok {
		return
	}
	eventID, _ := strconv.ParseInt(r.URL.Query().Get("eventId"), 10, 64)
	if eventID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "Не передан eventId")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT r.user_id,
		       COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.surname, u.firstname, u.lastname)), ''), 'Сотрудник'),
		       COALESCE(u.avatar_url, ''), r.created_at, r.slot_id
		FROM public.event_rsvps r
		LEFT JOIN public.user_info u ON u.id = r.user_id
		LEFT JOIN public.event_slots s ON s.id = r.slot_id
		WHERE r.event_id = $1
		ORDER BY s.position NULLS FIRST, s.id, r.created_at, r.user_id`, eventID)
	if err != nil {
		calendarFailDB(w, "rsvp participants", err)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var uid int64
		var name, avatar string
		var at time.Time
		var slot *int64
		if err := rows.Scan(&uid, &name, &avatar, &at, &slot); err != nil {
			calendarFailDB(w, "rsvp participants scan", err)
			return
		}
		list = append(list, map[string]any{"id": uid, "name": name, "avatar_url": avatar, "joinedAt": at, "slotId": slot})
	}
	if err := rows.Err(); err != nil {
		calendarFailDB(w, "rsvp participants rows", err)
		return
	}
	httpx.OK(w, list, "OK")
}

func (h *EventRSVP) list(w http.ResponseWriter, r *http.Request, cur *auth.User) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT event_id, slot_id FROM public.event_rsvps WHERE user_id = $1 ORDER BY event_id`, cur.ID)
	if err != nil {
		calendarFailDB(w, "rsvp list", err)
		return
	}
	defer rows.Close()
	ids := []int64{}
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var slot *int64
		if rows.Scan(&id, &slot) == nil {
			ids = append(ids, id)
			items = append(items, map[string]any{"eventId": id, "slotId": slot})
		}
	}
	httpx.OK(w, map[string]any{"eventIds": ids, "items": items}, "")
}
