package orderbook

import "testing"

func TestNormaliseCodeMakesOneCodeOutOfEveryWayOfTypingIt(t *testing.T) {
	cases := []struct {
		typed string
		want  string
	}{
		{"MANGO-500", "MANGO-500"},
		{"mango-500", "MANGO-500"},
		{"mango 500", "MANGO-500"},
		{"Mango_Pickle__500", "MANGO-PICKLE-500"},
		{"  mango   500  ", "MANGO-500"},
		{"mango-500-", "MANGO-500"},
		{"--mango--500--", "MANGO-500"},
		{"mango/500", "MANGO-500"},
		{"MANGO500", "MANGO500"},
		{"", ""},
		{"   ", ""},
		{"---", ""},
		{"अतार", ""},
		{"achar-अतार-500", "ACHAR-500"},
	}
	for _, c := range cases {
		if got := NormaliseCode(c.typed); got != c.want {
			t.Errorf("NormaliseCode(%q) = %q, want %q", c.typed, got, c.want)
		}
	}
}

func TestPackLabelReadsTheWayALabelDoes(t *testing.T) {
	cases := []struct {
		name    string
		amount  Qty
		measure Measure
		want    string
	}{
		{"half a kilogramme in grams", Whole(500), ByWeight, "500 g"},
		{"a kilogramme", Whole(1000), ByWeight, "1 kg"},
		{"a kilogramme and a half", Whole(1500), ByWeight, "1.5 kg"},
		{"two kilogrammes", Whole(2000), ByWeight, "2 kg"},
		{"an awkward weight", Whole(750), ByWeight, "750 g"},
		{"a quarter of a gram", 250, ByWeight, "0.25 g"},
		{"a quarter litre", Whole(250), ByVolume, "250 ml"},
		{"a litre", Whole(1000), ByVolume, "1 litre"},
		{"one piece", Whole(1), ByCount, "1 piece"},
		{"a dozen", Whole(12), ByCount, "12 pieces"},
		{"no measure at all", Whole(500), NoMeasure, ""},
		{"no amount at all", 0, ByWeight, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Product{Code: "X", Name: "Thing", PackAmount: c.amount, PackMeasure: c.measure, Price: 100}
			// Normalise drops the pack amount where there is no measure, which
			// is the behaviour being checked in that case.
			if got := p.Normalise().PackLabel(); got != c.want {
				t.Fatalf("pack label is %q, want %q", got, c.want)
			}
		})
	}
}

func TestPricePerStandardQuantityMakesPackSizesComparable(t *testing.T) {
	cases := []struct {
		name     string
		price    Money
		amount   Qty
		measure  Measure
		want     Money
		wantUnit string
		ok       bool
	}{
		{"a 500 g jar at 320", 32000, Whole(500), ByWeight, 64000, "kg", true},
		{"a 1 kg jar at 580", 58000, Whole(1000), ByWeight, 58000, "kg", true},
		{"a 400 g jar at 260", 26000, Whole(400), ByWeight, 65000, "kg", true},
		{"a 250 g packet at 140", 14000, Whole(250), ByWeight, 56000, "kg", true},
		{"a 250 ml jar at 150", 15000, Whole(250), ByVolume, 60000, "litre", true},
		{"a box of 12 at 480", 48000, Whole(12), ByCount, 4000, "piece", true},
		{"a dozen pandesal at 180", 18000, Whole(12), ByCount, 1500, "piece", true},
		{"an odd division rounds once", 10000, Whole(3), ByCount, 333333, "piece", false},
		{"no pack size", 32000, 0, ByWeight, 0, "", false},
		{"no measure", 32000, Whole(500), NoMeasure, 0, "", false},
		{"no price", 0, Whole(500), ByWeight, 0, "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Product{Code: "X", Name: "Thing", Price: c.price, PackAmount: c.amount, PackMeasure: c.measure}.Normalise()
			got, unit, ok := p.PerStandard()

			if c.name == "an odd division rounds once" {
				// A hundred rupees for three pieces is 33.33 and a third; the
				// figure printed is rounded once and stated as such.
				if !ok {
					t.Fatal("a price per piece should still be worked out")
				}
				if got != 3333 {
					t.Fatalf("a hundred for three came to %s a piece, want 33.33", got)
				}
				return
			}

			if !c.ok {
				if ok {
					t.Fatalf("a figure was produced where there should be none: %s a %s", got, unit)
				}
				return
			}
			if !ok {
				t.Fatal("no figure was produced")
			}
			if got != c.want || unit != c.wantUnit {
				t.Fatalf("got %s a %s, want %s a %s", got, unit, c.want, c.wantUnit)
			}
		})
	}
}

