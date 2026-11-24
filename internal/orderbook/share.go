package orderbook

import (
	"fmt"
	"strings"
)

// Share is the catalogue as one block of text, ready to paste into a broadcast,
// a status or a group, with everything needed to decide where it will fit.
//
// It is rebuilt from the catalogue every time it is asked for, and never
// stored. That is the point: a price list kept as text goes stale the moment a
// price moves, and the shop that has been pasting the same block for two months
// is the shop arguing with a customer about ₹20. Change a price, ask for the
// text again, and the text is right.
type Share struct {
	// Text is the whole block.
	Text string `json:"text"`
	// Characters is how long it is, counted in characters.
	Characters int `json:"characters"`
	// Lines is how many lines it runs to.
	Lines int `json:"lines"`
	// Listed is how many products are in it.
	Listed int `json:"listed"`
	// SoldOut is how many were listed as off the list today.
	SoldOut int `json:"soldOut"`
	// Hidden is how many were left out because the shop marked them hidden.
	Hidden int `json:"hidden"`
	// FitsStatus reports whether it is short enough for a WhatsApp status.
	FitsStatus bool `json:"fitsStatus"`
	// FitsMessage reports whether it is short enough for one message.
	FitsMessage bool `json:"fitsMessage"`
	// StatusLimit and MessageLimit are the figures those answers were measured
	// against, so the page can state them rather than keep its own copy.
	StatusLimit  int `json:"statusLimit"`
	MessageLimit int `json:"messageLimit"`
	// Link is the shop's own chat link carrying the catalogue, for sending it
	// to one customer who asked.
	Link Link `json:"link"`
	// Problems is what is wrong with the catalogue behind the text.
	Problems []Problem `json:"problems"`
	// PricesChangedOn is the day the text says the prices are from.
	PricesChangedOn Date `json:"pricesChangedOn"`
}

// ShareCatalogue builds the shareable catalogue text.
//
// What goes in, and why:
//
//   - Available things first, with the price, the pack size and the price per
//     kilogramme or litre where the pack has a size. The per-kilogramme figure
//     is what makes two pack sizes comparable, and a customer who can compare
//     them stops asking.
//   - Made-to-order things next, each saying how much notice it needs, because
//     the question that follows a price list is always "can I have it
//     tomorrow".
//   - Sold-out things last and plainly marked. Leaving them off looks tidier
//     and produces an order for them the same afternoon.
//   - The day the prices are from, and how to order.
//
// Anything the shop marked hidden is left out, and anything with no price or no
// name is left out too — a catalogue line with a blank in it is worse than a
// line that is not there.
func ShareCatalogue(shop Shop, c Catalogue) Share {
	shop = shop.Normalise()
	c = c.Normalise()
	currency := shop.Currency

	var available, toOrder, soldOut []Product
	hidden := 0

	for _, p := range c.Products {
		if p.Hidden {
			hidden++
			continue
		}
		if p.Name == "" || p.Price <= 0 {
			continue
		}
		switch p.Availability.Normalise() {
		case SoldOut:
			soldOut = append(soldOut, p)
		case MadeToOrder:
			toOrder = append(toOrder, p)
		default:
			available = append(available, p)
		}
	}

	var b builder

	if shop.Name != "" {
		header := shop.Name
		if shop.Area != "" {
			header += " — " + shop.Area
		}
		b.line(bold(header))
	}
	if !c.PricesChangedOn.IsZero() {
		b.addf("Prices from %s", c.PricesChangedOn.Pretty())
	}

	section := func(title string, products []Product, withNotice bool) {
		if len(products) == 0 {
			return
		}
		b.blank()
		b.line(bold(title))
		for _, p := range products {
			b.line(shareLine(p, currency, withNotice))
		}
	}

	section("Available now", available, false)
	section("Made to order", toOrder, true)

	if len(soldOut) > 0 {
		b.blank()
		b.line(bold("Sold out just now"))
		names := make([]string, 0, len(soldOut))
		for _, p := range soldOut {
			names = append(names, p.Label())
		}
		b.line(strings.Join(names, ", "))
	}

	if shop.HowToOrder != "" {
		b.blank()
		b.line(shop.HowToOrder)
	}
	if shop.PayTo != "" {
		b.addf("Payment: %s", shop.PayTo)
	}
	if shop.Phone != "" {
		if number, err := shop.Number(); err == nil {
			b.addf("Orders: %s", number.Display(shop.DialCode))
		}
	}

	text := b.String()
	limits := shop.Limits.Normalise()

	share := Share{
		Text:            text,
		Characters:      characters(text),
		Lines:           strings.Count(text, "\n") + 1,
		Listed:          len(available) + len(toOrder),
		SoldOut:         len(soldOut),
		Hidden:          hidden,
		StatusLimit:     limits.StatusCharacters,
		MessageLimit:    limits.MessageCharacters,
		PricesChangedOn: c.PricesChangedOn,
		Problems:        c.Problems(),
	}
	if text == "" {
		share.Lines = 0
	}
	share.FitsStatus = share.Characters > 0 && share.Characters <= limits.StatusCharacters
	share.FitsMessage = share.Characters > 0 && share.Characters <= limits.MessageCharacters

	share.Link = linkTo(shop.Phone, shop, text)
	if shop.Phone == "" {
		share.Link = CopyOnly(text, "the shop has no number set, so there is no link — copy the text instead", limits)
	}

	if share.Listed == 0 {
		share.Problems = append(share.Problems, Problem{
			Code:     ProblemNothingToShare,
			Message:  "nothing in the catalogue can be shared: everything is hidden, sold out, unnamed or unpriced",
			Severity: Stop,
		})
	}
	if !share.FitsStatus && share.Characters > 0 {
		share.Problems = append(share.Problems, Problem{
			Code: ProblemTooLongForStatus,
			Message: fmt.Sprintf("the catalogue runs to %d characters and a status holds about %d; "+
				"send it as a message or a broadcast instead, or hide a few items",
				share.Characters, limits.StatusCharacters),
			Severity: Check,
		})
	}
	share.Problems = sortProblems(share.Problems)

	return share
}

// shareLine is one product as one line of the shared catalogue.
func shareLine(p Product, currency Currency, withNotice bool) string {
	line := bold(p.Name)
	if pack := p.PackLabel(); pack != "" {
		line += " " + pack
	}
	line += " — " + p.Price.Format(currency)
	if p.Unit != "" && p.Unit != "unit" {
		line += " a " + p.Unit
	}

	// The price per standard quantity, where it says something the price does
	// not already say. A one-kilogramme pack at ₹580 does not need "₹580 a kg"
	// after it.
	if per, name, ok := p.PerStandard(); ok && per != p.Price {
		line += fmt.Sprintf(" (%s a %s)", per.Format(currency), name)
	}

	if withNotice && p.NoticeDays > 0 {
		line += fmt.Sprintf(" — %s", days(p.NoticeDays))
	}
	if p.MinQty > 0 {
		line += fmt.Sprintf(" — least %s", p.MinQty)
	}
	if p.Note != "" {
		line += " — " + p.Note
	}
	return line
}
