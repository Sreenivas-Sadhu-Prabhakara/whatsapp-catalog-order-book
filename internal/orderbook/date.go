package orderbook

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Date is a calendar day with no time and no zone. A delivery is on a day, not
// at an instant, and a shop in Pune and a shop in Cebu both think of "Friday"
// as the same kind of thing.
type Date struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// Today is the day the machine thinks it is, in its own local zone.
func Today() Date {
	now := time.Now()
	return Date{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}
}

// IsZero reports whether no day has been given.
func (d Date) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// Valid reports whether this is a day that exists. The 29th of February exists
// in 2028 and does not in 2027.
func (d Date) Valid() bool {
	if d.Year < 1900 || d.Year > 2200 || d.Month < 1 || d.Month > 12 || d.Day < 1 {
		return false
	}
	return d.Day <= DaysInMonth(d.Year, d.Month)
}

// DaysInMonth is how many days that month of that year has.
func DaysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if IsLeapYear(year) {
			return 29
		}
		return 28
	}
	return 0
}

// IsLeapYear applies the Gregorian rule: every fourth year, except every
// hundredth, except every four-hundredth. 2000 was a leap year and 1900 was
// not.
func IsLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// String writes the day as a shop's own file would: 2026-11-24.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// MarshalJSON sends the day as its text, or as nothing where there is no day.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte(`""`), nil
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON reads a day from 2026-11-24, which is what a date field on a
// web page sends.
func (d *Date) UnmarshalJSON(raw []byte) error {
	var typed string
	if err := json.Unmarshal(raw, &typed); err != nil {
		// An older file may hold the three numbers.
		var parts struct {
			Year  int `json:"year"`
			Month int `json:"month"`
			Day   int `json:"day"`
		}
		if err2 := json.Unmarshal(raw, &parts); err2 != nil {
			return err
		}
		*d = Date{Year: parts.Year, Month: parts.Month, Day: parts.Day}
		return nil
	}
	parsed, err := ParseDate(typed)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// ParseDate reads 2026-11-24. An empty string is no day, which is allowed: an
// order taken for "whenever it is ready" has no delivery date yet.
func ParseDate(typed string) (Date, error) {
	trimmed := strings.TrimSpace(typed)
	if trimmed == "" {
		return Date{}, nil
	}
	var d Date
	if n, err := fmt.Sscanf(trimmed, "%4d-%2d-%2d", &d.Year, &d.Month, &d.Day); err != nil || n != 3 {
		return Date{}, fmt.Errorf("%q is not a date; write it as 2026-11-24", typed)
	}
	if !d.Valid() {
		return Date{}, fmt.Errorf("%q is not a day that exists", typed)
	}
	return d, nil
}

// time turns the day into an instant at midnight, for the arithmetic the
// standard library already does correctly.
func (d Date) time() time.Time {
	return time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC)
}

// fromTime brings an instant back to a day.
func fromTime(t time.Time) Date {
	return Date{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}

// AddDays moves the day forward, or back where the count is negative, carrying
// over month and year ends and through the end of February.
func (d Date) AddDays(days int) Date {
	if d.IsZero() {
		return d
	}
	return fromTime(d.time().AddDate(0, 0, days))
}

// Before reports whether this day comes before another.
func (d Date) Before(other Date) bool { return d.time().Before(other.time()) }

// After reports whether this day comes after another.
func (d Date) After(other Date) bool { return other.Before(d) }

// DaysUntil is how many days from this day to another: negative where the
// other day has passed.
func (d Date) DaysUntil(other Date) int {
	if d.IsZero() || other.IsZero() {
		return 0
	}
	return int(other.time().Sub(d.time()).Hours() / 24)
}

// Weekday names the day of the week, because a shop takes orders by the day of
// the week as much as by the date.
func (d Date) Weekday() string {
	if d.IsZero() {
		return ""
	}
	return d.time().Weekday().String()
}

// Pretty writes the day the way a message to a customer should read: Tuesday,
// 24 November 2026. A bare 24-11-2026 is read as the 11th of December by
// someone used to the other order, and a delivery on the wrong day is the one
// mistake a customer does not forgive.
func (d Date) Pretty() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s, %d %s %d", d.Weekday(), d.Day, time.Month(d.Month).String(), d.Year)
}

// Slot is the part of the day a delivery or a pickup is for. A home kitchen
// delivers in batches, not at an appointed minute, so the book holds a part of
// the day and not a time.
type Slot string

const (
	// SlotAny is no particular part of the day.
	SlotAny Slot = ""
	// SlotMorning is before midday.
	SlotMorning Slot = "morning"
	// SlotAfternoon is the middle of the day.
	SlotAfternoon Slot = "afternoon"
	// SlotEvening is the end of the day.
	SlotEvening Slot = "evening"
)

// Slots are the parts of a day, in the order they happen.
func Slots() []Slot { return []Slot{SlotAny, SlotMorning, SlotAfternoon, SlotEvening} }

// Normalise brings an unknown part of the day back to no particular part.
func (s Slot) Normalise() Slot {
	switch Slot(strings.ToLower(strings.TrimSpace(string(s)))) {
	case SlotMorning:
		return SlotMorning
	case SlotAfternoon:
		return SlotAfternoon
	case SlotEvening:
		return SlotEvening
	default:
		return SlotAny
	}
}

// Name is the part of the day written out.
func (s Slot) Name() string {
	switch s.Normalise() {
	case SlotMorning:
		return "morning"
	case SlotAfternoon:
		return "afternoon"
	case SlotEvening:
		return "evening"
	default:
		return "any time"
	}
}

// order is where in the day this part falls, for sorting a day's orders into
// the sequence they have to be made and sent in.
func (s Slot) order() int {
	switch s.Normalise() {
	case SlotMorning:
		return 1
	case SlotAfternoon:
		return 2
	case SlotEvening:
		return 3
	default:
		return 4
	}
}
