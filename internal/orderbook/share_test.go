package orderbook

import (
	"net/url"
	"strings"
	"testing"
)

func TestTheSharedCatalogueCarriesEveryPriceAndTheComparableFigure(t *testing.T) {
	shop := testShop()
	catalogue := testCatalogue()
	catalogue = catalogue.Upsert(Product{
		Code: "MANGO-1000", Name: "Mango pickle", Unit: "jar", Price: 58000,
		PackAmount: Whole(1000), PackMeasure: ByWeight, Availability: Available,
	})

	share := ShareCatalogue(shop, catalogue)

	for _, want := range []string{
		"Aaji's Kitchen — Pune",
		"Prices from Sunday, 15 November 2026",
		"Available now",
		"*Mango pickle* 500 g — ₹320.00 a jar",
		"(₹640.00 a kg)", // the figure that makes the two jars comparable
		"*Mango pickle* 1 kg — ₹580.00 a jar",
		"*Lime pickle* 400 g — ₹260.00 a jar",
		"Made to order",
		"*Chakli* 250 g",
		"2 days' notice",
		"least 2",
		"Sold out just now",
		"Idli podi (200 g)",
		"Payment: UPI test@examplebank",
		"Orders: +91 9876543210",
	} {
		if !strings.Contains(share.Text, want) {
			t.Errorf("the shared catalogue does not carry %q\n---\n%s\n---", want, share.Text)
		}
	}

	// The one-kilogramme jar's per-kilogramme figure is its own price, so it is
	// not repeated.
	if strings.Contains(share.Text, "₹580.00 a jar (₹580.00 a kg)") {
		t.Errorf("the kilogramme jar repeats its own price as a per-kilogramme figure:\n%s", share.Text)
	}

	if share.Listed != 4 {
		t.Errorf("%d products listed, want 4", share.Listed)
	}
	if share.SoldOut != 1 {
		t.Errorf("%d sold out, want 1", share.SoldOut)
	}
	if share.Characters != characters(share.Text) {
		t.Errorf("the character count is %d and the text is %d characters", share.Characters, characters(share.Text))
	}
	if share.Lines < 10 {
		t.Errorf("the catalogue runs to %d lines", share.Lines)
	}
}

func TestTheSharedCatalogueIsRebuiltWhenAPriceChanges(t *testing.T) {
	// The point of building the text rather than keeping it: a price list held
	// as text goes stale the moment a price moves.
	shop := testShop()
	catalogue := testCatalogue()

	before := ShareCatalogue(shop, catalogue)
	if !strings.Contains(before.Text, "₹320.00") {
		t.Fatalf("the old price is not in the old text:\n%s", before.Text)
	}

	mango := mustFind(t, catalogue, "MANGO-500")
	mango.Price = 36000
	catalogue = catalogue.Upsert(mango)
	catalogue.PricesChangedOn = Date{2026, 11, 24}

	after := ShareCatalogue(shop, catalogue)
	if strings.Contains(after.Text, "₹320.00 a jar") {
		t.Fatalf("the new text still carries the old price:\n%s", after.Text)
	}
	if !strings.Contains(after.Text, "₹360.00 a jar") {
		t.Fatalf("the new text does not carry the new price:\n%s", after.Text)
	}
	if !strings.Contains(after.Text, "₹720.00 a kg") {
		t.Fatalf("the per-kilogramme figure did not move with the price:\n%s", after.Text)
	}
	if !strings.Contains(after.Text, "Prices from Tuesday, 24 November 2026") {
		t.Fatalf("the text does not say when the prices are from:\n%s", after.Text)
	}
	if after.Text == before.Text {
		t.Fatal("the text did not change at all")
	}
}

func TestSoldOutThingsAreListedRatherThanLeftOut(t *testing.T) {
	// Leaving them off looks tidier and produces an order for them the same
	// afternoon.
	share := ShareCatalogue(testShop(), testCatalogue())
	if !strings.Contains(share.Text, "Idli podi") {
		t.Fatalf("the sold-out item is not in the catalogue at all:\n%s", share.Text)
	}
	if !strings.Contains(share.Text, "Sold out just now") {
		t.Fatalf("the sold-out item is not marked as sold out:\n%s", share.Text)
	}
	// And it is not in the available section.
	available := share.Text[strings.Index(share.Text, "Available now"):strings.Index(share.Text, "Made to order")]
	if strings.Contains(available, "Idli podi") {
		t.Fatalf("a sold-out item is listed as available:\n%s", available)
	}
}

func TestAHiddenProductIsLeftOutAndCounted(t *testing.T) {
	catalogue := testCatalogue()
	catalogue = catalogue.Upsert(Product{
		Code: "HAMPER", Name: "Festival hamper", Unit: "hamper", Price: 125000,
		Availability: MadeToOrder, NoticeDays: 4, Hidden: true,
	})

	share := ShareCatalogue(testShop(), catalogue)
	if strings.Contains(share.Text, "Festival hamper") {
		t.Fatalf("a hidden product is in the shared text:\n%s", share.Text)
	}
	if share.Hidden != 1 {
		t.Fatalf("%d hidden, want 1", share.Hidden)
	}
}

