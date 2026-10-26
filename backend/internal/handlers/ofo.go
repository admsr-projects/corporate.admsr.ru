package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"corporate.admsr.ru/backend/internal/auth"
	"corporate.admsr.ru/backend/internal/httpx"
)

type OFO struct {
	Pool *pgxpool.Pool
	Auth *auth.Service
}

func (h *OFO) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}
	// Оргструктура — внутренние данные портала, анонимам не отдаём (SEC-001).
	if _, ok := requireUser(w, r, h.Auth); !ok {
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, title, parent, type, sort_order
		FROM public.ofo
		ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Query error",
		})
		return
	}
	defer rows.Close()

	var data []map[string]any
	for rows.Next() {
		var id, parent, sortOrder int64
		var title, typ string
		if err := rows.Scan(&id, &title, &parent, &typ, &sortOrder); err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"message": "Query error",
			})
			return
		}
		data = append(data, map[string]any{
			"id":         id,
			"title":      title,
			"parent":     parent,
			"type":       typ,
			"sort_order": sortOrder,
		})
	}
	if data == nil {
		data = []map[string]any{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
}

func (h *OFO) Seats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.MethodNotAllowed(w)
		return
	}
	// Оргструктура — внутренние данные портала, анонимам не отдаём (SEC-001).
	if _, ok := requireUser(w, r, h.Auth); !ok {
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, title, ofo, insurance, rating
		FROM public.ofo_seats
		ORDER BY id ASC`)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Query error",
		})
		return
	}
	defer rows.Close()

	var data []map[string]any
	for rows.Next() {
		var id int64
		var title, ofo, insurance, rating string
		if err := rows.Scan(&id, &title, &ofo, &insurance, &rating); err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"message": "Query error",
			})
			return
		}
		data = append(data, map[string]any{
			"id":        id,
			"title":     title,
			"ofo":       ofo,
			"insurance": insurance,
			"rating":    rating,
		})
	}
	if data == nil {
		data = []map[string]any{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
}

// Tree — GET: дерево ОФО; POST: добавить подразделение (админ).
func (h *OFO) Tree(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.treeList(w, r)
	case http.MethodPost:
		h.unitAdd(w, r)
	default:
		httpx.MethodNotAllowed(w)
	}
}

const maxUnitNameLen = 300

// unitAdd добавляет подразделение. Тело: { name, category_id, parent_id? }.
// Без parent_id — корневое подразделение категории; иначе категория берётся у родителя.
func (h *OFO) unitAdd(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r, h.Auth); !ok {
		return
	}
	var body struct {
		Name       string `json:"name"`
		CategoryID int64  `json:"category_id"`
		ParentID   *int64 `json:"parent_id"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный JSON")
		return
	}
	name := strings.Join(strings.Fields(body.Name), " ")
	if name == "" {
		httpx.Fail(w, http.StatusBadRequest, "Введите название подразделения")
		return
	}
	if utf8.RuneCountInString(name) > maxUnitNameLen {
		httpx.Fail(w, http.StatusBadRequest, "Название подразделения слишком длинное")
		return
	}

	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer tx.Rollback(ctx)

	// Как и у должностей: сериализуем, чтобы параллельные запросы не взяли один id.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ofo_unit'))`); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}

	categoryID := body.CategoryID
	var level int64
	var familyNumber *int64
	if body.ParentID != nil {
		var parentLevel int64
		err := tx.QueryRow(ctx, `
			SELECT category_id, level, family_number FROM public.ofo_unit WHERE id = $1`, *body.ParentID,
		).Scan(&categoryID, &parentLevel, &familyNumber)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "Родительское подразделение не найдено")
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
		level = parentLevel + 1
	} else {
		var catExists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM public.ofo_category WHERE id = $1)`, categoryID,
		).Scan(&catExists); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
		if !catExists {
			httpx.Fail(w, http.StatusNotFound, "Категория не найдена")
			return
		}
		// Уровень корня — как у существующих корней категории.
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MIN(level), 0) FROM public.ofo_unit
			WHERE category_id = $1 AND parent_id IS NULL`, categoryID).Scan(&level); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
	}

	var dup bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM public.ofo_unit
			WHERE category_id = $1 AND parent_id IS NOT DISTINCT FROM $2::bigint AND lower(btrim(name)) = lower($3)
		)`, categoryID, body.ParentID, name).Scan(&dup); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	if dup {
		httpx.Fail(w, http.StatusConflict, "Подразделение с таким названием уже есть на этом уровне")
		return
	}

	var seq *string
	if err := tx.QueryRow(ctx,
		`SELECT pg_get_serial_sequence('public.ofo_unit', 'id')`).Scan(&seq); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	const nextUnitNumber = `(SELECT COALESCE(MAX(unit_number), 0) + 1 FROM public.ofo_unit)`
	const nextSort = `(SELECT COALESCE(MAX(sort_order), 0) + 1 FROM public.ofo_unit WHERE category_id = $1 AND parent_id IS NOT DISTINCT FROM $2::bigint)`
	var id, unitNumber, sortOrder int64
	if seq != nil {
		err = tx.QueryRow(ctx, `
			INSERT INTO public.ofo_unit (name, category_id, parent_id, level, unit_number, family_number, sort_order)
			VALUES ($3, $1, $2, $4, `+nextUnitNumber+`, $5, `+nextSort+`)
			RETURNING id, unit_number, sort_order`,
			categoryID, body.ParentID, name, level, familyNumber).Scan(&id, &unitNumber, &sortOrder)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO public.ofo_unit (id, name, category_id, parent_id, level, unit_number, family_number, sort_order)
			VALUES ((SELECT COALESCE(MAX(id), 0) + 1 FROM public.ofo_unit), $3, $1, $2, $4, `+nextUnitNumber+`, $5, `+nextSort+`)
			RETURNING id, unit_number, sort_order`,
			categoryID, body.ParentID, name, level, familyNumber).Scan(&id, &unitNumber, &sortOrder)
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Не удалось добавить подразделение")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Не удалось добавить подразделение")
		return
	}
	httpx.Created(w, map[string]any{
		"id":            id,
		"name":          name,
		"category_id":   categoryID,
		"parent_id":     body.ParentID,
		"level":         level,
		"unit_number":   unitNumber,
		"family_number": familyNumber,
		"sort_order":    sortOrder,
	}, "Подразделение добавлено")
}

