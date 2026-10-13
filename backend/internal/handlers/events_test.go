package handlers

import "testing"

func TestParseEventScheduleNew(t *testing.T) {
	got, msg := parseEventSchedule(map[string]any{
		"time_start":       "9:05",
		"time_end":         "15:30",
		"location":         "  Районный центр культуры  ",
		"max_participants": float64(30), // так приходит число из JSON
	}, eventSchedule{})
	if msg != "" {
		t.Fatal(msg)
	}
	if *got.TimeStart != "09:05" || *got.TimeEnd != "15:30" || *got.Location != "Районный центр культуры" || *got.MaxParticipants != 30 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseEventScheduleEmptyMeansUnset(t *testing.T) {
	got, msg := parseEventSchedule(map[string]any{
		"time_start": "", "time_end": nil, "location": " ", "max_participants": nil,
	}, eventSchedule{})
	if msg != "" || got.TimeStart != nil || got.TimeEnd != nil || got.Location != nil || got.MaxParticipants != nil {
		t.Fatalf("got %+v, %q", got, msg)
	}
	// 0 — тоже «без ограничения»
	got, _ = parseEventSchedule(map[string]any{"max_participants": float64(0)}, eventSchedule{})
	if got.MaxParticipants != nil {
		t.Fatal("0 должен значить «без ограничения»")
	}
}

func TestParseEventScheduleKeepsMissingKeys(t *testing.T) {
	start, loc := "10:00", "Зал"
	limit := int32(5)
	cur := eventSchedule{TimeStart: &start, Location: &loc, MaxParticipants: &limit}
	got, msg := parseEventSchedule(map[string]any{"time_end": "11:00"}, cur)
	if msg != "" || *got.TimeStart != "10:00" || *got.TimeEnd != "11:00" || *got.Location != "Зал" || *got.MaxParticipants != 5 {
		t.Fatalf("got %+v, %q", got, msg)
	}
}

func TestParseEventScheduleRejects(t *testing.T) {
	cases := map[string]map[string]any{
		"время не время":        {"time_start": "25:00"},
		"время текстом":         {"time_start": "в обед"},
		"конец без начала":      {"time_end": "12:00"},
		"конец раньше начала":   {"time_start": "13:00", "time_end": "12:59"},
		"дробный лимит":         {"max_participants": float64(2.5)},
		"отрицательный лимит":   {"max_participants": float64(-3)},
		"лимит текстом":         {"max_participants": "много"},
		"слишком длинное место": {"location": string(make([]rune, eventLocationMax+1))},
	}
	for name, d := range cases {
		if _, msg := parseEventSchedule(d, eventSchedule{}); msg == "" {
			t.Errorf("%s: принято %v", name, d)
		}
	}
}
