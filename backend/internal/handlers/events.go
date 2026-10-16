package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"corporate.admsr.ru/backend/internal/auth"
	"corporate.admsr.ru/backend/internal/httpx"
)

const defaultEventImage = "/favicon.svg"

type Events struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
}

type eventRow struct {
	ID          int64
	Title       string
	Description *string
	Badge       *string
	Date        *time.Time
	Image       string
	ImageFull   string
	AlbumID     *int64
	AlbumName   *string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	// Location — место (V22, одно на все времена).
	Location *string
	// SlotsJSON — времена мероприятия (V23, ADR-056), см. eventSlotsJSON.
	SlotsJSON string
	// Participants — сколько записалось на все времена (event_rsvps).
	Participants int64
}

const (
	eventLocationMax     = 300
	eventParticipantsMax = 100000
)

// eventCols — колонки для eventRow.dest(); таблица — под псевдонимом e.
const eventCols = `e.id, e.title, e.description, e.badge, e.date, e.image, e.image_full, e.album_id,
	e.created_at, e.updated_at, e.location, ` + eventSlotsJSON + `,
	(SELECT COUNT(*) FROM public.event_rsvps r WHERE r.event_id = e.id)`

func (row *eventRow) dest() []any {
	return []any{
		&row.ID, &row.Title, &row.Description, &row.Badge, &row.Date, &row.Image, &row.ImageFull, &row.AlbumID,
		&row.CreatedAt, &row.UpdatedAt, &row.Location, &row.SlotsJSON, &row.Participants,
	}
}

// eventsFailDB — без V22/V23 (нет места или таблицы времён) — понятный 503 вместо «ошибки БД».
func eventsFailDB(w http.ResponseWriter, err error, fallback string) {
	if isUndefinedColumn(err) || isUndefinedTable(err) {
		httpx.Fail(w, http.StatusServiceUnavailable, "Мероприятия не обновлены на сервере: нужны миграции V22__events_schedule_limit.sql и V23__event_slots.sql")
		return
	}
	httpx.Fail(w, http.StatusInternalServerError, fallback)
}

var eventTimeRe = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

// normalizeEventTime — «9:00» → «09:00»; пусто — nil; false — не время.
func normalizeEventTime(v any) (*string, bool) {
	s := strings.TrimSpace(strVal(v))
	if s == "" {
		return nil, true
	}
	m := eventTimeRe.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	h, _ := strconv.Atoi(m[1])
	t := fmt.Sprintf("%02d:%s", h, m[2])
	return &t, true
}

func (h *Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPost:
		h.post(w, r)
	case http.MethodPut:
		h.put(w, r)
	case http.MethodDelete:
		h.del(w, r)
	default:
		httpx.MethodNotAllowed(w)
	}
}

func (h *Events) get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if idStr := q.Get("id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(w, http.StatusBadRequest, "Некорректный id")
			return
		}
		row, err := h.fetchOne(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpx.Fail(w, http.StatusNotFound, "Мероприятие не найдено")
				return
			}
			eventsFailDB(w, err, "Ошибка подключения к БД")
			return
		}
		httpx.OK(w, fmtEvent(row), "OK")
		return
	}

	if q.Has("limit") || q.Has("cursor") {
		page, err := h.fetchCursorPage(r.Context(), q)
		if err != nil {
			if err.Error() == "bad_cursor" {
				httpx.Fail(w, http.StatusBadRequest, "Некорректный cursor")
				return
			}
			eventsFailDB(w, err, "Ошибка подключения к БД")
			return
		}
		httpx.OK(w, page, "OK")
		return
	}

	items, err := h.fetchList(r.Context(), q)
	if err != nil {
		eventsFailDB(w, err, "Ошибка подключения к БД")
		return
	}
	httpx.OK(w, items, "OK")
}

