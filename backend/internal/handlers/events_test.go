package handlers

import "testing"

func TestParseEventSlots(t *testing.T) {
	slots, present, msg := parseEventSlots(map[string]any{"slots": []any{
		map[string]any{"id": float64(7), "time_start": "9:05", "time_end": "11:00", "max_participants": float64(30)},
		map[string]any{"time_start": "12:00", "time_end": "", "max_participants": nil},
	}})
	if msg != "" || !present || len(slots) != 2 {
		t.Fatalf("%q %v %d", msg, present, len(slots))
	}
	a, b := slots[0], slots[1]
	if a.ID != 7 || *a.TimeStart != "09:05" || *a.TimeEnd != "11:00" || *a.MaxParticipants != 30 {
		t.Fatalf("slot 1 %+v", a)
	}
	if b.ID != 0 || *b.TimeStart != "12:00" || b.TimeEnd != nil || b.MaxParticipants != nil {
		t.Fatalf("slot 2 %+v", b)
	}
}

func TestParseEventSlotsAbsentAndEmpty(t *testing.T) {
	if _, present, msg := parseEventSlots(map[string]any{}); present || msg != "" {
		t.Fatal("без ключа slots времена не трогаем")
	}
	// Пустой список — одно пустое время («время уточняется»).
	slots, present, msg := parseEventSlots(map[string]any{"slots": []any{}})
	if !present || msg != "" || len(slots) != 1 || slots[0].TimeStart != nil {
		t.Fatalf("%+v %v %q", slots, present, msg)
	}
	// 0 — без ограничения
	slots, _, _ = parseEventSlots(map[string]any{"slots": []any{map[string]any{"max_participants": float64(0)}}})
	if slots[0].MaxParticipants != nil {
		t.Fatal("0 должен значить «без ограничения»")
	}
}

func TestParseEventSlotsRejects(t *testing.T) {
	tooMany := make([]any, eventSlotsMax+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{}
	}
	cases := map[string]any{
		"не список":           "11:00",
		"время не время":      []any{map[string]any{"time_start": "25:00"}},
		"время текстом":       []any{map[string]any{"time_start": "в обед"}},
		"конец без начала":    []any{map[string]any{"time_end": "12:00"}},
		"конец раньше начала": []any{map[string]any{"time_start": "13:00", "time_end": "12:59"}},
		"дробный лимит":       []any{map[string]any{"max_participants": float64(2.5)}},
		"отрицательный лимит": []any{map[string]any{"max_participants": float64(-3)}},
		"лимит текстом":       []any{map[string]any{"max_participants": "много"}},
		"слишком много":       tooMany,
	}
	for name, v := range cases {
		if _, _, msg := parseEventSlots(map[string]any{"slots": v}); msg == "" {
			t.Errorf("%s: принято", name)
		}
	}
}

func TestParseEventLocation(t *testing.T) {
	cur := "Зал"
	if got, _ := parseEventLocation(map[string]any{}, &cur); got != &cur {
		t.Fatal("без ключа — текущее место")
	}
	if got, _ := parseEventLocation(map[string]any{"location": "  РЦК  "}, nil); *got != "РЦК" {
		t.Fatal(*got)
	}
	if got, _ := parseEventLocation(map[string]any{"location": nil}, &cur); got != nil {
		t.Fatal("null — место не указано")
	}
	if _, msg := parseEventLocation(map[string]any{"location": string(make([]rune, eventLocationMax+1))}, nil); msg == "" {
		t.Fatal("слишком длинное место принято")
	}
}
