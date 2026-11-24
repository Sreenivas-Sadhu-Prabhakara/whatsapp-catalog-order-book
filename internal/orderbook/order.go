package orderbook

import (
	"fmt"
	"strings"
)

// Status is where an order has got to.
type Status string

const (
	// Taken is written down and nothing more.
	Taken Status = "taken"
	// Confirmed is agreed with the customer.
	Confirmed Status = "confirmed"
	// Packed is made and packed.
	Packed Status = "packed"
	// OutForDelivery is on its way, or waiting to be collected.
	OutForDelivery Status = "out"
	// Delivered is handed over.
	Delivered Status = "delivered"
	// Cancelled is off. It stays in the book: a cancelled order is a thing
	// that happened, and a book that forgets them cannot tell the shopkeeper
	// why a Tuesday was quiet.
	Cancelled Status = "cancelled"
)

// Statuses are the stages in the order they happen, with cancelled last.
func Statuses() []Status {
	return []Status{Taken, Confirmed, Packed, OutForDelivery, Delivered, Cancelled}
}

// Normalise brings an unknown status back to the first one.
func (s Status) Normalise() Status {
	switch Status(strings.ToLower(strings.TrimSpace(string(s)))) {
	case Confirmed:
		return Confirmed
	case Packed:
		return Packed
	case OutForDelivery, "out-for-delivery", "sent":
		return OutForDelivery
	case Delivered, "done", "complete":
		return Delivered
	case Cancelled, "canceled":
		return Cancelled
	default:
		return Taken
	}
}

// Name is the status written out.
func (s Status) Name() string {
	switch s.Normalise() {
	case Confirmed:
		return "confirmed"
	case Packed:
		return "packed"
	case OutForDelivery:
		return "out for delivery"
	case Delivered:
		return "delivered"
	case Cancelled:
		return "cancelled"
	default:
		return "taken"
	}
}

// Open reports whether the order is still work to do.
func (s Status) Open() bool {
	switch s.Normalise() {
	case Delivered, Cancelled:
		return false
	default:
		return true
	}
}

// stage is how far along the status is, for deciding what may follow it.
func (s Status) stage() int {
	switch s.Normalise() {
	case Taken:
		return 0
	case Confirmed:
		return 1
	case Packed:
		return 2
	case OutForDelivery:
		return 3
	case Delivered:
		return 4
	default:
		return -1
	}
}

