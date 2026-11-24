package orderbook

import (
	"encoding/json"
	"testing"
)

func TestLeapYearsFollowTheGregorianRule(t *testing.T) {
	cases := []struct {
		year int
		leap bool
	}{
		{2024, true},
		{2025, false},
		{2026, false},
		{2027, false},
		{2028, true},
		{1900, false},
		{2000, true},
		{2100, false},
		{2400, true},
	}
	for _, c := range cases {
		if got := IsLeapYear(c.year); got != c.leap {
			t.Errorf("IsLeapYear(%d) = %v, want %v", c.year, got, c.leap)
		}
	}
}

func TestDaysInMonth(t *testing.T) {
	cases := []struct {
		year, month, want int
	}{
		{2026, 1, 31},
		{2026, 2, 28},
		{2028, 2, 29},
		{2000, 2, 29},
		{1900, 2, 28},
		{2026, 4, 30},
		{2026, 11, 30},
		{2026, 12, 31},
		{2026, 13, 0},
		{2026, 0, 0},
	}
	for _, c := range cases {
		if got := DaysInMonth(c.year, c.month); got != c.want {
			t.Errorf("DaysInMonth(%d, %d) = %d, want %d", c.year, c.month, got, c.want)
		}
	}
}

func TestDateValidityKnowsWhichDaysExist(t *testing.T) {
	cases := []struct {
		name  string
		date  Date
		valid bool
	}{
		{"an ordinary day", Date{2026, 11, 24}, true},
		{"the last day of a long month", Date{2026, 1, 31}, true},
		{"the 31st of a short month", Date{2026, 11, 31}, false},
		{"the 29th of February in a leap year", Date{2028, 2, 29}, true},
		{"the 29th of February in a common year", Date{2027, 2, 29}, false},
		{"the 30th of February ever", Date{2028, 2, 30}, false},
		{"month zero", Date{2026, 0, 12}, false},
		{"month thirteen", Date{2026, 13, 1}, false},
		{"day zero", Date{2026, 11, 0}, false},
		{"nothing at all", Date{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.date.Valid(); got != c.valid {
				t.Fatalf("%v validity = %v, want %v", c.date, got, c.valid)
			}
		})
	}
}

func TestAddDaysCarriesOverEveryBoundary(t *testing.T) {
	cases := []struct {
		name string
		from Date
		add  int
		want Date
	}{
		{"an ordinary day", Date{2026, 11, 24}, 1, Date{2026, 11, 25}},
		{"a two-day notice", Date{2026, 11, 24}, 2, Date{2026, 11, 26}},
		{"over a 30-day month end", Date{2026, 11, 30}, 1, Date{2026, 12, 1}},
		{"over a 31-day month end", Date{2026, 1, 31}, 1, Date{2026, 2, 1}},
		{"over the year end", Date{2026, 12, 31}, 1, Date{2027, 1, 1}},
		{"into a common February", Date{2027, 2, 28}, 1, Date{2027, 3, 1}},
		{"into a leap February", Date{2028, 2, 28}, 1, Date{2028, 2, 29}},
		{"out of a leap February", Date{2028, 2, 29}, 1, Date{2028, 3, 1}},
		{"a week from the end of a leap February", Date{2028, 2, 27}, 7, Date{2028, 3, 5}},
		{"a week back over a month start", Date{2026, 3, 3}, -7, Date{2026, 2, 24}},
		{"a whole common year", Date{2026, 11, 24}, 365, Date{2027, 11, 24}},
		{"a whole year over a leap day", Date{2027, 11, 24}, 365, Date{2028, 11, 23}},
		{"no days at all", Date{2026, 11, 24}, 0, Date{2026, 11, 24}},
		{"nothing stays nothing", Date{}, 5, Date{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.from.AddDays(c.add); got != c.want {
				t.Fatalf("%s + %d days = %s, want %s", c.from, c.add, got, c.want)
			}
		})
	}
}

