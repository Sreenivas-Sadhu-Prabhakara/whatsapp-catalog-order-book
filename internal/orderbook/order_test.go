package orderbook

import (
	"strings"
	"testing"
)

// testShop is a shop to check orders against.
func testShop() Shop {
	return Shop{
		Name: "Aaji's Kitchen", Area: "Pune", Currency: INR, DialCode: "91",
		Phone: "+91 98765 43210", PayTo: "UPI test@examplebank",
	}.Normalise()
}

// testCatalogue is a catalogue to check orders against.
func testCatalogue() Catalogue {
	return Catalogue{
		PricesChangedOn: Date{2026, 11, 15},
		Products: []Product{
			{Code: "MANGO-500", Name: "Mango pickle", Unit: "jar", Price: 32000,
				PackAmount: Whole(500), PackMeasure: ByWeight, Availability: Available},
			{Code: "LIME-400", Name: "Lime pickle", Unit: "jar", Price: 26000,
				PackAmount: Whole(400), PackMeasure: ByWeight, Availability: Available},
			{Code: "CHAKLI-250", Name: "Chakli", Unit: "packet", Price: 14000,
				PackAmount: Whole(250), PackMeasure: ByWeight, Availability: MadeToOrder,
				NoticeDays: 2, MinQty: Whole(2)},
			{Code: "PODI-200", Name: "Idli podi", Unit: "packet", Price: 18000,
				PackAmount: Whole(200), PackMeasure: ByWeight, Availability: SoldOut},
		},
	}.Normalise()
}

// soundOrder is an order with nothing wrong with it.
func soundOrder() Order {
	return Order{
		Ref:        "ORD-0001",
		PlacedOn:   Date{2026, 11, 24},
		Customer:   Customer{Name: "Meera Kulkarni", Phone: "098765 43211", Address: "Flat 3, Kothrud"},
		Fulfilment: Delivery, Wanted: Date{2026, 11, 26}, Slot: SlotMorning,
		Lines: []Line{
			LineFor(Product{Code: "MANGO-500", Name: "Mango pickle", Unit: "jar", Price: 32000,
				PackAmount: Whole(500), PackMeasure: ByWeight}, Whole(2), ""),
		},
		DeliveryFee: 4000,
		Status:      Taken,
	}.Normalise()
}

