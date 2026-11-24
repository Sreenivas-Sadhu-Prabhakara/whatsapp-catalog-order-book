package orderbook

import (
	"sort"
	"strings"
)

// DayOrder is one order as it appears on a day's sheet.
type DayOrder struct {
	Ref      string `json:"ref"`
	Customer string `json:"customer"`
	Slot     Slot   `json:"slot"`
	SlotName string `json:"slotName"`
	Where    string `json:"where"`
	Status   Status `json:"status"`
	Total    Money  `json:"total"`
	Paid     Money  `json:"paid"`
	Balance  Money  `json:"balance"`
	Items    Qty    `json:"items"`
	Blocked  bool   `json:"blocked"`
}

// MakeLine is one product and how much of it the whole day needs. This is the
// list a home kitchen actually works from: not five orders to read through, but
// "fourteen jars of mango and six boxes of laddu", which is one morning's work
// written down once.
type MakeLine struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Pack   string `json:"pack"`
	Unit   string `json:"unit"`
	Qty    Qty    `json:"qty"`
	Orders int    `json:"orders"`
	// SoldOut is true where the day's list needs something the catalogue now
	// says is sold out — which is the shop's most urgent problem that morning.
	SoldOut bool `json:"soldOut"`
}

// DayTotals is the money and the count for a day.
type DayTotals struct {
	Orders    int   `json:"orders"`
	Cancelled int   `json:"cancelled"`
	Items     Qty   `json:"items"`
	Value     Money `json:"value"`
	Collected Money `json:"collected"`
	ToCollect Money `json:"toCollect"`
}

// DaySheet is everything a shop needs for one day: what to make, who it goes
// to, and what to collect.
type DaySheet struct {
	On        Date       `json:"on"`
	Weekday   string     `json:"weekday"`
	Pretty    string     `json:"pretty"`
	Orders    []DayOrder `json:"orders"`
	Make      []MakeLine `json:"make"`
	Totals    DayTotals  `json:"totals"`
	Cancelled []DayOrder `json:"cancelled"`
}

