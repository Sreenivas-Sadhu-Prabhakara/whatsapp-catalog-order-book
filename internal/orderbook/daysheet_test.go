package orderbook

import "testing"

// dayOrders is a day with five orders on it: two customers wanting the same
// thing, one wanting something else, one cancelled, and one on another day.
func dayOrders(on Date) []Order {
	return []Order{
		{
			Ref: "ORD-0001", PlacedOn: on.AddDays(-2),
			Customer:   Customer{Name: "Meera", Phone: "9876543211", Address: "Kothrud"},
			Fulfilment: Delivery, Wanted: on, Slot: SlotEvening,
			Lines: []Line{
				{Code: "MANGO-500", Name: "Mango pickle", Pack: "500 g", Unit: "jar", Qty: Whole(2), UnitPrice: 32000},
				{Code: "LIME-400", Name: "Lime pickle", Pack: "400 g", Unit: "jar", Qty: Whole(1), UnitPrice: 26000},
			},
			DeliveryFee: 4000, Paid: 50000, Status: Confirmed,
		},
		{
			Ref: "ORD-0002", PlacedOn: on.AddDays(-1),
			Customer:   Customer{Name: "Sandeep", Phone: "9876543212"},
			Fulfilment: Pickup, Wanted: on, Slot: SlotMorning,
			Lines: []Line{
				{Code: "MANGO-500", Name: "Mango pickle", Pack: "500 g", Unit: "jar", Qty: Whole(12), UnitPrice: 32000},
			},
			Status: Packed,
		},
		{
			Ref: "ORD-0003", PlacedOn: on.AddDays(-1),
			Customer:   Customer{Name: "Priya", Phone: "9876543213"},
			Fulfilment: Pickup, Wanted: on, Slot: SlotMorning,
			Lines: []Line{
				{Code: "PODI-200", Name: "Idli podi", Pack: "200 g", Unit: "packet", Qty: Whole(3), UnitPrice: 18000},
			},
			Paid: 54000, Status: Taken,
		},
		{
			Ref: "ORD-0004", PlacedOn: on.AddDays(-3),
			Customer:   Customer{Name: "Anil", Phone: "9876543214"},
			Fulfilment: Pickup, Wanted: on, Slot: SlotAfternoon,
			Lines: []Line{
				{Code: "MANGO-500", Name: "Mango pickle", Pack: "500 g", Unit: "jar", Qty: Whole(50), UnitPrice: 32000},
			},
			Status: Cancelled,
		},
		{
			Ref: "ORD-0005", PlacedOn: on,
			Customer:   Customer{Name: "Latha", Phone: "9876543215"},
			Fulfilment: Pickup, Wanted: on.AddDays(3), Slot: SlotMorning,
			Lines: []Line{
				{Code: "LIME-400", Name: "Lime pickle", Pack: "400 g", Unit: "jar", Qty: Whole(4), UnitPrice: 26000},
			},
			Status: Taken,
		},
	}
}

func TestADaySheetAddsUpWhatHasToBeMadeAcrossEveryOrder(t *testing.T) {
	on := Date{2026, 11, 26}
	sheet := Day(testCatalogue(), dayOrders(on), on)

	if sheet.On != on {
		t.Fatalf("the sheet is for %s", sheet.On)
	}
	if sheet.Weekday != "Thursday" {
		t.Fatalf("26 November 2026 is a %s by this sheet", sheet.Weekday)
	}

	// Three live orders for the day, one cancelled, one on another day.
	if sheet.Totals.Orders != 3 {
		t.Fatalf("%d orders on the day, want 3", sheet.Totals.Orders)
	}
	if sheet.Totals.Cancelled != 1 {
		t.Fatalf("%d cancelled, want 1", sheet.Totals.Cancelled)
	}
	if len(sheet.Cancelled) != 1 || sheet.Cancelled[0].Ref != "ORD-0004" {
		t.Fatalf("the cancelled list is %+v", sheet.Cancelled)
	}

	// Mango across two orders: 2 and 12, which is 14 jars. The 50 on the
	// cancelled order is not in it.
	byCode := map[string]MakeLine{}
	for _, m := range sheet.Make {
		byCode[m.Code] = m
	}
	mango, ok := byCode["MANGO-500"]
	if !ok {
		t.Fatalf("the make list has no mango in it: %+v", sheet.Make)
	}
	if mango.Qty != Whole(14) {
		t.Fatalf("the day needs %s jars of mango, want 14", mango.Qty)
	}
	if mango.Orders != 2 {
		t.Fatalf("the mango is on %d orders, want 2", mango.Orders)
	}

	lime, ok := byCode["LIME-400"]
	if !ok {
		t.Fatal("the make list has no lime in it")
	}
	if lime.Qty != Whole(1) {
		t.Fatalf("the day needs %s jars of lime, want 1 — the 4 on the later order must not be in it", lime.Qty)
	}

	// Something on the day's list that the catalogue now says is sold out is
	// the shop's most urgent problem that morning.
	podi, ok := byCode["PODI-200"]
	if !ok {
		t.Fatal("the make list has no idli podi in it")
	}
	if !podi.SoldOut {
		t.Fatal("the day needs something marked sold out and the sheet does not say so")
	}

	if len(sheet.Make) != 3 {
		t.Fatalf("%d things to make, want 3: %+v", len(sheet.Make), sheet.Make)
	}
}