func TestAnOrderKeepsThePriceItWasTakenAtWhenTheCatalogueChanges(t *testing.T) {
	// The rule the whole book rests on. A shop puts its prices up on Monday.
	// An order taken on Sunday is still owed at Sunday's price, and a bill that
	// silently reprices itself is a bill the shop cannot defend.
	catalogue := testCatalogue()
	shop := testShop()

	mango, ok := catalogue.Find("MANGO-500")
	if !ok {
		t.Fatal("the test catalogue has no mango pickle in it")
	}

	order := Order{
		Ref: "ORD-0001", PlacedOn: Date{2026, 11, 24},
		Customer:   Customer{Name: "Meera", Phone: "9876543211"},
		Fulfilment: Pickup, Wanted: Date{2026, 11, 25},
		Lines: []Line{
			LineFor(mango, Whole(3), ""),
			LineFor(mustFind(t, catalogue, "LIME-400"), Whole(1), ""),
		},
		Status: Confirmed,
	}.Normalise()

	before := order.Add()
	if before.Subtotal != 3*32000+26000 {
		t.Fatalf("the order came to %s before the price change", before.Subtotal)
	}

	// The shop puts the mango jar up by ₹40 and the lime jar down by ₹10.
	mango.Price = 36000
	catalogue = catalogue.Upsert(mango)
	lime := mustFind(t, catalogue, "LIME-400")
	lime.Price = 25000
	catalogue = catalogue.Upsert(lime)

	after := order.Add()
	if after != before {
		t.Fatalf("the order's own figures moved with the catalogue.\n  before: %+v\n   after: %+v", before, after)
	}
	if order.Lines[0].UnitPrice != 32000 {
		t.Fatalf("the line's price became %s; it was taken at 320.00", order.Lines[0].UnitPrice)
	}
	if order.Lines[0].Amount() != 96000 {
		t.Fatalf("the line came to %s, want 960.00", order.Lines[0].Amount())
	}

	// And the change is reported, so the shopkeeper knows there is a decision
	// to make — on both lines, in both directions.
	changes := order.PriceChanges(catalogue)
	if len(changes) != 2 {
		t.Fatalf("%d price change(s) reported, want 2: %+v", len(changes), changes)
	}
	if changes[0].TakenAt != 32000 || changes[0].NowCosts != 36000 || changes[0].Difference != 4000 {
		t.Fatalf("the mango change reads %+v", changes[0])
	}
	if changes[1].TakenAt != 26000 || changes[1].NowCosts != 25000 || changes[1].Difference != -1000 {
		t.Fatalf("the lime change reads %+v", changes[1])
	}

	// It is reported as something to look at and never as something that stops
	// the order: the order is correct as it stands.
	var reported int
	for _, problem := range order.Problems(catalogue, shop) {
		if problem.Code != ProblemPriceChanged {
			continue
		}
		reported++
		if problem.Severity != Check {
			t.Fatalf("a price change was reported as %q; the order is not wrong", problem.Severity)
		}
		if !strings.Contains(problem.Message, "keeps the price it was taken at") {
			t.Fatalf("the message does not say the order keeps its price: %q", problem.Message)
		}
	}
	if reported != 2 {
		t.Fatalf("%d price changes reported as problems, want 2", reported)
	}
	if order.Blocked(catalogue, shop) {
		t.Fatal("a price change stopped the order; it must not")
	}
}

func TestDeletingAProductDoesNotChangeABillAlreadyGiven(t *testing.T) {
	catalogue := testCatalogue()
	order := soundOrder()
	before := order.Add()

	catalogue, removed := catalogue.Remove("MANGO-500")
	if !removed {
		t.Fatal("the product was not removed")
	}
	if _, found := catalogue.Find("MANGO-500"); found {
		t.Fatal("the product is still in the catalogue")
	}

	if after := order.Add(); after != before {
		t.Fatalf("the bill changed when the product was deleted.\n  before: %+v\n   after: %+v", before, after)
	}
	if order.Lines[0].Name != "Mango pickle" || order.Lines[0].Pack != "500 g" {
		t.Fatalf("the line lost its own copy of what was ordered: %+v", order.Lines[0])
	}

	// And it is reported, because the shopkeeper should know the catalogue and
	// the book have parted company.
	problems := order.Problems(catalogue, testShop())
	if !hasProblem(problems, ProblemUnknownProduct) {
		t.Fatalf("a line for a deleted product was not reported: %+v", problems)
	}
	for _, p := range problems {
		if p.Code == ProblemUnknownProduct && p.Severity != Check {
			t.Fatal("a line for a deleted product stopped the order; the line is still valid")
		}
	}
}