func (h *OFO) treeList(w http.ResponseWriter, r *http.Request) {
	// Оргструктура — внутренние данные портала, анонимам не отдаём (SEC-001).
	if _, ok := requireUser(w, r, h.Auth); !ok {
		return
	}
	ctx := r.Context()

	catRows, err := h.Pool.Query(ctx, `
		SELECT id, name, sort_order FROM public.ofo_category ORDER BY sort_order, id`)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer catRows.Close()

	categories := []map[string]any{}
	for catRows.Next() {
		var id, sortOrder int64
		var name string
		if err := catRows.Scan(&id, &name, &sortOrder); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
		categories = append(categories, map[string]any{
			"id":         id,
			"name":       name,
			"sort_order": sortOrder,
		})
	}

	unitRows, err := h.Pool.Query(ctx, `
		SELECT id, name, category_id, parent_id, level, unit_number, family_number, sort_order
		FROM public.ofo_unit
		ORDER BY category_id, level, name`)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer unitRows.Close()

	posCounts := map[int64]int{}
	posRows, err := h.Pool.Query(ctx, `
		SELECT unit_number, COUNT(*)::int FROM public.ofo_unit_position GROUP BY unit_number`)
	if err == nil {
		for posRows.Next() {
			var unitNumber int64
			var count int
			if err := posRows.Scan(&unitNumber, &count); err == nil {
				posCounts[unitNumber] = count
			}
		}
		posRows.Close()
	}

	userCounts := map[int64]int{}
	userRows, err := h.Pool.Query(ctx, `
		SELECT ofo, COUNT(*)::int FROM public.user_info WHERE ofo ~ '^[0-9]+$' GROUP BY ofo`)
	if err == nil {
		for userRows.Next() {
			var ofoID int64
			var count int
			var ofoStr string
			if err := userRows.Scan(&ofoStr, &count); err == nil {
				if n, err := strconv.ParseInt(ofoStr, 10, 64); err == nil {
					ofoID = n
					userCounts[ofoID] = count
				}
			}
		}
		userRows.Close()
	}

	units := []map[string]any{}
	for unitRows.Next() {
		var id, categoryID, level, unitNumber, sortOrder int64
		var parentID, familyNumber *int64
		var name string
		if err := unitRows.Scan(&id, &name, &categoryID, &parentID, &level, &unitNumber, &familyNumber, &sortOrder); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
		units = append(units, map[string]any{
			"id":             id,
			"name":           name,
			"category_id":    categoryID,
			"parent_id":      parentID,
			"level":          level,
			"unit_number":    unitNumber,
			"family_number":  familyNumber,
			"sort_order":     sortOrder,
			"position_count": posCounts[unitNumber],
			"user_count":     userCounts[id],
		})
	}

	httpx.OK(w, map[string]any{
		"categories": categories,
		"units":      units,
	}, "OK")
}

