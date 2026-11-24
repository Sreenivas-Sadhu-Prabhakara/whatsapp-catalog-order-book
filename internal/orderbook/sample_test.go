package orderbook

import (
	"net/url"
	"strings"
	"testing"
)

func TestBothWorkedShopsHangTogether(t *testing.T) {
	today := Date{2026, 11, 24}

	for _, market := range Markets() {
		t.Run(string(market), func(t *testing.T) {
			shop, catalogue, orders := Sample(market, today)

			// The shop itself has to be usable: a name, a currency, and a
			// number a link can be built to.
			if shop.Name == "" || shop.Area == "" || shop.PayTo == "" {
				t.Fatalf("the worked shop is incomplete: %+v", shop)
			}
			number, err := shop.Number()
			if err != nil {
				t.Fatalf("the worked shop's own number is unusable: %v", err)
			}
			if err := number.Validate(); err != nil {
				t.Fatalf("the worked shop's number will not make a link: %v", err)
			}
			if market == PH && shop.Currency != PHP {
				t.Fatalf("the Philippine shop prices in %s", shop.Currency)
			}
			if market == IN && shop.Currency != INR {
				t.Fatalf("the Indian shop prices in %s", shop.Currency)
			}

			// The catalogue has to have something of each kind in it, so the
			// whole tool can be seen working.
			if len(catalogue.Products) < 5 {
				t.Fatalf("the worked catalogue has %d products", len(catalogue.Products))
			}
			kinds := map[Availability]int{}
			for _, p := range catalogue.Products {
				kinds[p.Availability]++
			}
			for _, want := range Availabilities() {
				if kinds[want] == 0 {
					t.Errorf("the worked catalogue has nothing that is %s", want)
				}
			}

			// Nothing in it may be broken.
			if problems := catalogue.Problems(); blocked(problems) {
				t.Fatalf("the worked catalogue is blocked: %s", problemCodes(problems))
			}

			// Every order's lines must name products that exist, and every
			// order must have a reference and a customer.
			if len(orders) < 3 {
				t.Fatalf("the worked book has %d orders", len(orders))
			}
			refs := map[string]bool{}
			for _, o := range orders {
				if o.Ref == "" {
					t.Fatalf("an order has no reference: %+v", o)
				}
				if refs[o.Ref] {
					t.Fatalf("two orders share the reference %s", o.Ref)
				}
				refs[o.Ref] = true

				if o.Customer.Name == "" {
					t.Errorf("%s has no customer name", o.Ref)
				}
				if _, err := NormalisePhone(o.Customer.Phone, shop.DialCode); err != nil {
					t.Errorf("%s has an unusable number: %v", o.Ref, err)
				}
				if len(o.Lines) == 0 {
					t.Errorf("%s has nothing on it", o.Ref)
				}
				for _, l := range o.Lines {
					if _, found := catalogue.Find(l.Code); !found {
						t.Errorf("%s has a line for %s, which is not in the worked catalogue", o.Ref, l.Code)
					}
					if l.Qty <= 0 || l.UnitPrice <= 0 {
						t.Errorf("%s has a line for %s at %s", o.Ref, l.Qty, l.UnitPrice)
					}
				}
			}

			// There must be at least one order at several stages, and at least
			// one cancelled or delivered, so the book reads like a real one.
			stages := map[Status]int{}
			for _, o := range orders {
				stages[o.Status.Normalise()]++
			}
			if len(stages) < 3 {
				t.Errorf("the worked book has orders at only %d stage(s)", len(stages))
			}

			// Prices must be from a day in the past, so the text says
			// something.
			if catalogue.PricesChangedOn.IsZero() || !catalogue.PricesChangedOn.Before(today.AddDays(1)) {
				t.Errorf("the worked catalogue's prices are from %s", catalogue.PricesChangedOn)
			}
		})
	}
}