func TestOrderTotalsAddUpTheWayACustomerAddsThemUp(t *testing.T) {
	cases := []struct {
		name     string
		order    Order
		subtotal Money
		total    Money
		balance  Money
		lines    int
		items    Qty
	}{
		{
			name: "two lines, a delivery charge and part payment",
			order: Order{
				Lines: []Line{
					{Code: "A", Qty: Whole(2), UnitPrice: 32000},
					{Code: "B", Qty: Whole(1), UnitPrice: 26000},
				},
				DeliveryFee: 4000, Paid: 50000,
			},
			subtotal: 90000, total: 94000, balance: 44000, lines: 2, items: Whole(3),
		},
		{
			name: "a discount",
			order: Order{
				Lines:    []Line{{Code: "A", Qty: Whole(4), UnitPrice: 14000}},
				Discount: 5000,
			},
			subtotal: 56000, total: 51000, balance: 51000, lines: 1, items: Whole(4),
		},
		{
			name:     "an empty order",
			order:    Order{},
			subtotal: 0, total: 0, balance: 0, lines: 0, items: 0,
		},
		{
			name: "a line for nothing adds nothing and is not counted",
			order: Order{
				Lines: []Line{
					{Code: "A", Qty: Whole(2), UnitPrice: 32000},
					{Code: "B", Qty: 0, UnitPrice: 26000},
				},
			},
			subtotal: 64000, total: 64000, balance: 64000, lines: 1, items: Whole(2),
		},
		{
			name: "a fractional quantity",
			order: Order{
				Lines: []Line{{Code: "A", Qty: 500, UnitPrice: 30000}},
			},
			subtotal: 15000, total: 15000, balance: 15000, lines: 1, items: 500,
		},
		{
			name: "paid in full leaves nothing owing",
			order: Order{
				Lines: []Line{{Code: "A", Qty: Whole(1), UnitPrice: 32000}},
				Paid:  32000,
			},
			subtotal: 32000, total: 32000, balance: 0, lines: 1, items: Whole(1),
		},
		{
			name: "a discount larger than the bill never prints a negative total",
			order: Order{
				Lines:    []Line{{Code: "A", Qty: Whole(1), UnitPrice: 32000}},
				Discount: 50000,
			},
			subtotal: 32000, total: 0, balance: 0, lines: 1, items: Whole(1),
		},
		{
			name: "negative figures are treated as zero rather than as credits",
			order: Order{
				Lines:       []Line{{Code: "A", Qty: Whole(1), UnitPrice: 32000}},
				DeliveryFee: -4000, Discount: -1000,
			},
			subtotal: 32000, total: 32000, balance: 32000, lines: 1, items: Whole(1),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.order.Normalise().Add()
			if got.Subtotal != c.subtotal {
				t.Errorf("subtotal is %s, want %s", got.Subtotal, c.subtotal)
			}
			if got.Total != c.total {
				t.Errorf("total is %s, want %s", got.Total, c.total)
			}
			if got.Balance != c.balance {
				t.Errorf("balance is %s, want %s", got.Balance, c.balance)
			}
			if got.Lines != c.lines {
				t.Errorf("line count is %d, want %d", got.Lines, c.lines)
			}
			if got.Items != c.items {
				t.Errorf("item count is %s, want %s", got.Items, c.items)
			}
		})
	}
}

func TestTheTotalIsTheSumOfTheLinesAsPrinted(t *testing.T) {
	// Three lines that each round, so that the sum of the printed lines and the
	// rounding of the unrounded sum are different numbers. The total has to be
	// the first one, because that is the one a customer adding up the message
	// gets.
	order := Order{
		Lines: []Line{
			{Code: "A", Qty: 333, UnitPrice: 10000}, // 33.30
			{Code: "B", Qty: 333, UnitPrice: 10000}, // 33.30
			{Code: "C", Qty: 333, UnitPrice: 10000}, // 33.30
		},
	}.Normalise()

	totals := order.Add()
	var sumOfPrinted Money
	for _, l := range order.Lines {
		sumOfPrinted += l.Amount()
	}
	if totals.Subtotal != sumOfPrinted {
		t.Fatalf("the subtotal is %s and the lines add up to %s", totals.Subtotal, sumOfPrinted)
	}
	if totals.Subtotal != 9990 {
		t.Fatalf("the subtotal is %s, want 99.90", totals.Subtotal)
	}
}