// Positions — GET: должности подразделения; POST: добавить должность (админ).
func (h *OFO) Positions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.positionsList(w, r)
	case http.MethodPost:
		h.positionsAdd(w, r)
	default:
		httpx.MethodNotAllowed(w)
	}
}

const maxPositionNameLen = 200

// positionsAdd добавляет должность в справочник подразделения.
// Тело: { unit_number, name }. Если должность с таким названием уже есть в
// справочнике (у другого подразделения), привязывает её, а не плодит дубль.
func (h *OFO) positionsAdd(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r, h.Auth); !ok {
		return
	}
	var body struct {
		UnitNumber int64  `json:"unit_number"`
		Name       string `json:"name"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Некорректный JSON")
		return
	}
	name := strings.Join(strings.Fields(body.Name), " ")
	if body.UnitNumber <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "Укажите unit_number")
		return
	}
	if name == "" {
		httpx.Fail(w, http.StatusBadRequest, "Введите название должности")
		return
	}
	if utf8.RuneCountInString(name) > maxPositionNameLen {
		httpx.Fail(w, http.StatusBadRequest, "Название должности слишком длинное")
		return
	}

	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer tx.Rollback(ctx)

	// Справочник правят редко; сериализуем, чтобы два одновременных запроса
	// не создали дубль и не взяли один и тот же id.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ofo_position'))`); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}

	var unitExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM public.ofo_unit WHERE unit_number = $1)`, body.UnitNumber,
	).Scan(&unitExists); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	if !unitExists {
		httpx.Fail(w, http.StatusNotFound, "Подразделение не найдено")
		return
	}

	var id int64
	var isHead bool
	err = tx.QueryRow(ctx, `
		SELECT id, is_head FROM public.ofo_position
		WHERE lower(btrim(name)) = lower($1)
		ORDER BY id LIMIT 1`, name).Scan(&id, &isHead)
	switch {
	case err == nil:
		// Должность уже есть в справочнике — используем её.
	case errors.Is(err, pgx.ErrNoRows):
		// В проде id может быть serial/identity, в локальной схеме — нет:
		// берём последовательность, если она есть, иначе MAX(id)+1.
		var seq *string
		if err := tx.QueryRow(ctx,
			`SELECT pg_get_serial_sequence('public.ofo_position', 'id')`).Scan(&seq); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
		const sortOrder = `(SELECT COALESCE(MAX(sort_order), 0) + 1 FROM public.ofo_position)`
		if seq != nil {
			err = tx.QueryRow(ctx, `
				INSERT INTO public.ofo_position (name, is_head, sort_order)
				VALUES ($1, false, `+sortOrder+`) RETURNING id`, name).Scan(&id)
		} else {
			err = tx.QueryRow(ctx, `
				INSERT INTO public.ofo_position (id, name, is_head, sort_order)
				VALUES ((SELECT COALESCE(MAX(id), 0) + 1 FROM public.ofo_position), $1, false, `+sortOrder+`)
				RETURNING id`, name).Scan(&id)
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Не удалось добавить должность")
			return
		}
	default:
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO public.ofo_unit_position (unit_number, position_id)
		SELECT $1::bigint, $2::bigint
		WHERE NOT EXISTS (
			SELECT 1 FROM public.ofo_unit_position WHERE unit_number = $1::bigint AND position_id = $2::bigint
		)`, body.UnitNumber, id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Не удалось добавить должность")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, http.StatusConflict, "Такая должность в этом подразделении уже есть")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Не удалось добавить должность")
		return
	}
	httpx.Created(w, map[string]any{"id": id, "name": name, "is_head": isHead}, "Должность добавлена")
}

func (h *OFO) positionsList(w http.ResponseWriter, r *http.Request) {
	// Оргструктура — внутренние данные портала, анонимам не отдаём (SEC-001).
	if _, ok := requireUser(w, r, h.Auth); !ok {
		return
	}
	unitNumber, err := strconv.ParseInt(r.URL.Query().Get("unit_number"), 10, 64)
	if err != nil || unitNumber <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "Укажите unit_number")
		return
	}

	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.id, p.name, p.is_head
		FROM public.ofo_position p
		JOIN public.ofo_unit_position oup ON oup.position_id = p.id
		WHERE oup.unit_number = $1
		ORDER BY p.is_head DESC, p.sort_order, p.name`, unitNumber)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
		return
	}
	defer rows.Close()

	data := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		var isHead bool
		if err := rows.Scan(&id, &name, &isHead); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "Ошибка подключения к БД")
			return
		}
		data = append(data, map[string]any{
			"id":      id,
			"name":    name,
			"is_head": isHead,
		})
	}
	httpx.OK(w, data, "OK")
}