// CanMoveTo reports whether an order may go from this status to another, and
// says why not where it may not.
//
// The rule is deliberately narrow: forward one stage at a time, back one stage
// at a time, cancelled from anywhere that is not already delivered, and a
// cancelled order reopened only to the beginning. A book where any status can
// become any other is a book where a delivered order quietly goes back to
// unpacked and the day's takings stop adding up.
func (s Status) CanMoveTo(next Status) error {
	from, to := s.Normalise(), next.Normalise()
	if from == to {
		return nil
	}

	switch {
	case to == Cancelled:
		if from == Delivered {
			return fmt.Errorf("this order has been delivered; it cannot be cancelled, only refunded by hand")
		}
		return nil

	case from == Cancelled:
		if to != Taken {
			return fmt.Errorf("a cancelled order comes back as taken, not as %s", to.Name())
		}
		return nil

	case from == Delivered:
		if to == OutForDelivery {
			return nil
		}
		return fmt.Errorf("a delivered order cannot go back to %s", to.Name())

	default:
		step := to.stage() - from.stage()
		if step == 1 || step == -1 {
			return nil
		}
		return fmt.Errorf("an order goes from %s to %s one stage at a time; %s is %d stages away",
			from.Name(), to.Name(), to.Name(), abs(step))
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Fulfilment is how the order reaches the customer.
type Fulfilment string

const (
	// Delivery is taken to the customer.
	Delivery Fulfilment = "delivery"
	// Pickup is collected from the shop.
	Pickup Fulfilment = "pickup"
)

// Fulfilments are both ways.
func Fulfilments() []Fulfilment { return []Fulfilment{Delivery, Pickup} }

// Normalise brings an unknown way back to pickup, which needs no address.
func (f Fulfilment) Normalise() Fulfilment {
	if Fulfilment(strings.ToLower(strings.TrimSpace(string(f)))) == Delivery {
		return Delivery
	}
	return Pickup
}

// Name is the way written out.
func (f Fulfilment) Name() string {
	if f.Normalise() == Delivery {
		return "delivery"
	}
	return "pickup"
}

// Customer is who the order is for. A name and a number is all a shop keeps,
// plus an address where something is being delivered. There is nothing here a
// shop does not already have in its chat thread, and none of it leaves the
// machine.
type Customer struct {
	Name string `json:"name"`
	// Phone is as the shopkeeper typed it. It is normalised when a link is
	// built, and kept as typed so that what is on screen is what was entered.
	Phone string `json:"phone"`
	// Email is an alternative for a customer who does not use a chat app.
	Email string `json:"email"`
	// Address is where a delivery goes, in the one block a rider needs.
	Address string `json:"address"`
}

// Normalise trims the customer's details.
func (c Customer) Normalise() Customer {
	c.Name = strings.TrimSpace(c.Name)
	c.Phone = strings.TrimSpace(c.Phone)
	c.Email = strings.TrimSpace(c.Email)
	c.Address = strings.TrimSpace(c.Address)
	return c
}

// Line is one thing on one order, at the price it was taken at.
//
// Every field but the quantity and the note is a copy taken from the catalogue
// at the moment the line was added, and that is the whole point. A shop puts
// its prices up. An order taken last week at last week's price is still owed at
// last week's price, and a bill that silently reprices itself when the
// catalogue changes is a bill the shop cannot defend. So the line carries its
// own copy of the name, the pack, the unit and above all the price, and nothing
// that happens to the catalogue afterwards can move it.
type Line struct {
	// Code is which product, for matching back to the catalogue.
	Code string `json:"code"`
	// Name is the name as it was when the order was taken.
	Name string `json:"name"`
	// Pack is the pack size as it was when the order was taken.
	Pack string `json:"pack"`
	// Unit is what one of it was called when the order was taken.
	Unit string `json:"unit"`
	// Qty is how many.
	Qty Qty `json:"qty"`
	// UnitPrice is the price this line was taken at, and is never recalculated.
	UnitPrice Money `json:"unitPrice"`
	// Note is anything the customer asked for on this line.
	Note string `json:"note"`
}

// Normalise tidies a line.
func (l Line) Normalise() Line {
	l.Code = NormaliseCode(l.Code)
	l.Name = strings.TrimSpace(l.Name)
	l.Pack = strings.TrimSpace(l.Pack)
	l.Unit = strings.TrimSpace(l.Unit)
	l.Note = strings.TrimSpace(l.Note)
	return l
}

// Amount is what this line comes to: the quantity times the price it was taken
// at, rounded once.
func (l Line) Amount() Money { return l.Qty.Times(l.UnitPrice) }

// Label is the line's product as text, with the pack size where there is one.
func (l Line) Label() string {
	if l.Pack != "" {
		return l.Name + " (" + l.Pack + ")"
	}
	return l.Name
}

// LineFor makes an order line from a product as it stands now, taking the copy
// of the price that the line will keep for the rest of its life.
func LineFor(p Product, quantity Qty, note string) Line {
	p = p.Normalise()
	return Line{
		Code:      p.Code,
		Name:      p.Name,
		Pack:      p.PackLabel(),
		Unit:      p.Unit,
		Qty:       quantity,
		UnitPrice: p.Price,
		Note:      strings.TrimSpace(note),
	}.Normalise()
}

// Order is one order in the book.
type Order struct {
	// Ref is what the shop and the customer both call this order.
	Ref string `json:"ref"`
	// PlacedOn is the day it was taken, which is what the notice period runs
	// from.
	PlacedOn Date `json:"placedOn"`
	// Customer is who it is for.
	Customer Customer `json:"customer"`
	// Fulfilment is delivery or pickup.
	Fulfilment Fulfilment `json:"fulfilment"`
	// Wanted is the day the customer wants it.
	Wanted Date `json:"wanted"`
	// Slot is the part of that day.
	Slot Slot `json:"slot"`
	// Lines are what was ordered, each at the price it was taken at.
	Lines []Line `json:"lines"`
	// DeliveryFee is what is charged for bringing it.
	DeliveryFee Money `json:"deliveryFee"`
	// Discount is taken off the bill, as an amount and not a percentage, so
	// that the figure on the bill is the figure that was agreed.
	Discount Money `json:"discount"`
	// DiscountReason says why, because a discount with no reason becomes the
	// new price.
	DiscountReason string `json:"discountReason"`
	// Paid is how much has been received.
	Paid Money `json:"paid"`
	// PaymentNote is how it was paid, in the shop's own words. This tool takes
	// no payment and checks none: it records what the shopkeeper says arrived.
	PaymentNote string `json:"paymentNote"`
	// Status is where the order has got to.
	Status Status `json:"status"`
	// Note is anything else about the order.
	Note string `json:"note"`
}

// Normalise tidies an order and everything on it.
func (o Order) Normalise() Order {
	o.Ref = strings.TrimSpace(o.Ref)
	o.Customer = o.Customer.Normalise()
	o.Fulfilment = o.Fulfilment.Normalise()
	o.Slot = o.Slot.Normalise()
	o.Status = o.Status.Normalise()
	o.DiscountReason = strings.TrimSpace(o.DiscountReason)
	o.PaymentNote = strings.TrimSpace(o.PaymentNote)
	o.Note = strings.TrimSpace(o.Note)

	lines := make([]Line, 0, len(o.Lines))
	for _, l := range o.Lines {
		lines = append(lines, l.Normalise())
	}
	o.Lines = lines
	return o
}

// Totals is what an order comes to. Every figure is a whole number of minor
// units, and the total is the sum of the line amounts as they are printed —
// never a rounding of an unrounded sum, so a customer adding the lines up by
// hand gets the figure at the bottom.
type Totals struct {
	Lines       int   `json:"lines"`
	Items       Qty   `json:"items"`
	Subtotal    Money `json:"subtotal"`
	DeliveryFee Money `json:"deliveryFee"`
	Discount    Money `json:"discount"`
	Total       Money `json:"total"`
	Paid        Money `json:"paid"`
	Balance     Money `json:"balance"`
}

// Add is the one figure a customer asks for.
func (o Order) Add() Totals {
	t := Totals{DeliveryFee: o.DeliveryFee, Discount: o.Discount, Paid: o.Paid}
	if t.DeliveryFee < 0 {
		t.DeliveryFee = 0
	}
	if t.Discount < 0 {
		t.Discount = 0
	}

	for _, l := range o.Lines {
		if l.Qty <= 0 {
			// A line for nothing adds nothing, and is reported separately.
			continue
		}
		t.Lines++
		t.Items += l.Qty
		t.Subtotal += l.Amount()
	}

	t.Total = t.Subtotal + t.DeliveryFee - t.Discount
	if t.Total < 0 {
		// A discount larger than the bill is a mistake, reported as one. What
		// it must never do is print a bill that owes the customer money.
		t.Total = 0
	}
	t.Balance = t.Total - t.Paid
	return t
}

// PriceChange is a line whose product now costs something different from what
// the line was taken at. It is not an error and never changes the line: it is
// the shopkeeper's decision whether to honour the old price or to ring the
// customer, and the tool's job is to make sure they know there is a decision to
// make.
type PriceChange struct {
	Line       int    `json:"line"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	TakenAt    Money  `json:"takenAt"`
	NowCosts   Money  `json:"nowCosts"`
	Difference Money  `json:"difference"`
}

// PriceChanges lists every line of the order whose catalogue price has moved
// since the order was taken.
func (o Order) PriceChanges(c Catalogue) []PriceChange {
	var out []PriceChange
	for i, l := range o.Lines {
		product, found := c.Find(l.Code)
		if !found || product.Price == l.UnitPrice {
			continue
		}
		out = append(out, PriceChange{
			Line:       i + 1,
			Code:       l.Code,
			Name:       nameOr(l.Name, l.Code),
			TakenAt:    l.UnitPrice,
			NowCosts:   product.Price,
			Difference: product.Price - l.UnitPrice,
		})
	}
	return out
}

// EarliestPossible is the first day this order could be ready: the latest lead
// time of anything on it, counted from the day it was taken.
func (o Order) EarliestPossible(c Catalogue) Date {
	earliest := o.PlacedOn
	for _, l := range o.Lines {
		product, found := c.Find(l.Code)
		if !found {
			continue
		}
		if day := product.LeadTime(o.PlacedOn); day.After(earliest) {
			earliest = day
		}
	}
	return earliest
}

// Problems reports everything wrong with the order, checked against the
// catalogue as it stands.
//
// The ones that stop a message being built are the ones that would have the
// shop promise a customer something it cannot deliver, or send a bill that is
// wrong: a line for something sold out, a line for nothing, a line below the
// minimum the shop will make, a discount bigger than the bill, more money
// received than is owed. The rest are listed to look at.
func (o Order) Problems(c Catalogue, shop Shop) []Problem {
	var problems []Problem
	add := func(code, message string, severity Severity, line int) {
		problems = append(problems, Problem{Code: code, Message: message, Severity: severity, Line: line})
	}

	if len(o.Lines) == 0 {
		add(ProblemNoLines, "there is nothing on this order", Stop, 0)
	}

	for i, l := range o.Lines {
		at := i + 1
		label := nameOr(l.Name, l.Code)

		switch {
		case l.Qty < 0:
			add(ProblemNegativeQuantity, fmt.Sprintf("line %d is for %s of %s, which is less than nothing",
				at, l.Qty, label), Stop, at)
		case l.Qty == 0:
			add(ProblemZeroQuantity, fmt.Sprintf("line %d has no quantity: how many %s of %s?",
				at, plural(l.Unit), label), Stop, at)
		}

		if l.UnitPrice <= 0 {
			add(ProblemFreeLine, fmt.Sprintf("line %d was taken at no price, so %s is on this order for nothing",
				at, label), Stop, at)
		}

		product, found := c.Find(l.Code)
		if !found {
			add(ProblemUnknownProduct, fmt.Sprintf("line %d is %s, which is no longer in the catalogue; the line "+
				"keeps the name and the price it was taken at", at, label), Check, at)
			continue
		}

		if product.Availability.Normalise() == SoldOut {
			add(ProblemSoldOut, fmt.Sprintf("line %d is %s, which is marked sold out; either put it back on the "+
				"catalogue or take the line off the order", at, label), Stop, at)
		}

		if product.MinQty > 0 && l.Qty > 0 && l.Qty < product.MinQty {
			add(ProblemBelowMinimum, fmt.Sprintf("line %d is %s of %s and the smallest the shop makes is %s",
				at, l.Qty, label, product.MinQty), Stop, at)
		}

		if product.Availability.Normalise() == MadeToOrder && product.NoticeDays > 0 && !o.Wanted.IsZero() && !o.PlacedOn.IsZero() {
			earliest := product.LeadTime(o.PlacedOn)
			if o.Wanted.Before(earliest) {
				add(ProblemShortNotice, fmt.Sprintf("line %d is %s, which needs %s; ordered on %s the earliest it "+
					"can be ready is %s, and this order is wanted on %s",
					at, label, days(product.NoticeDays), o.PlacedOn, earliest, o.Wanted), Stop, at)
			}
		}
	}

	for _, change := range o.PriceChanges(c) {
		direction := "gone up to"
		if change.Difference < 0 {
			direction = "come down to"
		}
		add(ProblemPriceChanged, fmt.Sprintf("line %d was taken at %s for %s and the catalogue price has %s %s; "+
			"the order keeps the price it was taken at",
			change.Line, change.TakenAt.Format(shop.Currency), change.Name, direction,
			change.NowCosts.Format(shop.Currency)), Check, change.Line)
	}

	if o.Customer.Name == "" {
		add(ProblemNoCustomerName, "this order has no customer name on it", Stop, 0)
	}

	if o.Customer.Phone == "" {
		add(ProblemUnreachable, "there is no number for this customer, so there is no chat link — the message can "+
			"still be copied", Check, 0)
	} else if number, err := NormalisePhone(o.Customer.Phone, shop.DialCode); err != nil {
		add(ProblemUnreachable, fmt.Sprintf("the customer's number cannot be made into a chat link: %v", err), Check, 0)
	} else if !LooksMobile(number) {
		add(ProblemNotMobile, fmt.Sprintf("%s does not look like a mobile number; a chat link to a landline opens a "+
			"chat nobody reads", number.Display(shop.DialCode)), Check, 0)
	}

	if o.Fulfilment.Normalise() == Delivery && o.Customer.Address == "" {
		add(ProblemNoAddress, "this order is for delivery and has no address on it", Stop, 0)
	}

	if o.Wanted.IsZero() {
		add(ProblemNoDeliveryDate, "no day has been set for this order, so it is on no day's list", Check, 0)
	} else if !o.PlacedOn.IsZero() && o.Wanted.Before(o.PlacedOn) {
		add(ProblemDeliveryBefore, fmt.Sprintf("this order was taken on %s and is wanted on %s, which is earlier",
			o.PlacedOn, o.Wanted), Stop, 0)
	}

	totals := o.Add()

	if o.DeliveryFee < 0 {
		add(ProblemNegativeFee, "the delivery charge is less than nothing", Stop, 0)
	}
	if o.Discount < 0 {
		add(ProblemNegativeDiscount, "the discount is less than nothing; a negative discount is a charge, so put "+
			"it in the delivery charge or on a line", Stop, 0)
	}
	if o.Discount > totals.Subtotal+totals.DeliveryFee {
		add(ProblemDiscountTooLarge, fmt.Sprintf("the discount of %s is more than the %s this order comes to",
			o.Discount.Format(shop.Currency), (totals.Subtotal+totals.DeliveryFee).Format(shop.Currency)), Stop, 0)
	}
	if o.Paid < 0 {
		add(ProblemNegativePaid, "the amount received is less than nothing", Stop, 0)
	}
	if o.Paid > totals.Total {
		add(ProblemOverpaid, fmt.Sprintf("%s has been received against a bill of %s, which is %s too much",
			o.Paid.Format(shop.Currency), totals.Total.Format(shop.Currency),
			(o.Paid-totals.Total).Format(shop.Currency)), Stop, 0)
	}

	return sortProblems(problems)
}

// Blocked reports whether this order is in a state the tool will not build a
// customer message from.
func (o Order) Blocked(c Catalogue, shop Shop) bool {
	return blocked(o.Problems(c, shop))
}

// plural is the crude plural a one-line message needs.
func plural(unit string) string {
	unit = strings.TrimSpace(unit)
	switch {
	case unit == "":
		return "units"
	case strings.HasSuffix(unit, "s"), strings.HasSuffix(unit, "x"), strings.HasSuffix(unit, "ch"), strings.HasSuffix(unit, "sh"):
		return unit + "es"
	// A y after a consonant becomes ies — a jelly, two jellies — and a y after
	// a vowel does not: a tray, two trays.
	case strings.HasSuffix(unit, "y") && len(unit) > 1 && !strings.ContainsRune("aeiou", rune(unit[len(unit)-2])):
		return unit[:len(unit)-1] + "ies"
	default:
		return unit + "s"
	}
}

// days writes a notice period.
func days(n int) string {
	if n == 1 {
		return "a day's notice"
	}
	return fmt.Sprintf("%d days' notice", n)
}