func TestAnOrderIsStoppedByWhatWouldMakeItWrong(t *testing.T) {
	catalogue := testCatalogue()
	shop := testShop()

	cases := []struct {
		name   string
		change func(o *Order)
		code   string
		stops  bool
	}{
		{"a line for something sold out",
			func(o *Order) {
				o.Lines = append(o.Lines, LineFor(mustFind(nil, catalogue, "PODI-200"), Whole(1), ""))
			}, ProblemSoldOut, true},

		{"a line for no quantity",
			func(o *Order) { o.Lines[0].Qty = 0 }, ProblemZeroQuantity, true},

		{"a line for a negative quantity",
			func(o *Order) { o.Lines[0].Qty = -Whole(1) }, ProblemNegativeQuantity, true},

		{"a line taken at no price",
			func(o *Order) { o.Lines[0].UnitPrice = 0 }, ProblemFreeLine, true},

		{"a line below the smallest the shop makes",
			func(o *Order) {
				o.Lines = []Line{LineFor(mustFind(nil, catalogue, "CHAKLI-250"), Whole(1), "")}
				o.Wanted = o.PlacedOn.AddDays(3)
			}, ProblemBelowMinimum, true},

		{"an order with nothing on it",
			func(o *Order) { o.Lines = nil }, ProblemNoLines, true},

		{"a made-to-order line wanted sooner than the notice allows",
			func(o *Order) {
				o.Lines = []Line{LineFor(mustFind(nil, catalogue, "CHAKLI-250"), Whole(2), "")}
				o.Wanted = o.PlacedOn.AddDays(1)
			}, ProblemShortNotice, true},

		{"a day before the order was taken",
			func(o *Order) { o.Wanted = o.PlacedOn.AddDays(-1) }, ProblemDeliveryBefore, true},

		{"no customer name",
			func(o *Order) { o.Customer.Name = "" }, ProblemNoCustomerName, true},

		{"a delivery with no address",
			func(o *Order) { o.Customer.Address = "" }, ProblemNoAddress, true},

		{"a discount bigger than the bill",
			func(o *Order) { o.Discount = 999900 }, ProblemDiscountTooLarge, true},

		{"a negative discount",
			func(o *Order) { o.Discount = -1000 }, ProblemNegativeDiscount, true},

		{"a negative delivery charge",
			func(o *Order) { o.DeliveryFee = -1000 }, ProblemNegativeFee, true},

		{"more money than the bill",
			func(o *Order) { o.Paid = 999900 }, ProblemOverpaid, true},

		{"a negative payment",
			func(o *Order) { o.Paid = -1000 }, ProblemNegativePaid, true},

		// These are reported and do not stop the order.
		{"no phone number",
			func(o *Order) { o.Customer.Phone = "" }, ProblemUnreachable, false},

		{"a number that cannot make a link",
			func(o *Order) { o.Customer.Phone = "98765" }, ProblemUnreachable, false},

		{"a landline",
			func(o *Order) { o.Customer.Phone = "+91 22 3456 7890" }, ProblemNotMobile, false},

		{"no day set",
			func(o *Order) { o.Wanted = Date{} }, ProblemNoDeliveryDate, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			order := soundOrder()
			if order.Blocked(catalogue, shop) {
				t.Fatalf("the sound order is already stopped: %+v", order.Problems(catalogue, shop))
			}

			c.change(&order)
			order = order.Normalise()
			problems := order.Problems(catalogue, shop)

			if !hasProblem(problems, c.code) {
				t.Fatalf("%s was not reported. got: %s", c.code, problemCodes(problems))
			}
			for _, p := range problems {
				if p.Code != c.code {
					continue
				}
				want := Check
				if c.stops {
					want = Stop
				}
				if p.Severity != want {
					t.Fatalf("%s was reported as %q, want %q", c.code, p.Severity, want)
				}
				if p.Message == "" {
					t.Fatalf("%s was reported with no message", c.code)
				}
			}
			if got := order.Blocked(catalogue, shop); got != c.stops {
				t.Fatalf("the order being stopped is %v, want %v (problems: %s)", got, c.stops, problemCodes(problems))
			}
		})
	}
}

