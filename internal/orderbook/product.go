package orderbook

import (
	"fmt"
	"strings"
)

// Availability is whether a thing can be ordered today.
type Availability string

const (
	// Available means it is on the shelf.
	Available Availability = "available"
	// MadeToOrder means it is made when it is ordered, and the shop needs
	// notice.
	MadeToOrder Availability = "made-to-order"
	// SoldOut means it cannot be ordered at all. A line for something sold out
	// is the one catalogue mistake that reaches the customer as a promise the
	// shop cannot keep, so it stops the order.
	SoldOut Availability = "sold-out"
)

// Availabilities is every state, for the settings page.
func Availabilities() []Availability { return []Availability{Available, MadeToOrder, SoldOut} }

// Normalise brings an unknown state back to available.
func (a Availability) Normalise() Availability {
	switch Availability(strings.ToLower(strings.TrimSpace(string(a)))) {
	case MadeToOrder, "made to order", "to-order":
		return MadeToOrder
	case SoldOut, "sold out", "out-of-stock", "out of stock":
		return SoldOut
	default:
		return Available
	}
}

// Name is the state written out for a customer.
func (a Availability) Name() string {
	switch a.Normalise() {
	case MadeToOrder:
		return "made to order"
	case SoldOut:
		return "sold out"
	default:
		return "available"
	}
}

// Measure is what a pack is measured in. A pack size is the thing a customer
// compares between two sellers, and it is only comparable if the shop says
// whether 500 means grams, millilitres or pieces.
type Measure string

const (
	// ByWeight is a pack measured in grams.
	ByWeight Measure = "g"
	// ByVolume is a pack measured in millilitres.
	ByVolume Measure = "ml"
	// ByCount is a pack measured in pieces.
	ByCount Measure = "piece"
	// NoMeasure is a pack with no stated size.
	NoMeasure Measure = ""
)

// Measures is every measure a pack can be in.
func Measures() []Measure { return []Measure{NoMeasure, ByWeight, ByVolume, ByCount} }

// Normalise brings an unknown measure back to none.
func (m Measure) Normalise() Measure {
	switch Measure(strings.ToLower(strings.TrimSpace(string(m)))) {
	case ByWeight, "gram", "grams", "gm":
		return ByWeight
	case ByVolume, "millilitre", "millilitres", "milliliter":
		return ByVolume
	case ByCount, "pieces", "pc", "pcs", "nos":
		return ByCount
	default:
		return NoMeasure
	}
}

// standardName is the measure a price per standard quantity is quoted in: a
// kilogramme for weight, a litre for volume, one of them for a count.
func (m Measure) standardName() string {
	switch m.Normalise() {
	case ByWeight:
		return "kg"
	case ByVolume:
		return "litre"
	case ByCount:
		return "piece"
	default:
		return ""
	}
}

// perStandard is how many of the measure's own units make one standard
// quantity: a thousand grams to a kilogramme, a thousand millilitres to a
// litre, one piece to a piece.
func (m Measure) perStandard() int64 {
	switch m.Normalise() {
	case ByWeight, ByVolume:
		return 1000
	case ByCount:
		return 1
	default:
		return 0
	}
}

// Product is one thing the shop sells, at one price, in one pack.
type Product struct {
	// Code is the short name the shopkeeper types to put this on an order. It
	// is also what an order line keeps, so it must not change under an order
	// that already refers to it.
	Code string `json:"code"`
	// Name is what the customer reads.
	Name string `json:"name"`
	// Unit is what one of it is called: a jar, a packet, a box, a dozen.
	Unit string `json:"unit"`
	// Price is what one unit costs.
	Price Money `json:"price"`
	// PackAmount is how much is in one unit, in the pack's own measure.
	PackAmount Qty `json:"packAmount"`
	// PackMeasure is what the pack amount is measured in.
	PackMeasure Measure `json:"packMeasure"`
	// Availability is whether it can be ordered.
	Availability Availability `json:"availability"`
	// NoticeDays is how many days' notice a made-to-order item needs.
	NoticeDays int `json:"noticeDays"`
	// MinQty is the smallest quantity the shop will make or sell, where there
	// is one. Zero means no minimum.
	MinQty Qty `json:"minQty"`
	// Note is a short line that goes out with the product in the shared
	// catalogue: "no preservatives", "contains peanuts".
	Note string `json:"note"`
	// Hidden keeps the product out of the shared catalogue text while leaving
	// it on the shop's own list — a special order for one customer, or
	// something not being offered this week.
	Hidden bool `json:"hidden"`
}

// NormaliseCode brings a product code to the one form everything else uses:
// upper case, with runs of anything that is not a letter or a digit replaced by
// a single hyphen. "mango pickle 500" and "Mango_Pickle__500" are then the same
// product, which they plainly are, and an order that refers to one finds the
// other.
func NormaliseCode(raw string) string {
	var out strings.Builder
	lastWasSeparator := false

	for _, r := range strings.TrimSpace(strings.ToUpper(raw)) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out.WriteRune(r)
			lastWasSeparator = false
		default:
			if !lastWasSeparator && out.Len() > 0 {
				out.WriteByte('-')
				lastWasSeparator = true
			}
		}
	}
	return strings.TrimRight(out.String(), "-")
}

