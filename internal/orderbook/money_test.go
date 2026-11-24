package orderbook

import (
	"encoding/json"
	"testing"
)

func TestMoneyFormatsTheWayTheMarketWritesIt(t *testing.T) {
	cases := []struct {
		name     string
		amount   Money
		currency Currency
		want     string
	}{
		{"nothing", 0, INR, "₹0.00"},
		{"one paisa", 1, INR, "₹0.01"},
		{"under a rupee", 99, INR, "₹0.99"},
		{"exactly a rupee", 100, INR, "₹1.00"},
		{"under a thousand", 99999, INR, "₹999.99"},
		{"a thousand", 100000, INR, "₹1,000.00"},
		{"ten thousand", 1000000, INR, "₹10,000.00"},
		{"a lakh groups the Indian way", 10000000, INR, "₹1,00,000.00"},
		{"ten lakh", 100000000, INR, "₹10,00,000.00"},
		{"a crore", 1000000000, INR, "₹1,00,00,000.00"},
		{"a deduction", -4000, INR, "-₹40.00"},
		{"pesos group in threes", 10000000, PHP, "₱100,000.00"},
		{"a million pesos", 100000000, PHP, "₱1,000,000.00"},
		{"peso paise", 18050, PHP, "₱180.50"},
		{"a peso deduction", -32000, PHP, "-₱320.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.amount.Format(c.currency); got != c.want {
				t.Fatalf("%d minor units in %s formatted as %q, want %q", c.amount, c.currency, got, c.want)
			}
		})
	}
}

func TestMoneyStringIsPlainDecimal(t *testing.T) {
	cases := []struct {
		amount Money
		want   string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{50, "0.50"},
		{32000, "320.00"},
		{32050, "320.50"},
		{-32050, "-320.50"},
		{100000000, "1000000.00"},
	}
	for _, c := range cases {
		if got := c.amount.String(); got != c.want {
			t.Errorf("%d printed as %q, want %q", c.amount, got, c.want)
		}
	}
}

func TestParseMoneyReadsWhatAShopkeeperTypes(t *testing.T) {
	cases := []struct {
		name  string
		typed string
		want  Money
		fails bool
	}{
		{"whole rupees", "320", 32000, false},
		{"with both places", "320.50", 32050, false},
		{"one decimal place", "320.5", 32050, false},
		{"a trailing point", "320.", 32000, false},
		{"just paise", ".75", 75, false},
		{"empty is nothing", "", 0, false},
		{"only spaces", "   ", 0, false},
		{"with a rupee sign", "₹320.50", 32050, false},
		{"with a peso sign", "₱180.00", 18000, false},
		{"with Rs.", "Rs.320", 32000, false},
		{"Indian grouping", "1,23,456.78", 12345678, false},
		{"western grouping", "123,456.78", 12345678, false},
		{"a negative", "-40", -4000, false},
		{"with spaces inside", "1 250.00", 125000, false},
		{"three decimal places", "320.505", 0, true},
		{"two decimal points", "32.0.5", 0, true},
		{"letters", "three twenty", 0, true},
		{"a number with a stray letter", "320a", 0, true},
		{"a bare range", "320-340", 0, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseMoney(c.typed)
			if c.fails {
				if err == nil {
					t.Fatalf("%q was read as %s; it should have been refused", c.typed, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q was refused: %v", c.typed, err)
			}
			if got != c.want {
				t.Fatalf("%q read as %d minor units, want %d", c.typed, got, c.want)
			}
		})
	}
}

func TestMoneyRoundTripsThroughJSON(t *testing.T) {
	type wrapper struct {
		Price Money `json:"price"`
	}

	for _, amount := range []Money{0, 1, 99, 32000, 32050, -4000, 1234567890} {
		raw, err := json.Marshal(wrapper{Price: amount})
		if err != nil {
			t.Fatalf("encoding %d: %v", amount, err)
		}
		var back wrapper
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("decoding %s: %v", raw, err)
		}
		if back.Price != amount {
			t.Fatalf("%d went out as %s and came back as %d", amount, raw, back.Price)
		}
	}
}

func TestMoneyReadsAJSONNumberAsMinorUnits(t *testing.T) {
	// A caller that works in minor units already should not have to turn them
	// into text first.
	var m Money
	if err := json.Unmarshal([]byte("32050"), &m); err != nil {
		t.Fatalf("reading a whole number: %v", err)
	}
	if m != 32050 {
		t.Fatalf("32050 read as %d", m)
	}

	if err := json.Unmarshal([]byte("320.5"), &m); err == nil {
		t.Fatal("a fractional JSON number was accepted; an amount must not arrive as a float")
	}
}

func TestMoneyIsNeverAFloat(t *testing.T) {
	// The classic float failure: a tenth plus two tenths is not three tenths.
	// In minor units it is exact, and the same sum made 100,000 times stays
	// exact.
	var total Money
	for i := 0; i < 100000; i++ {
		total += 10 // ten paise
	}
	if total != 1000000 {
		t.Fatalf("a hundred thousand ten-paise amounts came to %s, want 10000.00", total)
	}

	// And a price that a float cannot hold exactly survives the round trip.
	parsed, err := ParseMoney("0.07")
	if err != nil {
		t.Fatalf("reading 0.07: %v", err)
	}
	if parsed*3 != 21 {
		t.Fatalf("three times seven paise came to %d minor units, want 21", parsed*3)
	}
}

func TestDivRoundGoesHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		numerator   int64
		denominator int64
		want        int64
	}{
		{0, 3, 0},
		{10, 3, 3},
		{11, 3, 4},
		{5, 2, 3},
		{-5, 2, -3},
		{7, 2, 4},
		{-7, 2, -4},
		{3, 2, 2},
		{1, 2, 1},
		{-1, 2, -1},
		{1, 3, 0},
		{100, 1, 100},
		{5, -2, -3},
		{5, 0, 0},
	}

	for _, c := range cases {
		if got := divRound(c.numerator, c.denominator); got != c.want {
			t.Errorf("divRound(%d, %d) = %d, want %d", c.numerator, c.denominator, got, c.want)
		}
	}
}

func TestCurrencyNormalisesToSomethingKnown(t *testing.T) {
	cases := []struct {
		in   Currency
		want Currency
	}{
		{"INR", INR},
		{"inr", INR},
		{" PHP ", PHP},
		{"php", PHP},
		{"", INR},
		{"USD", INR},
	}
	for _, c := range cases {
		if got := c.in.Normalise(); got != c.want {
			t.Errorf("%q normalised to %q, want %q", c.in, got, c.want)
		}
	}
	if INR.DialCode() != "91" || PHP.DialCode() != "63" {
		t.Fatalf("dial codes are %q and %q", INR.DialCode(), PHP.DialCode())
	}
}
