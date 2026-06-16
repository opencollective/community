package calendar

import (
	"testing"
	"time"
)

// TestEVT09_PresetToRRule pins the recurrence-encoding half of EVT-09.
func TestEVT09_PresetToRRule(t *testing.T) {
	// 2026-06-08 is a Monday; the 2nd Tuesday of June 2026 is the 9th.
	monday := time.Date(2026, 6, 8, 17, 0, 0, 0, time.UTC)
	secondTue := time.Date(2026, 6, 9, 17, 0, 0, 0, time.UTC)

	cases := []struct {
		preset string
		start  time.Time
		want   string
	}{
		{"none", monday, ""},
		{"weekly", monday, "FREQ=WEEKLY"},
		{"monthly", monday, "FREQ=MONTHLY"},
		{"yearly", monday, "FREQ=YEARLY"},
		{"weekday", monday, "FREQ=WEEKLY;BYDAY=MO"},
		{"nth", secondTue, "FREQ=MONTHLY;BYDAY=2TU"},
	}
	for _, c := range cases {
		got, err := PresetToRRule(c.preset, c.start)
		if err != nil || got != c.want {
			t.Errorf("PresetToRRule(%q) = %q, %v; want %q", c.preset, got, err, c.want)
		}
	}
	if _, err := PresetToRRule("bogus", monday); err == nil {
		t.Error("unknown preset must error")
	}
}

// TestNextOccurrence covers the homepage next-occurrence computation.
func TestNextOccurrence(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	// One-off in the past → zero.
	past := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	if !NextOccurrence(past, "", now).IsZero() {
		t.Error("a past one-off must have no next occurrence")
	}
	// One-off in the future → itself.
	future := time.Date(2026, 6, 20, 17, 0, 0, 0, time.UTC)
	if got := NextOccurrence(future, "", now); !got.Equal(future) {
		t.Errorf("future one-off: got %v", got)
	}
	// Weekly every Monday, started in the past → next Monday on/after now.
	monday := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC) // a Monday
	got := NextOccurrence(monday, "FREQ=WEEKLY;BYDAY=MO", now)
	if got.Weekday() != time.Monday || got.Before(now) {
		t.Errorf("weekly Monday: got %v (%s)", got, got.Weekday())
	}
	if got.Day() != 22 { // 2026-06-22 is the first Monday >= June 16
		t.Errorf("weekly Monday next: want June 22, got %v", got)
	}
	// Monthly 2nd Tuesday, started June → next is July's 2nd Tuesday (14th).
	secondTue := time.Date(2026, 6, 9, 17, 0, 0, 0, time.UTC)
	got = NextOccurrence(secondTue, "FREQ=MONTHLY;BYDAY=2TU", now)
	if got.Month() != time.July || got.Day() != 14 {
		t.Errorf("monthly 2nd Tuesday: want July 14, got %v", got)
	}
}