// Day builds the sheet for one day.
//
// A cancelled order is counted and shown, and is in neither the make list nor
// the money: a shop that makes a cancelled order's jars has lost the jars, and
// a shop whose day's takings include a cancelled order goes looking that
// evening for money nobody owes it.
func Day(c Catalogue, orders []Order, on Date) DaySheet {
	sheet := DaySheet{
		On:        on,
		Weekday:   on.Weekday(),
		Pretty:    on.Pretty(),
		Orders:    []DayOrder{},
		Make:      []MakeLine{},
		Cancelled: []DayOrder{},
	}
	if on.IsZero() {
		return sheet
	}

	type tally struct {
		line MakeLine
		seen map[string]bool
	}
	making := map[string]*tally{}
	var order []string

	for _, o := range orders {
		if o.Wanted != on {
			continue
		}
		o = o.Normalise()
		totals := o.Add()

		row := DayOrder{
			Ref:      o.Ref,
			Customer: o.Customer.Name,
			Slot:     o.Slot,
			SlotName: o.Slot.Name(),
			Where:    o.Fulfilment.Name(),
			Status:   o.Status,
			Total:    totals.Total,
			Paid:     totals.Paid,
			Balance:  totals.Balance,
			Items:    totals.Items,
		}

		if o.Status.Normalise() == Cancelled {
			sheet.Cancelled = append(sheet.Cancelled, row)
			sheet.Totals.Cancelled++
			continue
		}

		sheet.Orders = append(sheet.Orders, row)
		sheet.Totals.Orders++
		sheet.Totals.Items += totals.Items
		sheet.Totals.Value += totals.Total
		sheet.Totals.Collected += totals.Paid
		sheet.Totals.ToCollect += totals.Balance

		for _, l := range o.Lines {
			if l.Qty <= 0 {
				continue
			}
			key := l.Code + "\x00" + l.Name + "\x00" + l.Pack
			entry, ok := making[key]
			if !ok {
				product, found := c.Find(l.Code)
				entry = &tally{
					line: MakeLine{
						Code:    l.Code,
						Name:    nameOr(l.Name, l.Code),
						Pack:    l.Pack,
						Unit:    l.Unit,
						SoldOut: found && product.Availability.Normalise() == SoldOut,
					},
					seen: map[string]bool{},
				}
				making[key] = entry
				order = append(order, key)
			}
			entry.line.Qty += l.Qty
			if !entry.seen[o.Ref] {
				entry.seen[o.Ref] = true
				entry.line.Orders++
			}
		}
	}

	for _, key := range order {
		sheet.Make = append(sheet.Make, making[key].line)
	}

	// The make list reads alphabetically, because that is how a kitchen walks
	// its own shelves; the orders read through the day, because that is the
	// sequence they have to go out in.
	sort.SliceStable(sheet.Make, func(i, j int) bool {
		a, b := sheet.Make[i], sheet.Make[j]
		if !strings.EqualFold(a.Name, b.Name) {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.Pack < b.Pack
	})
	sort.SliceStable(sheet.Orders, func(i, j int) bool {
		a, b := sheet.Orders[i], sheet.Orders[j]
		if a.Slot.order() != b.Slot.order() {
			return a.Slot.order() < b.Slot.order()
		}
		return a.Ref < b.Ref
	})

	return sheet
}

// StatusCount is how many orders are at one stage.
type StatusCount struct {
	Status Status `json:"status"`
	Name   string `json:"name"`
	Orders int    `json:"orders"`
	Value  Money  `json:"value"`
}

// Book is the state of the whole order book: where every order has got to, what
// is outstanding, and which days have work on them.
type Book struct {
	Orders    int           `json:"orders"`
	Open      int           `json:"open"`
	ByStatus  []StatusCount `json:"byStatus"`
	Value     Money         `json:"value"`
	Collected Money         `json:"collected"`
	ToCollect Money         `json:"toCollect"`
	// Overdue is open orders whose day has already passed. It is the figure a
	// shop in a chat thread never has, and the reason an order gets lost.
	Overdue int `json:"overdue"`
	// Unscheduled is open orders with no day set at all.
	Unscheduled int `json:"unscheduled"`
	// Days is the days with work on them, from today forward.
	Days []DayLoad `json:"days"`
	// WithProblems is open orders the tool will not build a customer message
	// from until something is fixed.
	WithProblems int `json:"withProblems"`
}

// DayLoad is one day with work on it.
type DayLoad struct {
	On        Date   `json:"on"`
	Weekday   string `json:"weekday"`
	Pretty    string `json:"pretty"`
	Orders    int    `json:"orders"`
	Items     Qty    `json:"items"`
	Value     Money  `json:"value"`
	ToCollect Money  `json:"toCollect"`
	// Past is true where the day has already gone by and the orders on it are
	// still open.
	Past bool `json:"past"`
}

// Summarise adds the whole book up, as of a given day.
func Summarise(c Catalogue, shop Shop, orders []Order, today Date) Book {
	book := Book{ByStatus: []StatusCount{}, Days: []DayLoad{}}

	counts := map[Status]*StatusCount{}
	for _, status := range Statuses() {
		counts[status] = &StatusCount{Status: status, Name: status.Name()}
	}

	loads := map[Date]*DayLoad{}
	var dayOrder []Date

	for _, o := range orders {
		o = o.Normalise()
		totals := o.Add()
		status := o.Status.Normalise()

		book.Orders++
		counts[status].Orders++
		counts[status].Value += totals.Total

		if status == Cancelled {
			continue
		}

		book.Value += totals.Total
		book.Collected += totals.Paid

		if !status.Open() {
			// Delivered: still counted in the value, and still owed if it was
			// not paid for.
			book.ToCollect += totals.Balance
			continue
		}

		book.Open++
		book.ToCollect += totals.Balance
		if o.Blocked(c, shop) {
			book.WithProblems++
		}

		switch {
		case o.Wanted.IsZero():
			book.Unscheduled++
		default:
			if o.Wanted.Before(today) {
				book.Overdue++
			}
			load, ok := loads[o.Wanted]
			if !ok {
				load = &DayLoad{
					On:      o.Wanted,
					Weekday: o.Wanted.Weekday(),
					Pretty:  o.Wanted.Pretty(),
					Past:    o.Wanted.Before(today),
				}
				loads[o.Wanted] = load
				dayOrder = append(dayOrder, o.Wanted)
			}
			load.Orders++
			load.Items += totals.Items
			load.Value += totals.Total
			load.ToCollect += totals.Balance
		}
	}

	for _, status := range Statuses() {
		book.ByStatus = append(book.ByStatus, *counts[status])
	}

	sort.Slice(dayOrder, func(i, j int) bool { return dayOrder[i].Before(dayOrder[j]) })
	for _, day := range dayOrder {
		book.Days = append(book.Days, *loads[day])
	}
	return book
}
