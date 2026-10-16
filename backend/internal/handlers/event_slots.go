package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Времена мероприятия (event_slots, V23, ADR-056): этапы или потоки в один день, у
// каждого свой лимит мест. Сотрудник записывается на одно время мероприятия. У
// мероприятия всегда есть хотя бы одно время; пустое время — «время уточняется».

const eventSlotsMax = 20

// eventSlotInput — время из тела запроса. ID > 0 — существующее (правка), 0 — новое.
type eventSlotInput struct {
	ID              int64
	TimeStart       *string
	TimeEnd         *string
	MaxParticipants *int32
}

// eventSlotsJSON — времена мероприятия JSON-массивом в порядке показа, с числом записавшихся.
const eventSlotsJSON = `COALESCE((
	SELECT json_agg(json_build_object(
		'id', s.id,
		'time_start', COALESCE(s.time_start, ''),
		'time_end', COALESCE(s.time_end, ''),
		'max_participants', s.max_participants,
		'participants_count', (SELECT COUNT(*) FROM public.event_rsvps r WHERE r.slot_id = s.id)
	) ORDER BY s.position, s.id)
	FROM public.event_slots s WHERE s.event_id = e.id), '[]')::text`

// parseEventLimit — «лимит мест»: пусто / 0 — без ограничения.
func parseEventLimit(v any) (*int32, bool) {
	raw := strings.TrimSpace(strVal(v))
	if raw == "" || raw == "0" {
		return nil, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > eventParticipantsMax {
		return nil, false
	}
	n32 := int32(n)
	return &n32, true
}

// parseEventSlots — d["slots"]: [{id?, time_start, time_end, max_participants}].
// present=false — ключа нет (при правке времена не трогаем). Пустой список — одно
// пустое время. Вторым значением — текст ошибки для 422.
func parseEventSlots(d map[string]any) (slots []eventSlotInput, present bool, msg string) {
	raw, ok := d["slots"]
	if !ok || raw == nil {
		return nil, false, ""
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, true, "Времена мероприятия — списком"
	}
	if len(list) > eventSlotsMax {
		return nil, true, "Не больше " + strconv.Itoa(eventSlotsMax) + " времён у мероприятия"
	}
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, true, "Времена мероприятия — списком"
		}
		n := strconv.Itoa(i + 1)
		var s eventSlotInput
		s.ID = int64Of(m["id"])
		var valid bool
		if s.TimeStart, valid = normalizeEventTime(m["time_start"]); !valid {
			return nil, true, "Время " + n + ": начало — в формате ЧЧ:ММ"
		}
		if s.TimeEnd, valid = normalizeEventTime(m["time_end"]); !valid {
			return nil, true, "Время " + n + ": окончание — в формате ЧЧ:ММ"
		}
		if s.TimeEnd != nil && s.TimeStart == nil {
			return nil, true, "Время " + n + ": укажите начало"
		}
		if s.TimeEnd != nil && *s.TimeEnd < *s.TimeStart {
			return nil, true, "Время " + n + ": окончание раньше начала"
		}
		if s.MaxParticipants, valid = parseEventLimit(m["max_participants"]); !valid {
			return nil, true, "Время " + n + ": лимит мест — целое число от 1 (пусто — без ограничения)"
		}
		slots = append(slots, s)
	}
	if len(slots) == 0 {
		slots = []eventSlotInput{{}}
	}
	return slots, true, ""
}

// parseEventLocation — место мероприятия (одно на все времена). Ключа нет — cur.
func parseEventLocation(d map[string]any, cur *string) (*string, string) {
	v, ok := d["location"]
	if !ok {
		return cur, ""
	}
	loc := strings.TrimSpace(strVal(v))
	if utf8.RuneCountInString(loc) > eventLocationMax {
		return cur, "Место — не длиннее " + strconv.Itoa(eventLocationMax) + " символов"
	}
	if loc == "" {
		return nil, ""
	}
	return &loc, ""
}

// syncEventSlots — привести времена мероприятия к списку: существующие (по id этого
// мероприятия) обновить, новые добавить, остальные удалить (записи на них удаляются
// каскадом — форма предупреждает организатора). Порядок — как в списке.
func syncEventSlots(ctx context.Context, tx pgx.Tx, eventID int64, slots []eventSlotInput) error {
	rows, err := tx.Query(ctx, `SELECT id FROM public.event_slots WHERE event_id = $1`, eventID)
	if err != nil {
		return err
	}
	existing := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	keep := []int64{}
	for i, s := range slots {
		if s.ID > 0 && existing[s.ID] {
			if _, err := tx.Exec(ctx, `
				UPDATE public.event_slots
				SET position = $2, time_start = $3, time_end = $4, max_participants = $5
				WHERE id = $1`, s.ID, i, s.TimeStart, s.TimeEnd, s.MaxParticipants); err != nil {
				return err
			}
			keep = append(keep, s.ID)
			continue
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO public.event_slots (event_id, position, time_start, time_end, max_participants)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`, eventID, i, s.TimeStart, s.TimeEnd, s.MaxParticipants).Scan(&id); err != nil {
			return err
		}
		keep = append(keep, id)
	}
	_, err = tx.Exec(ctx, `DELETE FROM public.event_slots WHERE event_id = $1 AND NOT (id = ANY($2))`, eventID, keep)
	return err
}

// isUndefinedTable — нет таблицы (миграция не применена).
func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}