func TestADaySheetsMoneyExcludesACancelledOrder(t *testing.T) {
	on := Date{2026, 11, 26}
	sheet := Day(testCatalogue(), dayOrders(on), on)

	// ORD-0001: 2×320 + 1×260 + 40 delivery = 940, paid 500, owing 440.
	// ORD-0002: 12×320 = 3,840, nothing paid.
	// ORD-0003: 3×180 = 540, paid 540, owing nothing.
	// ORD-0004 is cancelled and in none of it.
	wantValue := Money(94000 + 384000 + 54000)
	wantCollected := Money(50000 + 0 + 54000)
	wantToCollect := wantValue - wantCollected

	if sheet.Totals.Value != wantValue {
		t.Errorf("the day is worth %s, want %s", sheet.Totals.Value, wantValue)
	}
	if sheet.Totals.Collected != wantCollected {
		t.Errorf("%s has been collected, want %s", sheet.Totals.Collected, wantCollected)
	}
	if sheet.Totals.ToCollect != wantToCollect {
		t.Errorf("%s is still to collect, want %s", sheet.Totals.ToCollect, wantToCollect)
	}
	if sheet.Totals.Items != Whole(3)+Whole(12)+Whole(3) {
		t.Errorf("%s items on the day", sheet.Totals.Items)
	}
}

func TestADaysOrdersComeOutInTheOrderTheyGoOut(t *testing.T) {
	on := Date{2026, 11, 26}
	sheet := Day(testCatalogue(), dayOrders(on), on)

	var slots []Slot
	for _, row := range sheet.Orders {
		slots = append(slots, row.Slot)
	}
	// Two in the morning, then the evening one. The cancelled afternoon order
	// is not among them.
	want := []Slot{SlotMorning, SlotMorning, SlotEvening}
	if len(slots) != len(want) {
		t.Fatalf("the day runs %v, want %v", slots, want)
	}
	for i := range want {
		if slots[i] != want[i] {
			t.Fatalf("the day runs %v, want %v", slots, want)
		}
	}
	// Within a part of the day, by reference.
	if sheet.Orders[0].Ref != "ORD-0002" || sheet.Orders[1].Ref != "ORD-0003" {
		t.Fatalf("the two morning orders are %s then %s", sheet.Orders[0].Ref, sheet.Orders[1].Ref)
	}
}

func TestTheMakeListReadsAlphabeticallyAndTheSheetIsEmptyWhenNothingIsOn(t *testing.T) {
	on := Date{2026, 11, 26}
	sheet := Day(testCatalogue(), dayOrders(on), on)

	for i := 1; i < len(sheet.Make); i++ {
		if sheet.Make[i-1].Name > sheet.Make[i].Name {
			t.Fatalf("the make list is not in order: %q before %q", sheet.Make[i-1].Name, sheet.Make[i].Name)
		}
	}

	empty := Day(testCatalogue(), dayOrders(on), Date{2026, 11, 27})
	if empty.Totals.Orders != 0 || len(empty.Make) != 0 || empty.Totals.Value != 0 {
		t.Fatalf("a day with nothing on it reads %+v", empty.Totals)
	}
	if empty.Make == nil || empty.Orders == nil {
		t.Fatal("an empty day should carry empty lists, not nothing at all")
	}

	nowhere := Day(testCatalogue(), dayOrders(on), Date{})
	if nowhere.Totals.Orders != 0 {
		t.Fatal("orders were counted against no day at all")
	}
}

func TestTheSameProductInTwoPackSizesIsTwoThingsToMake(t *testing.T) {
	on := Date{2026, 11, 26}
	orders := []Order{{
		Ref: "ORD-0001", PlacedOn: on, Customer: Customer{Name: "A", Phone: "9876543211"},
		Fulfilment: Pickup, Wanted: on,
		Lines: []Line{
			{Code: "MANGO-500", Name: "Mango pickle", Pack: "500 g", Unit: "jar", Qty: Whole(2), UnitPrice: 32000},
			{Code: "MANGO-1000", Name: "Mango pickle", Pack: "1 kg", Unit: "jar", Qty: Whole(1), UnitPrice: 58000},
		},
		Status: Taken,
	}}

	sheet := Day(testCatalogue(), orders, on)
	if len(sheet.Make) != 2 {
		t.Fatalf("two pack sizes of the same thing came out as %d line(s): %+v", len(sheet.Make), sheet.Make)
	}
	if sheet.Make[0].Pack == sheet.Make[1].Pack {
		t.Fatalf("both lines say %q", sheet.Make[0].Pack)
	}
}