func TestTheWorkedShopsShareableCatalogueIsSendable(t *testing.T) {
	today := Date{2026, 11, 24}

	for _, market := range Markets() {
		t.Run(string(market), func(t *testing.T) {
			shop, catalogue, _ := Sample(market, today)
			share := ShareCatalogue(shop, catalogue)

			if blocked(share.Problems) {
				t.Fatalf("the worked catalogue cannot be shared: %s", problemCodes(share.Problems))
			}
			if share.Listed < 4 {
				t.Fatalf("%d products listed", share.Listed)
			}
			if share.Link.URL == "" {
				t.Fatal("there is no link for the worked catalogue")
			}
			if !share.Link.Safe {
				t.Fatalf("the worked catalogue's link is %d characters, over the limit", share.Link.Characters)
			}
			if !share.FitsStatus {
				t.Fatalf("the worked catalogue is %d characters and does not fit a status; a worked example "+
					"should be short enough to demonstrate one", share.Characters)
			}

			parsed, err := url.Parse(share.Link.URL)
			if err != nil {
				t.Fatalf("the link does not parse: %v", err)
			}
			if got := parsed.Query().Get("text"); got != share.Text {
				t.Fatal("the worked catalogue did not survive its own link")
			}

			if !strings.Contains(share.Text, shop.Name) {
				t.Errorf("the catalogue does not name the shop:\n%s", share.Text)
			}
			if market == PH && !strings.Contains(share.Text, "₱") {
				t.Errorf("the Philippine catalogue has no peso sign in it:\n%s", share.Text)
			}
			if market == IN && !strings.Contains(share.Text, "₹") {
				t.Errorf("the Indian catalogue has no rupee sign in it:\n%s", share.Text)
			}
		})
	}
}

func TestTheWorkedOrdersProduceMessagesAndADaySheet(t *testing.T) {
	today := Date{2026, 11, 24}

	for _, market := range Markets() {
		t.Run(string(market), func(t *testing.T) {
			shop, catalogue, orders := Sample(market, today)

			// At least one order in each worked book has to produce a message
			// a shop could actually send, or the example demonstrates nothing.
			sendable := 0
			for _, o := range orders {
				for _, m := range MessagesFor(catalogue, shop, o) {
					if m.Kind != Confirmation || !m.Available {
						continue
					}
					sendable++
					if m.Text == "" {
						t.Fatalf("%s produced an empty confirmation", o.Ref)
					}
					if m.WhatsApp.URL == "" {
						t.Fatalf("%s produced no chat link", o.Ref)
					}
					parsed, err := url.Parse(m.WhatsApp.URL)
					if err != nil {
						t.Fatalf("%s produced a link that does not parse: %v", o.Ref, err)
					}
					if got := parsed.Query().Get("text"); got != m.Text {
						t.Fatalf("%s's message did not survive its link", o.Ref)
					}
					if !m.WhatsApp.Safe {
						t.Fatalf("%s's link is %d characters, over the limit", o.Ref, m.WhatsApp.Characters)
					}
				}
			}
			if sendable < 2 {
				t.Fatalf("only %d of the worked orders produce a message", sendable)
			}

			// Some day in the worked book has to have work on it.
			found := false
			for day := -5; day <= 5; day++ {
				sheet := Day(catalogue, orders, today.AddDays(day))
				if sheet.Totals.Orders > 0 && len(sheet.Make) > 0 {
					found = true
					text := DayText(shop, sheet)
					if !strings.Contains(text, "To make") {
						t.Fatalf("a day with work on it produced:\n%s", text)
					}
				}
			}
			if !found {
				t.Fatal("no day within a week of the worked example has anything on it")
			}

			// And the book summarises.
			book := Summarise(catalogue, shop, orders, today)
			if book.Orders != len(orders) {
				t.Fatalf("the book counts %d of %d orders", book.Orders, len(orders))
			}
			if book.Open == 0 {
				t.Fatal("the worked book has no open orders, so there is nothing to do in it")
			}
		})
	}
}

func TestTheWorkedOrdersShowAPriceThatHasMovedSinceAnOrderWasTaken(t *testing.T) {
	// The example exists partly to demonstrate this rule, so at least one
	// order in a worked book must have been taken at a price the catalogue no
	// longer shows.
	today := Date{2026, 11, 24}
	shop, catalogue, orders := Sample(IN, today)

	changes := 0
	for _, o := range orders {
		changes += len(o.PriceChanges(catalogue))
	}
	if changes == 0 {
		t.Fatal("no worked order was taken at a price the catalogue has since changed; " +
			"the example does not demonstrate the rule it is there to demonstrate")
	}

	// And it is reported as something to look at, never as something that
	// stops the order.
	for _, o := range orders {
		for _, p := range o.Problems(catalogue, shop) {
			if p.Code == ProblemPriceChanged && p.Severity != Check {
				t.Fatalf("%s reports a price change as %q", o.Ref, p.Severity)
			}
		}
	}
}

func TestMarketNamesAreWrittenOut(t *testing.T) {
	if MarketName(IN) != "India" {
		t.Errorf("IN reads as %q", MarketName(IN))
	}
	if MarketName(PH) != "the Philippines" {
		t.Errorf("PH reads as %q", MarketName(PH))
	}
	if MarketName("XX") != "India" {
		t.Errorf("an unknown market reads as %q", MarketName("XX"))
	}
	if len(Markets()) != 2 {
		t.Errorf("%d worked examples", len(Markets()))
	}
}