func TestPricePerKilogrammeExposesTheDearerPack(t *testing.T) {
	// The thing the figure is for: the big jar looks dearer and is cheaper.
	small := Product{Code: "S", Name: "Mango pickle", Price: 32000, PackAmount: Whole(500), PackMeasure: ByWeight}.Normalise()
	large := Product{Code: "L", Name: "Mango pickle", Price: 58000, PackAmount: Whole(1000), PackMeasure: ByWeight}.Normalise()

	smallPer, _, _ := small.PerStandard()
	largePer, _, _ := large.PerStandard()
	if !(largePer < smallPer) {
		t.Fatalf("the kilogramme jar at %s a kg is not cheaper than the half at %s a kg", largePer, smallPer)
	}
	if large.Price <= small.Price {
		t.Fatal("the test has lost its point: the larger jar should cost more in total")
	}
}

func TestProductNormalisationDropsTheContradictions(t *testing.T) {
	p := Product{
		Code:         "  mango 500 ",
		Name:         "  Mango pickle  ",
		Unit:         "",
		Price:        -100,
		PackAmount:   -5,
		PackMeasure:  "grams",
		Availability: "AVAILABLE",
		NoticeDays:   3,
		MinQty:       -2,
		Note:         "  no preservatives  ",
	}.Normalise()

	if p.Code != "MANGO-500" {
		t.Errorf("code is %q", p.Code)
	}
	if p.Name != "Mango pickle" {
		t.Errorf("name is %q", p.Name)
	}
	if p.Unit != "unit" {
		t.Errorf("a product with no unit became %q, want a fallback", p.Unit)
	}
	if p.Price != 0 || p.MinQty != 0 {
		t.Errorf("negative figures survived: price %s, minimum %s", p.Price, p.MinQty)
	}
	if p.PackMeasure != ByWeight {
		t.Errorf("measure is %q", p.PackMeasure)
	}
	if p.PackAmount != 0 {
		t.Errorf("a negative pack amount became %s", p.PackAmount)
	}
	if p.NoticeDays != 0 {
		t.Errorf("a stock item kept %d days' notice; it would refuse every order placed for today", p.NoticeDays)
	}
	if p.Note != "no preservatives" {
		t.Errorf("note is %q", p.Note)
	}
}

func TestNormalisationDropsAPackAmountWithNoMeasure(t *testing.T) {
	p := Product{Code: "X", Name: "Thing", Price: 100, PackAmount: Whole(500)}.Normalise()
	if p.PackAmount != 0 {
		t.Fatalf("a pack amount of %s survived with no measure to make sense of it", p.PackAmount)
	}
	if p.PackLabel() != "" {
		t.Fatalf("pack label is %q, want nothing", p.PackLabel())
	}
}