func TestAMadeToOrderLineWithEnoughNoticeIsFine(t *testing.T) {
	catalogue := testCatalogue()
	shop := testShop()
	chakli := mustFind(t, catalogue, "CHAKLI-250")

	order := soundOrder()
	order.Lines = []Line{LineFor(chakli, Whole(2), "")}

	// Two days' notice, so the second day after it was taken is the earliest.
	order.Wanted = order.PlacedOn.AddDays(2)
	if order.Blocked(catalogue, shop) {
		t.Fatalf("exactly enough notice was refused: %s", problemCodes(order.Problems(catalogue, shop)))
	}
	if got := order.EarliestPossible(catalogue); got != order.PlacedOn.AddDays(2) {
		t.Fatalf("the earliest possible day is %s", got)
	}

	// One day is not enough.
	order.Wanted = order.PlacedOn.AddDays(1)
	if !order.Blocked(catalogue, shop) {
		t.Fatal("a day's notice was accepted where two are needed")
	}
}

func TestNoticePeriodsCountOverAMonthEndAndALeapDay(t *testing.T) {
	catalogue := testCatalogue()
	shop := testShop()
	chakli := mustFind(t, catalogue, "CHAKLI-250")

	cases := []struct {
		name     string
		placed   Date
		earliest Date
	}{
		{"over a 30-day month end", Date{2026, 11, 29}, Date{2026, 12, 1}},
		{"over a year end", Date{2026, 12, 30}, Date{2027, 1, 1}},
		{"into a leap February", Date{2028, 2, 27}, Date{2028, 2, 29}},
		{"over a common February", Date{2027, 2, 27}, Date{2027, 3, 1}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			order := soundOrder()
			order.PlacedOn = c.placed
			order.Lines = []Line{LineFor(chakli, Whole(2), "")}

			if got := order.EarliestPossible(catalogue); got != c.earliest {
				t.Fatalf("ordered on %s the earliest is %s, want %s", c.placed, got, c.earliest)
			}

			order.Wanted = c.earliest
			if order.Blocked(catalogue, shop) {
				t.Fatalf("%s was refused: %s", c.earliest, problemCodes(order.Problems(catalogue, shop)))
			}

			order.Wanted = c.earliest.AddDays(-1)
			if !order.Blocked(catalogue, shop) {
				t.Fatalf("the day before the earliest, %s, was accepted", order.Wanted)
			}
		})
	}
}

func TestTheStatusGoesOneStageAtATime(t *testing.T) {
	cases := []struct {
		from, to Status
		allowed  bool
	}{
		{Taken, Taken, true},
		{Taken, Confirmed, true},
		{Confirmed, Packed, true},
		{Packed, OutForDelivery, true},
		{OutForDelivery, Delivered, true},
		{Confirmed, Taken, true},
		{Delivered, OutForDelivery, true},

		{Taken, Packed, false},
		{Taken, Delivered, false},
		{Confirmed, Delivered, false},
		{Delivered, Taken, false},
		{Delivered, Packed, false},

		{Taken, Cancelled, true},
		{Confirmed, Cancelled, true},
		{OutForDelivery, Cancelled, true},
		{Delivered, Cancelled, false},

		{Cancelled, Taken, true},
		{Cancelled, Confirmed, false},
		{Cancelled, Delivered, false},
		{Cancelled, Cancelled, true},
	}

	for _, c := range cases {
		err := c.from.CanMoveTo(c.to)
		if c.allowed && err != nil {
			t.Errorf("%s to %s was refused: %v", c.from, c.to, err)
		}
		if !c.allowed {
			if err == nil {
				t.Errorf("%s to %s was allowed", c.from, c.to)
			} else if err.Error() == "" {
				t.Errorf("%s to %s was refused with no reason", c.from, c.to)
			}
		}
	}
}

func TestStatusOpennessAndNames(t *testing.T) {
	open := map[Status]bool{Taken: true, Confirmed: true, Packed: true, OutForDelivery: true, Delivered: false, Cancelled: false}
	for status, want := range open {
		if got := status.Open(); got != want {
			t.Errorf("%s open = %v, want %v", status, got, want)
		}
		if status.Name() == "" {
			t.Errorf("%s has no name", status)
		}
	}
	if Status("rubbish").Normalise() != Taken {
		t.Fatal("an unknown status should come back as taken")
	}
	if Status("out-for-delivery").Normalise() != OutForDelivery {
		t.Fatal("the hyphenated form of out for delivery was not read")
	}
}