// Normalise tidies a product as it arrives: the code to its one form, the text
// trimmed, and figures that cannot be negative brought to zero. Notice days on
// something that is not made to order are dropped, because a stock item with
// three days' notice is a contradiction that would quietly refuse every order.
func (p Product) Normalise() Product {
	p.Code = NormaliseCode(p.Code)
	p.Name = strings.TrimSpace(p.Name)
	p.Unit = strings.TrimSpace(p.Unit)
	p.Note = strings.TrimSpace(p.Note)
	p.Availability = p.Availability.Normalise()
	p.PackMeasure = p.PackMeasure.Normalise()

	if p.Price < 0 {
		p.Price = 0
	}
	if p.PackAmount < 0 {
		p.PackAmount = 0
	}
	if p.MinQty < 0 {
		p.MinQty = 0
	}
	if p.NoticeDays < 0 {
		p.NoticeDays = 0
	}
	if p.Availability != MadeToOrder {
		p.NoticeDays = 0
	}
	if p.PackMeasure == NoMeasure {
		p.PackAmount = 0
	}
	if p.Unit == "" {
		p.Unit = "unit"
	}
	return p
}

// PackLabel writes the pack size the way a label does, using the larger measure
// where it is exact: 1 kg rather than 1000 g, 1 litre rather than 1000 ml, and
// 750 g as it is. A customer comparing two jars reads the pack size before the
// price.
func (p Product) PackLabel() string {
	if p.PackAmount <= 0 || p.PackMeasure == NoMeasure {
		return ""
	}
	// A pack of a kilogramme or more reads as kilogrammes, so long as it is a
	// whole number of grams; below that, and for anything finer, grams.
	whole := int64(p.PackAmount)%QtyScale == 0
	large := int64(p.PackAmount) >= 1000*QtyScale

	switch p.PackMeasure {
	case ByWeight:
		if whole && large {
			return fmt.Sprintf("%s kg", Qty(int64(p.PackAmount)/1000))
		}
		return fmt.Sprintf("%s g", p.PackAmount)
	case ByVolume:
		if whole && large {
			return fmt.Sprintf("%s litre", Qty(int64(p.PackAmount)/1000))
		}
		return fmt.Sprintf("%s ml", p.PackAmount)
	case ByCount:
		if p.PackAmount == Whole(1) {
			return "1 piece"
		}
		return fmt.Sprintf("%s pieces", p.PackAmount)
	}
	return ""
}

// Label is the product as one line of text: the name, then the pack size in
// brackets where there is one.
func (p Product) Label() string {
	if pack := p.PackLabel(); pack != "" {
		return p.Name + " (" + pack + ")"
	}
	return p.Name
}

// PerStandard is the price per kilogramme, per litre or per piece, and the name
// of that standard quantity.
//
// This is the figure that makes a catalogue honest. A 500 g jar at ₹320 and a
// 1 kg jar at ₹580 are hard to compare in a chat thread and trivial to compare
// at ₹640 and ₹580 the kilogramme. It is worked out in whole minor units, by
// one integer division rounded once, and comes back as nothing at all where the
// pack has no stated size — rather than as a figure that looks authoritative
// and was guessed.
func (p Product) PerStandard() (Money, string, bool) {
	per := p.PackMeasure.perStandard()
	if per == 0 || p.PackAmount <= 0 || p.Price <= 0 {
		return 0, "", false
	}
	// price × (units in a standard quantity) ÷ (units in this pack).
	value := divRound(int64(p.Price)*per*QtyScale, int64(p.PackAmount))
	return Money(value), p.PackMeasure.standardName(), true
}

// LeadTime is the earliest day this product can be delivered if it is ordered
// on a given day.
func (p Product) LeadTime(ordered Date) Date {
	if p.Availability.Normalise() != MadeToOrder || p.NoticeDays <= 0 {
		return ordered
	}
	return ordered.AddDays(p.NoticeDays)
}

// Problems reports what is wrong with the product itself, before anybody tries
// to order it.
func (p Product) Problems() []Problem {
	var problems []Problem
	add := func(code, message string, severity Severity) {
		problems = append(problems, Problem{Code: code, Message: message, Severity: severity})
	}

	if p.Code == "" {
		add(ProblemNoProductCode, "this product has no code, so nothing can refer to it on an order", Stop)
	}
	if p.Name == "" {
		add(ProblemNoProductName, "this product has no name, so a customer reading the catalogue sees a blank", Stop)
	}
	if p.Price <= 0 {
		add(ProblemNoProductPrice, fmt.Sprintf("%s has no price; a line for it would come to nothing",
			nameOr(p.Name, p.Code)), Stop)
	}
	if p.Availability.Normalise() == MadeToOrder && p.NoticeDays == 0 {
		add(ProblemShortNotice, fmt.Sprintf("%s is made to order with no notice period set, so the book will "+
			"accept an order for it today", nameOr(p.Name, p.Code)), Check)
	}
	if hasWhatsAppFormatting(p.Name) {
		add(ProblemFormattingInName, fmt.Sprintf("the name %q contains a character WhatsApp reads as formatting "+
			"(* _ ~ or a backtick), so it is sent without emphasis in the shared catalogue", p.Name), Check)
	}
	return sortProblems(problems)
}

// nameOr falls back to the code where there is no name yet.
func nameOr(name, code string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if code != "" {
		return code
	}
	return "this product"
}
