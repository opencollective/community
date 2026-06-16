// Package calendar maps the event template's recurrence presets to an
// RFC 5545 RRULE subset, computes next occurrences for the homepage, and
// formats VEVENTs for the ICS feeds (docs/nostr/channels.md § events).
package calendar

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var weekdayCode = [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// PresetToRRule turns a form preset into an RRULE, deriving weekday and
// week-of-month from the event's start (EVT-09). Empty preset / "none"
// yields no recurrence.
func PresetToRRule(preset string, start time.Time) (string, error) {
	start = start.UTC()
	switch preset {
	case "", "none":
		return "", nil
	case "weekly":
		return "FREQ=WEEKLY", nil
	case "monthly":
		return "FREQ=MONTHLY", nil
	case "yearly":
		return "FREQ=YEARLY", nil
	case "weekday":
		return "FREQ=WEEKLY;BYDAY=" + weekdayCode[start.Weekday()], nil
	case "nth":
		nth := (start.Day()-1)/7 + 1
		return fmt.Sprintf("FREQ=MONTHLY;BYDAY=%d%s", nth, weekdayCode[start.Weekday()]), nil
	default:
		return "", fmt.Errorf("unknown recurrence %q", preset)
	}
}

type rule struct {
	freq     string
	interval int
	byday    string // e.g. "MO" or "2TU"
}

func parse(rrule string) (rule, bool) {
	r := rule{interval: 1}
	for _, part := range strings.Split(rrule, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "FREQ":
			r.freq = kv[1]
		case "INTERVAL":
			if n, err := strconv.Atoi(kv[1]); err == nil && n > 0 {
				r.interval = n
			}
		case "BYDAY":
			r.byday = kv[1]
		}
	}
	return r, r.freq != ""
}

// NextOccurrence returns the first occurrence at or after now, or start
// when there is no recurrence. The zero time means the (one-off) event is
// in the past.
func NextOccurrence(start time.Time, rrule string, now time.Time) time.Time {
	start = start.UTC()
	now = now.UTC()
	if rrule == "" {
		if start.Before(now) {
			return time.Time{}
		}
		return start
	}
	r, ok := parse(rrule)
	if !ok {
		if start.Before(now) {
			return time.Time{}
		}
		return start
	}
	t := start
	for i := 0; i < 600; i++ {
		if !t.Before(now) {
			return t
		}
		switch r.freq {
		case "WEEKLY":
			t = t.AddDate(0, 0, 7*r.interval)
		case "MONTHLY":
			if nth, wd, ok := parseByDay(r.byday); ok {
				t = nthWeekdayOfMonth(t.AddDate(0, r.interval, -t.Day()+1), nth, wd, start)
			} else {
				t = t.AddDate(0, r.interval, 0)
			}
		case "YEARLY":
			t = t.AddDate(r.interval, 0, 0)
		default:
			return start
		}
	}
	return time.Time{}
}

func parseByDay(byday string) (nth int, wd time.Weekday, ok bool) {
	if byday == "" {
		return 0, 0, false
	}
	num := strings.TrimRight(byday, "SUMOTUWEHFRA")
	code := byday[len(num):]
	if num == "" {
		return 0, 0, false // plain weekday (WEEKLY handles it)
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0, 0, false
	}
	for i, c := range weekdayCode {
		if c == code {
			return n, time.Weekday(i), true
		}
	}
	return 0, 0, false
}

// nthWeekdayOfMonth returns the nth occurrence of weekday wd in the month
// of ref, preserving the clock time of start.
func nthWeekdayOfMonth(ref time.Time, nth int, wd time.Weekday, start time.Time) time.Time {
	first := time.Date(ref.Year(), ref.Month(), 1, start.Hour(), start.Minute(), 0, 0, time.UTC)
	offset := (int(wd) - int(first.Weekday()) + 7) % 7
	day := 1 + offset + (nth-1)*7
	return time.Date(ref.Year(), ref.Month(), day, start.Hour(), start.Minute(), 0, 0, time.UTC)
}

// VEvent is one event rendered for an ICS feed.
type VEvent struct {
	UID       string
	Title     string
	Location  string
	Start     int64
	End       int64
	AllDay    bool
	RRule     string
	Cancelled bool
}

// ICS renders a calendar from VEvents.
func ICS(host string, events []VEvent) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//opencollective//community//EN\r\n")
	for _, e := range events {
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:%s@%s\r\n", e.UID, host)
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", icsEscape(e.Title))
		if e.AllDay {
			fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", time.Unix(e.Start, 0).UTC().Format("20060102"))
			fmt.Fprintf(&b, "DTEND;VALUE=DATE:%s\r\n", time.Unix(e.End, 0).UTC().Format("20060102"))
		} else {
			fmt.Fprintf(&b, "DTSTART:%s\r\n", time.Unix(e.Start, 0).UTC().Format("20060102T150405Z"))
			fmt.Fprintf(&b, "DTEND:%s\r\n", time.Unix(e.End, 0).UTC().Format("20060102T150405Z"))
		}
		if e.Location != "" {
			fmt.Fprintf(&b, "LOCATION:%s\r\n", icsEscape(e.Location))
		}
		if e.RRule != "" {
			fmt.Fprintf(&b, "RRULE:%s\r\n", e.RRule)
		}
		if e.Cancelled {
			b.WriteString("STATUS:CANCELLED\r\n")
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func icsEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\n", "\\n")
	return r.Replace(s)
}
