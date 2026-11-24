package orderbook

import (
	"strings"
	"testing"
)

func TestFindAProductByACodeTypedAnyWay(t *testing.T) {
	c := testCatalogue()

	for _, typed := range []string{"MANGO-500", "mango-500", "mango 500", " Mango_500 ", "mango--500"} {
		p, ok := c.Find(typed)
		if !ok {
			t.Fatalf("%q found nothing", typed)
		}
		if p.Code != "MANGO-500" {
			t.Fatalf("%q found %q", typed, p.Code)
		}
	}

	for _, typed := range []string{"", "   ", "MANGO", "MANGO-5000", "LADDU-12"} {
		if p, ok := c.Find(typed); ok {
			t.Fatalf("%q found %q, and should have found nothing", typed, p.Code)
		}
	}
}

func TestUpsertChangesAPriceRatherThanAddingASecondProduct(t *testing.T) {
	c := testCatalogue()
	before := len(c.Products)

	mango := mustFind(t, c, "MANGO-500")
	mango.Price = 36000
	c = c.Upsert(mango)

	if len(c.Products) != before {
		t.Fatalf("the catalogue went from %d products to %d; a price change is not a new product",
			before, len(c.Products))
	}
	if got := mustFind(t, c, "MANGO-500"); got.Price != 36000 {
		t.Fatalf("the price is %s, want 360.00", got.Price)
	}

	// A code typed in another shape is still the same product.
	same := mustFind(t, c, "MANGO-500")
	same.Code = "mango 500"
	same.Price = 40000
	c = c.Upsert(same)
	if len(c.Products) != before {
		t.Fatalf("a code typed differently added a product: now %d", len(c.Products))
	}
	if got := mustFind(t, c, "MANGO-500"); got.Price != 40000 {
		t.Fatalf("the price is %s, want 400.00", got.Price)
	}

	// Something genuinely new is added.
	c = c.Upsert(Product{Code: "LADDU-12", Name: "Besan laddu", Unit: "box", Price: 48000})
	if len(c.Products) != before+1 {
		t.Fatalf("a new product did not go in: %d products", len(c.Products))
	}
}

func TestRemoveTakesAProductOffAndSaysWhetherItWasThere(t *testing.T) {
	c := testCatalogue()
	before := len(c.Products)

	c, removed := c.Remove("lime 400")
	if !removed {
		t.Fatal("the lime pickle was not removed")
	}
	if len(c.Products) != before-1 {
		t.Fatalf("%d products left, want %d", len(c.Products), before-1)
	}

	if _, removed := c.Remove("lime-400"); removed {
		t.Fatal("removing it twice said it was there the second time")
	}
	if _, removed := c.Remove(""); removed {
		t.Fatal("removing nothing said something was removed")
	}
}

func TestSellableAndSoldOutSplitTheCatalogue(t *testing.T) {
	c := testCatalogue()

	sellable := c.Sellable()
	soldOut := c.SoldOutItems()
	if len(sellable)+len(soldOut) != len(c.Products) {
		t.Fatalf("%d sellable and %d sold out do not add up to %d products",
			len(sellable), len(soldOut), len(c.Products))
	}
	for _, p := range sellable {
		if p.Availability == SoldOut {
			t.Errorf("%s is sellable and sold out", p.Code)
		}
	}
	if len(soldOut) != 1 || soldOut[0].Code != "PODI-200" {
		t.Fatalf("the sold-out list is %v", soldOut)
	}
}

func TestTwoProductsSharingACodeStopsTheCatalogue(t *testing.T) {
	// The one problem a product cannot see on its own: an order line naming
	// that code could mean either, so every bill built from it is a guess.
	c := Catalogue{Products: []Product{
		{Code: "MANGO-500", Name: "Mango pickle", Price: 32000},
		{Code: "mango 500", Name: "Mango pickle, hot", Price: 34000},
	}}.Normalise()

	problems := c.Problems()
	if !hasProblem(problems, ProblemDuplicateCode) {
		t.Fatalf("a shared code was not reported: %s", problemCodes(problems))
	}
	for _, p := range problems {
		if p.Code != ProblemDuplicateCode {
			continue
		}
		if p.Severity != Stop {
			t.Fatalf("a shared code was reported as %q", p.Severity)
		}
		for _, want := range []string{"MANGO-500", "Mango pickle", "Mango pickle, hot"} {
			if !strings.Contains(p.Message, want) {
				t.Errorf("the message does not name %q: %q", want, p.Message)
			}
		}
	}
	if !blocked(problems) {
		t.Fatal("a catalogue with a shared code is not blocked")
	}
}

func TestAnEmptyCatalogueIsReportedButDoesNotStopAnything(t *testing.T) {
	problems := Catalogue{}.Normalise().Problems()
	if !hasProblem(problems, ProblemCatalogueEmpty) {
		t.Fatalf("an empty catalogue was not reported: %s", problemCodes(problems))
	}
	if blocked(problems) {
		t.Fatal("an empty catalogue stops the tool; a shop setting up has an empty one")
	}
}

func TestCatalogueProblemsPointAtTheRightRow(t *testing.T) {
	c := Catalogue{Products: []Product{
		{Code: "A", Name: "First", Price: 1000},
		{Code: "B", Name: "Second", Price: 0},
		{Code: "C", Name: "Third", Price: 3000},
	}}.Normalise()

	problems := c.Problems()
	found := false
	for _, p := range problems {
		if p.Code == ProblemNoProductPrice {
			found = true
			if p.Line != 2 {
				t.Fatalf("the unpriced product is row 2 and was reported against row %d", p.Line)
			}
		}
	}
	if !found {
		t.Fatalf("the unpriced product was not reported: %s", problemCodes(problems))
	}
}

func TestNormalisingTheCatalogueDropsAnAbandonedRow(t *testing.T) {
	c := Catalogue{Products: []Product{
		{Code: "A", Name: "First", Price: 1000},
		{},
		{Code: "", Name: "", Price: 0, Note: "   "},
		{Code: "B", Name: "Second", Price: 2000},
	}}.Normalise()

	if len(c.Products) != 2 {
		t.Fatalf("%d products survived, want 2: %+v", len(c.Products), c.Products)
	}
}

func TestShopNormalisationFillsInWhatItCan(t *testing.T) {
	s := Shop{Currency: "php", DialCode: "+0063 ", Phone: " 0917 123 4567 "}.Normalise()
	if s.Currency != PHP {
		t.Errorf("currency is %q", s.Currency)
	}
	if s.DialCode != "63" {
		t.Errorf("dial code is %q, want 63", s.DialCode)
	}
	if s.Phone != "0917 123 4567" {
		t.Errorf("phone is %q", s.Phone)
	}
	if s.HowToOrder == "" {
		t.Error("a shop with no ordering line should get a default one")
	}
	if s.Limits != DefaultLimits() {
		t.Errorf("limits are %+v", s.Limits)
	}

	// With no dial code at all, the currency decides.
	if got := (Shop{Currency: PHP}).Normalise().DialCode; got != "63" {
		t.Errorf("a peso shop with no dial code got %q, want 63", got)
	}
	if got := (Shop{}).Normalise().DialCode; got != "91" {
		t.Errorf("a shop with nothing set got dial code %q, want 91", got)
	}

	number, err := s.Number()
	if err != nil {
		t.Fatalf("the shop's own number was refused: %v", err)
	}
	if number != "639171234567" {
		t.Fatalf("the shop's number became %q", number)
	}
}