func TestDaysUntilCountsCalendarDays(t *testing.T) {
	cases := []struct {
		from, to Date
		want     int
	}{
		{Date{2026, 11, 24}, Date{2026, 11, 24}, 0},
		{Date{2026, 11, 24}, Date{2026, 11, 26}, 2},
		{Date{2026, 11, 26}, Date{2026, 11, 24}, -2},
		{Date{2026, 12, 30}, Date{2027, 1, 2}, 3},
		{Date{2028, 2, 27}, Date{2028, 3, 1}, 3},
		{Date{2027, 2, 27}, Date{2027, 3, 1}, 2},
		{Date{}, Date{2026, 11, 24}, 0},
	}
	for _, c := range cases {
		if got := c.from.DaysUntil(c.to); got != c.want {
			t.Errorf("%s until %s = %d, want %d", c.from, c.to, got, c.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		typed string
		want  Date
		fails bool
	}{
		{"2026-11-24", Date{2026, 11, 24}, false},
		{"2028-02-29", Date{2028, 2, 29}, false},
		{"", Date{}, false},
		{"  2026-11-24 ", Date{2026, 11, 24}, false},
		{"2027-02-29", Date{}, true},
		{"2026-13-01", Date{}, true},
		{"2026-11-31", Date{}, true},
		{"24-11-2026", Date{}, true},
		{"not a date", Date{}, true},
		{"2026/11/24", Date{}, true},
	}
	for _, c := range cases {
		got, err := ParseDate(c.typed)
		if c.fails {
			if err == nil {
				t.Errorf("%q was read as %s; it should have been refused", c.typed, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q was refused: %v", c.typed, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q read as %s, want %s", c.typed, got, c.want)
		}
	}
}

func TestDateRoundTripsThroughJSONAsText(t *testing.T) {
	type wrapper struct {
		On Date `json:"on"`
	}
	for _, d := range []Date{{2026, 11, 24}, {2028, 2, 29}, {}} {
		raw, err := json.Marshal(wrapper{On: d})
		if err != nil {
			t.Fatalf("encoding %s: %v", d, err)
		}
		var back wrapper
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("decoding %s: %v", raw, err)
		}
		if back.On != d {
			t.Fatalf("%s went out as %s and came back as %s", d, raw, back.On)
		}
	}
}

func TestDateReadsWrittenOutForACustomer(t *testing.T) {
	// A date in a message has to be unambiguous: 24-11-2026 is read as
	// December by half the world.
	if got := (Date{2026, 11, 24}).Pretty(); got != "Tuesday, 24 November 2026" {
		t.Fatalf("pretty form is %q", got)
	}
	if got := (Date{2028, 2, 29}).Pretty(); got != "Tuesday, 29 February 2028" {
		t.Fatalf("pretty form of a leap day is %q", got)
	}
	if got := (Date{}).Pretty(); got != "" {
		t.Fatalf("no day printed as %q", got)
	}
}

func TestSlotsNormaliseAndSortThroughTheDay(t *testing.T) {
	cases := []struct {
		in   Slot
		want Slot
	}{
		{"morning", SlotMorning},
		{"MORNING", SlotMorning},
		{" evening ", SlotEvening},
		{"afternoon", SlotAfternoon},
		{"", SlotAny},
		{"midnight", SlotAny},
	}
	for _, c := range cases {
		if got := c.in.Normalise(); got != c.want {
			t.Errorf("%q normalised to %q, want %q", c.in, got, c.want)
		}
	}

	if !(SlotMorning.order() < SlotAfternoon.order() &&
		SlotAfternoon.order() < SlotEvening.order() &&
		SlotEvening.order() < SlotAny.order()) {
		t.Fatal("the parts of the day do not sort through the day with no-particular-time last")
	}
	if SlotAny.Name() != "any time" {
		t.Fatalf("no particular part of the day reads as %q", SlotAny.Name())
	}
}