func TestAvailabilityAndMeasureNormaliseFromWhatAPersonTypes(t *testing.T) {
	availability := []struct {
		in   Availability
		want Availability
	}{
		{"available", Available},
		{"AVAILABLE", Available},
		{"made-to-order", MadeToOrder},
		{"made to order", MadeToOrder},
		{"sold-out", SoldOut},
		{"sold out", SoldOut},
		{"out of stock", SoldOut},
		{"", Available},
		{"maybe", Available},
	}
	for _, c := range availability {
		if got := c.in.Normalise(); got != c.want {
			t.Errorf("availability %q normalised to %q, want %q", c.in, got, c.want)
		}
	}

	measures := []struct {
		in   Measure
		want Measure
	}{
		{"g", ByWeight},
		{"gram", ByWeight},
		{"GRAMS", ByWeight},
		{"ml", ByVolume},
		{"millilitres", ByVolume},
		{"piece", ByCount},
		{"pcs", ByCount},
		{"nos", ByCount},
		{"", NoMeasure},
		{"furlongs", NoMeasure},
	}
	for _, c := range measures {
		if got := c.in.Normalise(); got != c.want {
			t.Errorf("measure %q normalised to %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLeadTimeOnlyAppliesToSomethingMadeToOrder(t *testing.T) {
	ordered := Date{2026, 11, 24}

	cases := []struct {
		name string
		p    Product
		want Date
	}{
		{"a stock item is ready the same day",
			Product{Code: "X", Name: "X", Price: 100, Availability: Available}, ordered},
		{"two days' notice",
			Product{Code: "X", Name: "X", Price: 100, Availability: MadeToOrder, NoticeDays: 2}, Date{2026, 11, 26}},
		{"notice over a month end",
			Product{Code: "X", Name: "X", Price: 100, Availability: MadeToOrder, NoticeDays: 7}, Date{2026, 12, 1}},
		{"made to order with no notice set",
			Product{Code: "X", Name: "X", Price: 100, Availability: MadeToOrder}, ordered},
		{"a sold-out item is not rescued by a notice period",
			Product{Code: "X", Name: "X", Price: 100, Availability: SoldOut, NoticeDays: 3}, ordered},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.p.Normalise().LeadTime(ordered); got != c.want {
				t.Fatalf("lead time is %s, want %s", got, c.want)
			}
		})
	}

	// And over the end of February, in both kinds of year.
	notice := Product{Code: "X", Name: "X", Price: 100, Availability: MadeToOrder, NoticeDays: 2}.Normalise()
	if got := notice.LeadTime(Date{2028, 2, 28}); got != (Date{2028, 3, 1}) {
		t.Fatalf("two days from 28 February 2028 is %s, want 2028-03-01", got)
	}
	if got := notice.LeadTime(Date{2027, 2, 27}); got != (Date{2027, 3, 1}) {
		t.Fatalf("two days from 27 February 2027 is %s, want 2027-03-01", got)
	}
}

func TestProductProblemsNameWhatIsWrong(t *testing.T) {
	cases := []struct {
		name string
		p    Product
		want []string
	}{
		{"a sound product", Product{Code: "X", Name: "Thing", Price: 100}, nil},
		{"no code", Product{Name: "Thing", Price: 100}, []string{ProblemNoProductCode}},
		{"no name", Product{Code: "X", Price: 100}, []string{ProblemNoProductName}},
		{"no price", Product{Code: "X", Name: "Thing"}, []string{ProblemNoProductPrice}},
		{"made to order with no notice",
			Product{Code: "X", Name: "Thing", Price: 100, Availability: MadeToOrder},
			[]string{ProblemShortNotice}},
		{"a name WhatsApp would read as formatting",
			Product{Code: "X", Name: "Kolhapuri ~extra hot~ chutney", Price: 100},
			[]string{ProblemFormattingInName}},
		{"nothing filled in at all",
			Product{},
			[]string{ProblemNoProductCode, ProblemNoProductName, ProblemNoProductPrice}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.p.Normalise().Problems()
			codes := map[string]bool{}
			for _, p := range got {
				codes[p.Code] = true
				if p.Message == "" {
					t.Errorf("problem %s has no message", p.Code)
				}
			}
			for _, want := range c.want {
				if !codes[want] {
					t.Errorf("%s was not reported; got %v", want, codes)
				}
			}
			if len(c.want) == 0 && len(got) != 0 {
				t.Errorf("a sound product reported %d problem(s)", len(got))
			}
		})
	}
}
