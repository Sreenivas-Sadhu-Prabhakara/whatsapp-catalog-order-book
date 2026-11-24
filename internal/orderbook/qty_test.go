package orderbook

import (
	"encoding/json"
	"testing"
)

func TestQtyPrintsOnlyThePlacesItNeeds(t *testing.T) {
	cases := []struct {
		amount Qty
		want   string
	}{
		{0, "0"},
		{1000, "1"},
		{3000, "3"},
		{500, "0.5"},
		{250, "0.25"},
		{2250, "2.25"},
		{1, "0.001"},
		{10, "0.01"},
		{12345, "12.345"},
		{-1500, "-1.5"},
	}
	for _, c := range cases {
		if got := c.amount.String(); got != c.want {
			t.Errorf("%d thousandths printed as %q, want %q", c.amount, got, c.want)
		}
	}
}

func TestParseQtyReadsWhatAShopkeeperTypes(t *testing.T) {
	cases := []struct {
		name  string
		typed string
		want  Qty
		fails bool
	}{
		{"a count", "3", 3000, false},
		{"a half", "0.5", 500, false},
		{"a half with no leading zero", ".5", 500, false},
		{"two and a quarter", "2.25", 2250, false},
		{"grams of a kilogramme", "0.125", 125, false},
		{"empty is nothing", "", 0, false},
		{"a dozen", "12", 12000, false},
		{"with spaces", " 4 ", 4000, false},
		{"four decimal places", "0.1234", 0, true},
		{"a fraction written as a fraction", "1 1/2", 0, true},
		{"a word", "half", 0, true},
		{"two points", "1.2.3", 0, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseQty(c.typed)
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
				t.Fatalf("%q read as %d thousandths, want %d", c.typed, got, c.want)
			}
		})
	}
}

func TestQtyTimesPriceRoundsOnceAndStaysWhole(t *testing.T) {
	cases := []struct {
		name  string
		qty   Qty
		price Money
		want  Money
	}{
		{"three jars at 320", Whole(3), 32000, 96000},
		{"nothing at all", 0, 32000, 0},
		{"half a kilogramme at 300", 500, 30000, 15000},
		{"a third of a kilogramme at 300", 333, 30000, 9990},
		{"one and a half at 3.33 rounds up from a half", 1500, 333, 500},
		{"one and a half at 3.31 rounds up from .965", 1500, 331, 497},
		{"a gram of a 450 rupee kilogramme", 1, 45000, 45},
		{"a quarter of an odd price", 250, 101, 25},
		{"free at any quantity", Whole(7), 0, 0},
		{"a dozen at 15", Whole(12), 1500, 18000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.qty.Times(c.price); got != c.want {
				t.Fatalf("%s of something at %s came to %s, want %s",
					c.qty, c.price, got, c.want)
			}
		})
	}
}

func TestQtyRoundTripsThroughJSON(t *testing.T) {
	type wrapper struct {
		Qty Qty `json:"qty"`
	}
	for _, amount := range []Qty{0, 1, 500, 3000, 12345, -1500} {
		raw, err := json.Marshal(wrapper{Qty: amount})
		if err != nil {
			t.Fatalf("encoding %d: %v", amount, err)
		}
		var back wrapper
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("decoding %s: %v", raw, err)
		}
		if back.Qty != amount {
			t.Fatalf("%d went out as %s and came back as %d", amount, raw, back.Qty)
		}
	}
}
