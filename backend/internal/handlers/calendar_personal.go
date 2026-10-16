package handlers

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"corporate.admsr.ru/backend/internal/auth"
	"corporate.admsr.ru/backend/internal/httpx"
)

// Запись на мероприятия и личный календарь (V16, Q-03). Раньше это жило в localStorage
// браузера (`events-rsvp:v1`, `portal-calendar-local:v1`) и не было видно нигде, кроме
// устройства. Личность — только из серверной сессии: чужие записи не читаются и не меняются.

const (
	calendarTitleMaxRunes    = 200
	calendarLocationMaxRunes = 200
	calendarMaxPerUser       = 500
)

var calendarTimeRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

func calendarFailDB(w http.ResponseWriter, op string, err error) {
	log.Printf("calendar personal: %s: %v", op, err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table
		httpx.Fail(w, http.StatusServiceUnavailable, "Запись на мероприятия и личный календарь ещё не включены на сервере: нужна миграция V16__calendar_personal.sql")
		return
	}
	httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
}

// ── /api/calendar_entries.php ────────────────────────────────────────────────

// CalendarEntries: GET — личные записи и встречи текущего пользователя;
// POST {action:"create", source, dateKey, title, timeStart?, timeEnd?, location?, color?};
// POST {action:"update", id, …те же поля} и {action:"delete", id} — только свои.
type CalendarEntries struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
}

// calendarColorRe — цвет события: '#rrggbb'. Пусто — цвет по типу (встреча / личное).
var calendarColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (h *CalendarEntries) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
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
	switch strVal(body["action"]) {
	case "create":
		h.create(w, r, cur, body)
	case "update":
		h.update(w, r, cur, body)
	case "delete":
		h.remove(w, r, cur, body)
	default:
		httpx.Fail(w, http.StatusBadRequest, "Неизвестное действие")
	}
}

// calendarEntry — поля записи; пустые необязательные — "".
type calendarEntry struct {
	ID                           int64
	Source, DateKey, Title       string
	TimeStart, TimeEnd, Location string
	Color                        string
}