func TestLineForTakesACopyOfEverythingThatCanMove(t *testing.T) {
	product := Product{
		Code: " mango 500 ", Name: " Mango pickle ", Unit: "jar", Price: 32000,
		PackAmount: Whole(500), PackMeasure: ByWeight, Availability: Available,
	}
	line := LineFor(product, Whole(2), "  less oil  ")

	if line.Code != "MANGO-500" {
		t.Errorf("code is %q", line.Code)
	}
	if line.Name != "Mango pickle" {
		t.Errorf("name is %q", line.Name)
	}
	if line.Pack != "500 g" {
		t.Errorf("pack is %q", line.Pack)
	}
	if line.Unit != "jar" {
		t.Errorf("unit is %q", line.Unit)
	}
	if line.UnitPrice != 32000 {
		t.Errorf("price is %s", line.UnitPrice)
	}
	if line.Qty != Whole(2) {
		t.Errorf("quantity is %s", line.Qty)
	}
	if line.Note != "less oil" {
		t.Errorf("note is %q", line.Note)
	}
	if line.Amount() != 64000 {
		t.Errorf("amount is %s", line.Amount())
	}
	if line.Label() != "Mango pickle (500 g)" {
		t.Errorf("label is %q", line.Label())
	}
}

func TestFulfilmentNormalisesAndNeedsAnAddressOnlyForDelivery(t *testing.T) {
	if Fulfilment("DELIVERY").Normalise() != Delivery {
		t.Error("DELIVERY was not read as delivery")
	}
	if Fulfilment("").Normalise() != Pickup {
		t.Error("an unset way of fulfilment should be pickup, which needs no address")
	}

	catalogue, shop := testCatalogue(), testShop()
	order := soundOrder()
	order.Fulfilment = Pickup
	order.Customer.Address = ""
	if order.Blocked(catalogue, shop) {
		t.Fatalf("a pickup with no address was refused: %s", problemCodes(order.Problems(catalogue, shop)))
	}
}

func TestPluralsReadAsEnglish(t *testing.T) {
	cases := map[string]string{
		"jar": "jars", "box": "boxes", "packet": "packets", "tray": "trays",
		"hamper": "hampers", "dozen": "dozens", "": "units", "glass": "glasses",
		"jelly": "jellies", "dish": "dishes",
	}
	for unit, want := range cases {
		if got := plural(unit); got != want {
			t.Errorf("plural(%q) = %q, want %q", unit, got, want)
		}
	}
	if days(1) != "a day's notice" {
		t.Errorf("one day reads as %q", days(1))
	}
	if days(3) != "3 days' notice" {
		t.Errorf("three days reads as %q", days(3))
	}
}

// mustFind pulls a product out of a catalogue, failing the test where t is not
// nil and panicking in a table where it is.
func mustFind(t *testing.T, c Catalogue, code string) Product {
	p, ok := c.Find(code)
	if !ok {
		if t != nil {
			t.Helper()
			t.Fatalf("the test catalogue has no %s in it", code)
		}
		panic("the test catalogue has no " + code + " in it")
	}
	return p
}

// hasProblem reports whether a code is among the problems.
func hasProblem(problems []Problem, code string) bool {
	for _, p := range problems {
		if p.Code == code {
			return true
		}
	}
	return false
}

// problemCodes lists the codes, for a failure message.
func problemCodes(problems []Problem) string {
	if len(problems) == 0 {
		return "(none)"
	}
	codes := make([]string, 0, len(problems))
	for _, p := range problems {
		codes = append(codes, string(p.Severity)+":"+p.Code)
	}
	return strings.Join(codes, ", ")
}