func TestSummariseTheWholeBook(t *testing.T) {
	today := Date{2026, 11, 26}
	orders := dayOrders(today)

	// One more: delivered and paid, and one overdue.
	orders = append(orders,
		Order{
			Ref: "ORD-0006", PlacedOn: today.AddDays(-7),
			Customer:   Customer{Name: "Ravi", Phone: "9876543216"},
			Fulfilment: Pickup, Wanted: today.AddDays(-5),
			Lines: []Line{{Code: "MANGO-500", Name: "Mango pickle", Pack: "500 g", Unit: "jar", Qty: Whole(1), UnitPrice: 30000}},
			Paid:  30000, Status: Delivered,
		},
		Order{
			Ref: "ORD-0007", PlacedOn: today.AddDays(-6),
			Customer:   Customer{Name: "Usha", Phone: "9876543217"},
			Fulfilment: Pickup, Wanted: today.AddDays(-2),
			Lines:  []Line{{Code: "LIME-400", Name: "Lime pickle", Pack: "400 g", Unit: "jar", Qty: Whole(1), UnitPrice: 26000}},
			Status: Confirmed,
		},
		Order{
			Ref: "ORD-0008", PlacedOn: today,
			Customer:   Customer{Name: "Kiran", Phone: "9876543218"},
			Fulfilment: Pickup,
			Lines:      []Line{{Code: "LIME-400", Name: "Lime pickle", Pack: "400 g", Unit: "jar", Qty: Whole(1), UnitPrice: 26000}},
			Status:     Taken,
		},
	)

	book := Summarise(testCatalogue(), testShop(), orders, today)

	if book.Orders != 8 {
		t.Fatalf("%d orders in the book, want 8", book.Orders)
	}
	// Open: 0001, 0002, 0003, 0005, 0007, 0008. Not open: 0004 cancelled,
	// 0006 delivered.
	if book.Open != 6 {
		t.Fatalf("%d open orders, want 6", book.Open)
	}
	if book.Overdue != 1 {
		t.Fatalf("%d overdue, want 1 (ORD-0007 was wanted two days ago and is still open)", book.Overdue)
	}
	if book.Unscheduled != 1 {
		t.Fatalf("%d unscheduled, want 1 (ORD-0008 has no day)", book.Unscheduled)
	}

	byStatus := map[Status]StatusCount{}
	for _, row := range book.ByStatus {
		byStatus[row.Status] = row
	}
	if byStatus[Cancelled].Orders != 1 {
		t.Fatalf("%d cancelled, want 1", byStatus[Cancelled].Orders)
	}
	if byStatus[Delivered].Orders != 1 {
		t.Fatalf("%d delivered, want 1", byStatus[Delivered].Orders)
	}
	if len(book.ByStatus) != len(Statuses()) {
		t.Fatalf("the book reports %d statuses, want %d", len(book.ByStatus), len(Statuses()))
	}

	// A cancelled order is in no money figure.
	var want Money
	for _, o := range orders {
		if o.Status.Normalise() != Cancelled {
			want += o.Add().Total
		}
	}
	if book.Value != want {
		t.Fatalf("the book is worth %s, want %s", book.Value, want)
	}

	// The days with work on them come out in order, earliest first.
	if len(book.Days) < 2 {
		t.Fatalf("%d days with work on them", len(book.Days))
	}
	for i := 1; i < len(book.Days); i++ {
		if !book.Days[i-1].On.Before(book.Days[i].On) {
			t.Fatalf("the days are out of order: %s then %s", book.Days[i-1].On, book.Days[i].On)
		}
	}
	if !book.Days[0].Past {
		t.Fatalf("the first day, %s, is in the past and is not marked so", book.Days[0].On)
	}

	// The order whose line is for something sold out is counted as needing
	// attention.
	if book.WithProblems < 1 {
		t.Fatalf("%d orders with problems, want at least 1 (ORD-0003 is for something sold out)", book.WithProblems)
	}
}

func TestAnEmptyBookSummarisesToNothing(t *testing.T) {
	book := Summarise(testCatalogue(), testShop(), nil, Date{2026, 11, 26})
	if book.Orders != 0 || book.Open != 0 || book.Value != 0 || book.ToCollect != 0 {
		t.Fatalf("an empty book reads %+v", book)
	}
	if book.Days == nil || book.ByStatus == nil {
		t.Fatal("an empty book should carry empty lists, not nothing at all")
	}
	if len(book.ByStatus) != len(Statuses()) {
		t.Fatalf("an empty book reports %d statuses", len(book.ByStatus))
	}
}