func (h *Events) post(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSection(w, r, h.Pool, h.Auth, "events"); !ok {
		return
	}

	var d map[string]any
	if err := httpx.DecodeJSON(r, &d); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный JSON")
		return
	}
	if strings.TrimSpace(strVal(d["title"])) == "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, "Поле «title» обязательно")
		return
	}
	if strings.TrimSpace(strVal(d["date"])) == "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, "Поле «date» обязательно")
		return
	}

	location, msg := parseEventLocation(d, nil)
	if msg != "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, msg)
		return
	}
	slots, _, msg := parseEventSlots(d)
	if msg != "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if len(slots) == 0 {
		slots = []eventSlotInput{{}}
	}

	albumID, err := h.parseAlbumID(r.Context(), w, d, true)
	if err != nil {
		return
	}

	image := defaultEventImage
	if ip := strings.TrimSpace(strVal(d["image"])); ip != "" {
		image = ip
	}
	imageFull := defaultEventImage
	if ip := strings.TrimSpace(strVal(d["image_full"])); ip != "" {
		imageFull = ip
	} else if ip := strings.TrimSpace(strVal(d["image"])); ip != "" {
		imageFull = ip
	}

	var desc, badge *string
	if v, ok := d["description"]; ok {
		s := strings.TrimSpace(strVal(v))
		desc = &s
	}
	if v, ok := d["badge"]; ok {
		s := strings.TrimSpace(strVal(v))
		badge = &s
	}

	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer tx.Rollback(ctx)

	var newID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.events (title, description, badge, date, image, image_full, album_id, location)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		strings.TrimSpace(strVal(d["title"])),
		desc,
		badge,
		strings.TrimSpace(strVal(d["date"])),
		image,
		imageFull,
		albumID,
		location,
	).Scan(&newID)
	if err == nil {
		err = syncEventSlots(ctx, tx, newID, slots)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		if isUndefinedColumn(err) || strings.Contains(err.Error(), "event_slots") {
			eventsFailDB(w, err, "Ошибка БД: "+err.Error())
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "album_id") {
			httpx.Fail(w, http.StatusInternalServerError, "В БД нет колонки events.album_id. Примените миграцию db/migration/V7__events_gallery_album.sql")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка БД: "+err.Error())
		return
	}

	row, err := h.fetchOne(r.Context(), newID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	httpx.Created(w, fmtEvent(row), "Мероприятие создано")
}

func (h *Events) put(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSection(w, r, h.Pool, h.Auth, "events"); !ok {
		return
	}
	id, ok := queryID(r)
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "Укажите ?id=...")
		return
	}

	var d map[string]any
	if err := httpx.DecodeJSON(r, &d); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный JSON")
		return
	}
	if strings.TrimSpace(strVal(d["title"])) == "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, "Поле «title» обязательно")
		return
	}
	if strings.TrimSpace(strVal(d["date"])) == "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, "Поле «date» обязательно")
		return
	}

	var curImage, curImageFull string
	var curAlbumID *int64
	var curLocation *string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT image, image_full, album_id, location
		FROM public.events WHERE id = $1`, id).
		Scan(&curImage, &curImageFull, &curAlbumID, &curLocation)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "Мероприятие не найдено")
			return
		}
		eventsFailDB(w, err, "Ошибка подключения к БД")
		return
	}
	location, msg := parseEventLocation(d, curLocation)
	if msg != "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, msg)
		return
	}
	// Лимит времени можно опустить ниже числа записавшихся: уже записанные остаются,
	// новых не пускаем. Время, которого нет в списке, удаляется вместе с записями на него.
	slots, slotsPresent, msg := parseEventSlots(d)
	if msg != "" {
		httpx.Fail(w, http.StatusUnprocessableEntity, msg)
		return
	}

	var albumID *int64
	if _, has := d["album_id"]; has {
		albumID, err = h.parseAlbumID(r.Context(), w, d, true)
		if err != nil {
			return
		}
	} else {
		albumID = curAlbumID
	}

	image := curImage
	if ip := strings.TrimSpace(strVal(d["image"])); ip != "" {
		image = ip
	}
	imageFull := curImageFull
	if ip := strings.TrimSpace(strVal(d["image_full"])); ip != "" {
		imageFull = ip
	}

	var desc, badge *string
	if v, ok := d["description"]; ok {
		s := strings.TrimSpace(strVal(v))
		desc = &s
	}
	if v, ok := d["badge"]; ok {
		s := strings.TrimSpace(strVal(v))
		badge = &s
	}

	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE public.events
		SET title = $1, description = $2, badge = $3, date = $4,
		    image = $5, image_full = $6, album_id = $7, location = $8, updated_at = NOW()
		WHERE id = $9`,
		strings.TrimSpace(strVal(d["title"])),
		desc,
		badge,
		strings.TrimSpace(strVal(d["date"])),
		image,
		imageFull,
		albumID,
		location,
		id,
	)
	if err == nil && slotsPresent {
		err = syncEventSlots(ctx, tx, id, slots)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		if isUndefinedColumn(err) || strings.Contains(err.Error(), "event_slots") {
			eventsFailDB(w, err, "Ошибка БД: "+err.Error())
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "album_id") {
			httpx.Fail(w, http.StatusInternalServerError, "В БД нет колонки events.album_id. Примените миграцию db/migration/V7__events_gallery_album.sql")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка БД: "+err.Error())
		return
	}

	row, err := h.fetchOne(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	httpx.OK(w, fmtEvent(row), "Мероприятие обновлено")
}

func (h *Events) del(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSection(w, r, h.Pool, h.Auth, "events"); !ok {
		return
	}
	id, ok := queryID(r)
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "Укажите ?id=...")
		return
	}
	var exists int64
	err := h.Pool.QueryRow(r.Context(), `SELECT id FROM public.events WHERE id = $1`, id).Scan(&exists)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "Мероприятие не найдено")
		return
	}
	_, _ = h.Pool.Exec(r.Context(), `DELETE FROM public.events WHERE id = $1`, id)
	httpx.OK(w, nil, "Мероприятие удалено")
}