func TestAnUnpricedOrUnnamedProductIsLeftOutOfTheSharedText(t *testing.T) {
	catalogue := Catalogue{Products: []Product{
		{Code: "GOOD", Name: "A real thing", Unit: "jar", Price: 10000},
		{Code: "NOPRICE", Name: "No price yet", Unit: "jar"},
		{Code: "NONAME", Price: 10000, Unit: "jar"},
	}}.Normalise()

	share := ShareCatalogue(testShop(), catalogue)
	if !strings.Contains(share.Text, "A real thing") {
		t.Fatalf("the sound product is missing:\n%s", share.Text)
	}
	if strings.Contains(share.Text, "No price yet") {
		t.Fatalf("an unpriced product is in the shared text:\n%s", share.Text)
	}
	if share.Listed != 1 {
		t.Fatalf("%d listed, want 1", share.Listed)
	}
	// And the catalogue's own problems come with it.
	if !hasProblem(share.Problems, ProblemNoProductPrice) {
		t.Fatalf("the unpriced product was not reported: %s", problemCodes(share.Problems))
	}
}

func TestACatalogueWithNothingToShareSaysSo(t *testing.T) {
	cases := []struct {
		name      string
		catalogue Catalogue
	}{
		{"empty", Catalogue{}},
		{"everything hidden", Catalogue{Products: []Product{
			{Code: "A", Name: "A", Price: 1000, Hidden: true},
		}}},
		{"everything sold out", Catalogue{Products: []Product{
			{Code: "A", Name: "A", Price: 1000, Availability: SoldOut},
		}}},
		{"everything unpriced", Catalogue{Products: []Product{
			{Code: "A", Name: "A"},
		}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			share := ShareCatalogue(testShop(), c.catalogue.Normalise())
			if share.Listed != 0 {
				t.Fatalf("%d products listed", share.Listed)
			}
			if !hasProblem(share.Problems, ProblemNothingToShare) {
				t.Fatalf("nothing-to-share was not reported: %s", problemCodes(share.Problems))
			}
			if !blocked(share.Problems) {
				t.Fatal("a catalogue with nothing in it to share is not blocked")
			}
		})
	}
}

func TestTheSharedCatalogueSaysWhetherItFitsAStatus(t *testing.T) {
	shop := testShop()

	// A short catalogue fits.
	short := Catalogue{Products: []Product{
		{Code: "A", Name: "Mango pickle", Unit: "jar", Price: 32000, PackAmount: Whole(500), PackMeasure: ByWeight},
	}}.Normalise()
	share := ShareCatalogue(shop, short)
	if !share.FitsStatus {
		t.Fatalf("a %d-character catalogue does not fit a %d-character status", share.Characters, share.StatusLimit)
	}
	if hasProblem(share.Problems, ProblemTooLongForStatus) {
		t.Fatal("a short catalogue was reported as too long for a status")
	}

	// Forty products do not.
	long := Catalogue{}
	for i := 0; i < 40; i++ {
		long.Products = append(long.Products, Product{
			Code:  NormaliseCode(string(rune('A'+i%26)) + "-item-" + Qty(Whole(int64(i))).String()),
			Name:  "Something with a reasonably long name " + Qty(Whole(int64(i))).String(),
			Unit:  "jar",
			Price: Money(10000 + i*100), PackAmount: Whole(500), PackMeasure: ByWeight,
		})
	}
	share = ShareCatalogue(shop, long.Normalise())
	if share.FitsStatus {
		t.Fatalf("a %d-character catalogue was said to fit a %d-character status", share.Characters, share.StatusLimit)
	}
	if !hasProblem(share.Problems, ProblemTooLongForStatus) {
		t.Fatalf("a long catalogue was not reported as too long for a status: %s", problemCodes(share.Problems))
	}
	if !share.FitsMessage {
		t.Fatalf("a %d-character catalogue was said not to fit a %d-character message",
			share.Characters, share.MessageLimit)
	}

	// The limit the answer was measured against is reported, so the page need
	// not keep its own copy.
	if share.StatusLimit != DefaultStatusCharacters || share.MessageLimit != DefaultMessageCharacters {
		t.Fatalf("the limits reported are %d and %d", share.StatusLimit, share.MessageLimit)
	}
}

