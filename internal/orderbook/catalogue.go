package orderbook

import (
	"fmt"
	"sort"
	"strings"
)

// Shop is what the shop is and how it is reached — the part of the file that
// goes into the top of every message and changes about once a year.
type Shop struct {
	// Name is the shop's own name.
	Name string `json:"name"`
	// Area is where it is, in the one line a customer needs.
	Area string `json:"area"`
	// Currency is what it prices in.
	Currency Currency `json:"currency"`
	// DialCode is the country calling code its customers' numbers carry, used
	// to complete a number typed without one.
	DialCode string `json:"dialCode"`
	// Phone is the shop's own number, the one a customer replies to.
	Phone string `json:"phone"`
	// Email is where an order may also arrive, for a customer who does not use
	// a chat app.
	Email string `json:"email"`
	// PayTo is how a customer pays: a UPI identifier, a GCash number, a bank
	// line, or "cash on delivery". It is a line of text the shop writes, not a
	// payment integration — this tool collects no money and is connected to no
	// payment service.
	PayTo string `json:"payTo"`
	// HowToOrder is the line at the foot of the shared catalogue telling a
	// customer what to send.
	HowToOrder string `json:"howToOrder"`
	// Closing is the line a message ends on.
	Closing string `json:"closing"`
	// Limits are the lengths this shop's own clients handle.
	Limits Limits `json:"limits"`
}

// Normalise tidies the shop's details and fills in what has sensible defaults.
func (s Shop) Normalise() Shop {
	s.Name = strings.TrimSpace(s.Name)
	s.Area = strings.TrimSpace(s.Area)
	s.Phone = strings.TrimSpace(s.Phone)
	s.Email = strings.TrimSpace(s.Email)
	s.PayTo = strings.TrimSpace(s.PayTo)
	s.HowToOrder = strings.TrimSpace(s.HowToOrder)
	s.Closing = strings.TrimSpace(s.Closing)
	s.Currency = s.Currency.Normalise()
	s.Limits = s.Limits.Normalise()

	s.DialCode = strings.TrimLeft(strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s.DialCode), "0")
	if s.DialCode == "" {
		s.DialCode = s.Currency.DialCode()
	}
	if s.HowToOrder == "" {
		s.HowToOrder = "Reply with the item and how many, and the day you want it."
	}
	return s
}

// Number is the shop's own number in the shape a link takes, where it is
// usable.
func (s Shop) Number() (E164, error) { return NormalisePhone(s.Phone, s.DialCode) }

// Catalogue is everything the shop sells, with the day the prices last changed.
type Catalogue struct {
	// Products is the list, in the order the shop wants it read.
	Products []Product `json:"products"`
	// PricesChangedOn is the day a price last moved. The shared catalogue text
	// carries it, because a customer holding a three-month-old price list is
	// how an argument starts.
	PricesChangedOn Date `json:"pricesChangedOn"`
}

// Normalise tidies every product and drops the ones that are entirely empty —
// a row the shopkeeper started and abandoned.
func (c Catalogue) Normalise() Catalogue {
	out := make([]Product, 0, len(c.Products))
	for _, p := range c.Products {
		p = p.Normalise()
		if p.Code == "" && p.Name == "" && p.Price == 0 {
			continue
		}
		out = append(out, p)
	}
	c.Products = out
	return c
}

// Find looks a product up by its code, in either code's normalised form, so
// that a code typed in any shape finds the product it means.
func (c Catalogue) Find(code string) (Product, bool) {
	wanted := NormaliseCode(code)
	if wanted == "" {
		return Product{}, false
	}
	for _, p := range c.Products {
		if NormaliseCode(p.Code) == wanted {
			return p, true
		}
	}
	return Product{}, false
}

// Upsert puts a product in, replacing the one with the same code if there is
// one, and returns the catalogue with it in. Replacing rather than adding is
// what makes a price change a price change and not a second product at the old
// price.
func (c Catalogue) Upsert(p Product) Catalogue {
	p = p.Normalise()
	for i, existing := range c.Products {
		if NormaliseCode(existing.Code) == p.Code && p.Code != "" {
			c.Products[i] = p
			return c
		}
	}
	c.Products = append(c.Products, p)
	return c
}

// Remove takes a product off the list. It does not touch any order: an order
// already taken keeps its own copy of what was ordered and what it cost, so
// deleting a product can never change a bill that has already been given.
func (c Catalogue) Remove(code string) (Catalogue, bool) {
	wanted := NormaliseCode(code)
	for i, p := range c.Products {
		if NormaliseCode(p.Code) == wanted {
			c.Products = append(c.Products[:i], c.Products[i+1:]...)
			return c, true
		}
	}
	return c, false
}

// Sellable is the products a customer can order today, in catalogue order.
func (c Catalogue) Sellable() []Product {
	out := make([]Product, 0, len(c.Products))
	for _, p := range c.Products {
		if p.Availability.Normalise() != SoldOut {
			out = append(out, p)
		}
	}
	return out
}

// SoldOutItems is what is off the list today.
func (c Catalogue) SoldOutItems() []Product {
	out := make([]Product, 0, len(c.Products))
	for _, p := range c.Products {
		if p.Availability.Normalise() == SoldOut {
			out = append(out, p)
		}
	}
	return out
}

// Problems reports what is wrong with the catalogue: every product's own
// problems, plus the one problem a product cannot see on its own — two products
// sharing a code, which makes every order line ambiguous.
func (c Catalogue) Problems() []Problem {
	var problems []Problem

	if len(c.Products) == 0 {
		problems = append(problems, Problem{
			Code:     ProblemCatalogueEmpty,
			Message:  "there is nothing in the catalogue yet, so there is nothing to share and nothing to order",
			Severity: Check,
		})
	}

	seen := map[string][]string{}
	for _, p := range c.Products {
		code := NormaliseCode(p.Code)
		if code == "" {
			continue
		}
		seen[code] = append(seen[code], nameOr(p.Name, p.Code))
	}

	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		if len(seen[code]) > 1 {
			problems = append(problems, Problem{
				Code: ProblemDuplicateCode,
				Message: fmt.Sprintf("the code %s is on %d products (%s); an order line naming it could mean either",
					code, len(seen[code]), strings.Join(seen[code], ", ")),
				Severity: Stop,
			})
		}
	}

	for i, p := range c.Products {
		for _, problem := range p.Problems() {
			problem.Line = i + 1
			problems = append(problems, problem)
		}
	}
	return sortProblems(problems)
}