func (h *Events) fetchOne(ctx context.Context, id int64) (eventRow, error) {
	var row eventRow
	err := h.Pool.QueryRow(ctx, `SELECT `+eventCols+` FROM public.events e WHERE e.id = $1`, id).Scan(row.dest()...)
	if err != nil {
		return row, err
	}
	if row.AlbumID != nil && *row.AlbumID > 0 {
		var name *string
		err := h.Pool.QueryRow(ctx, `SELECT name FROM public.gallery WHERE id = $1`, *row.AlbumID).Scan(&name)
		if err == nil {
			row.AlbumName = name
		}
	}
	return row, nil
}

func (h *Events) fetchList(ctx context.Context, q interface{ Get(string) string }) ([]map[string]any, error) {
	cond, args, _ := h.buildFilters(q, false)
	allowed := map[string]bool{"date": true, "title": true, "created_at": true, "badge": true}
	order := q.Get("order")
	if !allowed[order] {
		order = "date"
	}
	dir := "ASC"
	if strings.EqualFold(q.Get("dir"), "desc") {
		dir = "DESC"
	}

	sql := "SELECT " + eventCols + " FROM public.events e"
	if len(cond) > 0 {
		sql += " WHERE " + strings.Join(cond, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY %s %s", order, dir)

	rows, err := h.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventRows(rows)
}

func (h *Events) fetchCursorPage(ctx context.Context, q interface{ Get(string) string }) (map[string]any, error) {
	limit := 12
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 12
	}
	if limit > 48 {
		limit = 48
	}

	cond, args, argN := h.buildFilters(q, true)

	if cursorRaw := strings.TrimSpace(q.Get("cursor")); cursorRaw != "" {
		cur, ok := decodeDateIDCursor(cursorRaw)
		if !ok {
			return nil, errors.New("bad_cursor")
		}
		cond = append(cond, fmt.Sprintf("(date < $%d OR (date = $%d AND id < $%d))", argN, argN, argN+1))
		args = append(args, cur.Date, cur.ID)
		argN += 2
	}

	fetchLimit := limit + 1
	sql := "SELECT " + eventCols + " FROM public.events e"
	if len(cond) > 0 {
		sql += " WHERE " + strings.Join(cond, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY date DESC, id DESC LIMIT $%d", argN)
	args = append(args, fetchLimit)

	rows, err := h.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items, err := scanEventRows(rows)
	if err != nil {
		return nil, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor any
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		dateStr, _ := last["date"].(string)
		id, _ := last["id"].(int64)
		if id == 0 {
			if f, ok := last["id"].(float64); ok {
				id = int64(f)
			}
		}
		nextCursor = encodeDateIDCursor(dateStr, id)
	}

	return map[string]any{
		"items":      items,
		"nextCursor": nextCursor,
		"hasMore":    hasMore,
	}, nil
}

func (h *Events) buildFilters(q interface{ Get(string) string }, cursorMode bool) ([]string, []any, int) {
	cond := []string{}
	args := []any{}
	argN := 1

	if s := strings.TrimSpace(q.Get("search")); s != "" {
		cond = append(cond, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", argN, argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	badgeParam := strings.TrimSpace(q.Get("badge"))
	if badgeParam != "" && !(cursorMode && badgeParam == "_all") {
		badges := []string{}
		for _, b := range strings.Split(badgeParam, ",") {
			if t := strings.TrimSpace(b); t != "" {
				badges = append(badges, t)
			}
		}
		if len(badges) > 0 {
			ph := []string{}
			for _, b := range badges {
				ph = append(ph, fmt.Sprintf("$%d", argN))
				args = append(args, b)
				argN++
			}
			cond = append(cond, "badge IN ("+strings.Join(ph, ",")+")")
		}
	}

	if df := strings.TrimSpace(q.Get("date_from")); df != "" {
		cond = append(cond, fmt.Sprintf("date >= $%d", argN))
		args = append(args, df)
		argN++
	}
	if dt := strings.TrimSpace(q.Get("date_to")); dt != "" {
		cond = append(cond, fmt.Sprintf("date <= $%d", argN))
		args = append(args, dt)
		argN++
	}

	return cond, args, argN
}

func (h *Events) parseAlbumID(ctx context.Context, w http.ResponseWriter, d map[string]any, allowNull bool) (*int64, error) {
	if _, ok := d["album_id"]; !ok {
		return nil, nil
	}
	v := d["album_id"]
	if v == nil || v == false || strings.TrimSpace(strVal(v)) == "" {
		if allowNull {
			return nil, nil
		}
		return nil, nil
	}
	id, err := strconv.ParseInt(strVal(v), 10, 64)
	if err != nil || id <= 0 {
		return nil, nil
	}
	var exists int64
	err = h.Pool.QueryRow(ctx, `SELECT id FROM public.gallery WHERE id = $1`, id).Scan(&exists)
	if err != nil {
		httpx.Fail(w, http.StatusUnprocessableEntity, "Альбом не найден")
		return nil, err
	}
	return &id, nil
}

type eventScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanEventRows(rows eventScanner) ([]map[string]any, error) {
	var out []map[string]any
	for rows.Next() {
		var row eventRow
		if err := rows.Scan(row.dest()...); err != nil {
			return nil, err
		}
		out = append(out, fmtEvent(row))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

func fmtEvent(r eventRow) map[string]any {
	desc := ""
	if r.Description != nil {
		desc = *r.Description
	}
	var badge any
	if r.Badge != nil {
		badge = *r.Badge
	}
	var albumID any
	if r.AlbumID != nil {
		albumID = *r.AlbumID
	}
	return map[string]any{
		"id":          r.ID,
		"title":       r.Title,
		"description": desc,
		"badge":       badge,
		"date":        formatDate(r.Date),
		"image":       r.Image,
		"image_full":  r.ImageFull,
		"album_id":    albumID,
		"album_name":  r.AlbumName,
		"created_at":  formatTime(r.CreatedAt),
		"updated_at":  formatTime(r.UpdatedAt),
		// V22/V23: место — одно на мероприятие; времена — со своими лимитами и числом записавшихся.
		"location":           derefStr(r.Location),
		"slots":              json.RawMessage(r.SlotsJSON),
		"participants_count": r.Participants,
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