func TestTheStatusLimitIsTheShopsOwnFigure(t *testing.T) {
	shop := testShop()
	catalogue := testCatalogue()

	generous := shop
	generous.Limits.StatusCharacters = 100000
	if !ShareCatalogue(generous, catalogue).FitsStatus {
		t.Fatal("the catalogue does not fit a 100,000-character status")
	}

	tight := shop
	tight.Limits.StatusCharacters = 20
	share := ShareCatalogue(tight, catalogue)
	if share.FitsStatus {
		t.Fatal("the catalogue fits a 20-character status")
	}
	if share.StatusLimit != 20 {
		t.Fatalf("the limit reported is %d, want the shop's own 20", share.StatusLimit)
	}
	for _, p := range share.Problems {
		if p.Code == ProblemTooLongForStatus && !strings.Contains(p.Message, "20") {
			t.Fatalf("the message does not state the limit in force: %q", p.Message)
		}
	}
}

func TestTheSharedCatalogueSurvivesItsOwnLink(t *testing.T) {
	// Every character a shop puts in a price list, through the link and back.
	shop := testShop()
	shop.Name = "Aaji's Kitchen & Co."
	shop.HowToOrder = "Reply with the item + how many. 50% advance for orders over ₹2,000."
	shop.PayTo = "UPI aaji@bank #1"

	catalogue := testCatalogue()
	catalogue = catalogue.Upsert(Product{
		Code: "ACHAR", Name: "अतार (mango)", Unit: "jar", Price: 32000,
		PackAmount: Whole(500), PackMeasure: ByWeight, Availability: Available,
		Note: "घरी बनवलेले \U0001f96d",
	})

	share := ShareCatalogue(shop, catalogue)
	if share.Link.URL == "" {
		t.Fatal("the shared catalogue has no link")
	}

	parsed, err := url.Parse(share.Link.URL)
	if err != nil {
		t.Fatalf("the catalogue's link does not parse: %v", err)
	}
	if got := parsed.Query().Get("text"); got != share.Text {
		t.Fatalf("the catalogue did not survive its own link.\n  sent: %q\n  back: %q", share.Text, got)
	}
	if got, err := url.PathUnescape(strings.TrimPrefix(parsed.RawQuery, "text=")); err != nil || got != share.Text {
		t.Fatalf("read by plain percent-decoding the catalogue became %q (%v)", got, err)
	}
}

func TestAShopWithNoNumberGetsTheCatalogueToCopy(t *testing.T) {
	shop := testShop()
	shop.Phone = ""

	share := ShareCatalogue(shop, testCatalogue())
	if share.Link.URL != "" {
		t.Fatalf("a link was built with no shop number: %q", share.Link.URL)
	}
	if share.Link.Text != share.Text {
		t.Fatal("the copy-only link does not carry the catalogue")
	}
	if share.Link.Note == "" {
		t.Fatal("the copy-only link gives no reason")
	}
	if !strings.Contains(share.Text, "Available now") {
		t.Fatalf("the catalogue text is missing:\n%s", share.Text)
	}
	if strings.Contains(share.Text, "Orders:") {
		t.Fatalf("a shop with no number printed an orders line:\n%s", share.Text)
	}
}

func TestAProductNameWithFormattingIsNotEmphasisedInTheSharedText(t *testing.T) {
	catalogue := Catalogue{Products: []Product{
		{Code: "HOT", Name: "Kolhapuri ~extra hot~ chutney", Unit: "jar", Price: 18000},
		{Code: "MILD", Name: "Mild chutney", Unit: "jar", Price: 16000},
	}}.Normalise()

	share := ShareCatalogue(testShop(), catalogue)
	if strings.Contains(share.Text, "*Kolhapuri ~extra hot~ chutney*") {
		t.Fatalf("a name with a tilde in it was wrapped in asterisks:\n%s", share.Text)
	}
	if !strings.Contains(share.Text, "Kolhapuri ~extra hot~ chutney") {
		t.Fatalf("the name was changed:\n%s", share.Text)
	}
	if !strings.Contains(share.Text, "*Mild chutney*") {
		t.Fatalf("a plain name was not emphasised:\n%s", share.Text)
	}
	if !hasProblem(share.Problems, ProblemFormattingInName) {
		t.Fatalf("the formatting character was not reported: %s", problemCodes(share.Problems))
	}
}

func TestAProductWithNoPackSizeStillReadsProperly(t *testing.T) {
	catalogue := Catalogue{Products: []Product{
		{Code: "CAKE", Name: "Birthday cake", Unit: "cake", Price: 95000,
			Availability: MadeToOrder, NoticeDays: 3, Note: "message for sizes"},
	}}.Normalise()

	share := ShareCatalogue(testShop(), catalogue)
	if !strings.Contains(share.Text, "*Birthday cake* — ₹950.00 a cake — 3 days' notice — message for sizes") {
		t.Fatalf("a product with no pack size reads:\n%s", share.Text)
	}
	if strings.Contains(share.Text, "()") {
		t.Fatalf("an empty bracket is in the text:\n%s", share.Text)
	}
}