func (h *CalendarEntries) list(w http.ResponseWriter, r *http.Request, cur *auth.User) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, source, to_char(date_key, 'YYYY-MM-DD'), title,
		       COALESCE(time_start, ''), COALESCE(time_end, ''), COALESCE(location, ''), COALESCE(color, '')
		FROM public.calendar_entries WHERE user_id = $1
		ORDER BY date_key, COALESCE(time_start, ''), id`, cur.ID)
	if err != nil {
		calendarFailDB(w, "entries list", err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var e calendarEntry
		if rows.Scan(&e.ID, &e.Source, &e.DateKey, &e.Title, &e.TimeStart, &e.TimeEnd, &e.Location, &e.Color) == nil {
			out = append(out, calendarEntryJSON(e))
		}
	}
	httpx.OK(w, map[string]any{"entries": out}, "")
}

// calendarEntryJSON отдаёт пустые необязательные поля как отсутствующие (фронт ждёт undefined).
func calendarEntryJSON(e calendarEntry) map[string]any {
	m := map[string]any{"id": e.ID, "source": e.Source, "dateKey": e.DateKey, "title": e.Title}
	for k, v := range map[string]string{"timeStart": e.TimeStart, "timeEnd": e.TimeEnd, "location": e.Location, "color": e.Color} {
		if v != "" {
			m[k] = v
		}
	}
	return m
}

// calendarEntryInput проверяет тело create / update; возвращает текст ошибки или "".
func calendarEntryInput(body map[string]any) (calendarEntry, string) {
	var e calendarEntry
	e.Source = strVal(body["source"])
	if e.Source != "meeting" && e.Source != "personal" {
		return e, "Тип записи: встреча или личное"
	}
	e.Title = strings.TrimSpace(strVal(body["title"]))
	if e.Title == "" {
		return e, "Укажите название"
	}
	if utf8.RuneCountInString(e.Title) > calendarTitleMaxRunes {
		return e, "Название длиннее 200 символов"
	}
	e.DateKey = strVal(body["dateKey"])
	if _, err := time.Parse("2006-01-02", e.DateKey); err != nil {
		return e, "Некорректная дата"
	}
	e.TimeStart = strings.TrimSpace(strVal(body["timeStart"]))
	e.TimeEnd = strings.TrimSpace(strVal(body["timeEnd"]))
	for _, t := range []string{e.TimeStart, e.TimeEnd} {
		if t != "" && !calendarTimeRe.MatchString(t) {
			return e, "Некорректное время"
		}
	}
	e.Location = strings.TrimSpace(strVal(body["location"]))
	if utf8.RuneCountInString(e.Location) > calendarLocationMaxRunes {
		return e, "Место длиннее 200 символов"
	}
	e.Color = strings.ToLower(strings.TrimSpace(strVal(body["color"])))
	if e.Color != "" && !calendarColorRe.MatchString(e.Color) {
		return e, "Некорректный цвет"
	}
	return e, ""
}

func (h *CalendarEntries) create(w http.ResponseWriter, r *http.Request, cur *auth.User, body map[string]any) {
	e, problem := calendarEntryInput(body)
	if problem != "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, problem)
		return
	}
	var n int64
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM public.calendar_entries WHERE user_id = $1`, cur.ID).Scan(&n); err != nil {
		calendarFailDB(w, "entries count", err)
		return
	}
	if n >= calendarMaxPerUser {
		httpx.Fail(w, http.StatusUnprocessableEntity, "Слишком много записей в календаре — удалите ненужные")
		return
	}
	if err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO public.calendar_entries (user_id, source, date_key, title, time_start, time_end, location, color)
		VALUES ($1, $2, $3::date, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''))
		RETURNING id`, cur.ID, e.Source, e.DateKey, e.Title, e.TimeStart, e.TimeEnd, e.Location, e.Color).Scan(&e.ID); err != nil {
		calendarFailDB(w, "entries create", err)
		return
	}
	httpx.OK(w, map[string]any{"entry": calendarEntryJSON(e)}, "Запись добавлена")
}

func (h *CalendarEntries) update(w http.ResponseWriter, r *http.Request, cur *auth.User, body map[string]any) {
	id := int64Of(body["id"])
	if id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный id")
		return
	}
	e, problem := calendarEntryInput(body)
	if problem != "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, problem)
		return
	}
	e.ID = id
	// user_id в условии: чужую запись изменить нельзя, а о её существовании не сообщаем.
	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE public.calendar_entries
		SET source = $3, date_key = $4::date, title = $5, time_start = NULLIF($6, ''),
		    time_end = NULLIF($7, ''), location = NULLIF($8, ''), color = NULLIF($9, '')
		WHERE id = $1 AND user_id = $2`,
		id, cur.ID, e.Source, e.DateKey, e.Title, e.TimeStart, e.TimeEnd, e.Location, e.Color)
	if err != nil {
		calendarFailDB(w, "entries update", err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	// Перенесли на другой день — прежнее напоминание больше не про это событие.
	if _, err := h.Pool.Exec(r.Context(), `
		DELETE FROM public.notifications
		WHERE kind = $1 AND calendar_entry_id = $2 AND event_date <> $3::date`,
		notifyKindEventReminder, id, e.DateKey); err != nil {
		log.Printf("calendar personal: drop stale reminders for entry %d: %v", id, err)
	}
	httpx.OK(w, map[string]any{"entry": calendarEntryJSON(e)}, "Запись изменена")
}

func (h *CalendarEntries) remove(w http.ResponseWriter, r *http.Request, cur *auth.User, body map[string]any) {
	id := int64Of(body["id"])
	if id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный id")
		return
	}
	// user_id в условии: чужую запись удалить нельзя, а о её существовании не сообщаем.
	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM public.calendar_entries WHERE id = $1 AND user_id = $2`, id, cur.ID)
	if err != nil {
		calendarFailDB(w, "entries delete", err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, http.StatusNotFound, "Запись не найдена")
		return
	}
	httpx.OK(w, nil, "Запись удалена")
}
