// Package orderbook holds the rules of a small shop's catalogue and order
// book: what is for sale at what price, what has been ordered, what it comes
// to, and the exact text and links the shopkeeper sends to a customer.
//
// Nothing in this package talks to a network, a messaging service or a payment
// provider. It produces text and links; a person sends them.
package orderbook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Money is an amount in the minor unit of its currency — paise in India,
// centavos in the Philippines. Nothing in the money path is ever a
// floating-point number: a price typed as text becomes a whole number of minor
// units on the way in and goes back out as text, and every sum, difference and
// product in between is integer arithmetic.
type Money int64

// MinorUnitsPerMajor is how many minor units make one major unit. ISO 4217
// gives both the Indian rupee and the Philippine peso two decimal places, so
// one figure serves both markets. It is here once so that nothing else in the
// package carries its own idea of the scale.
const MinorUnitsPerMajor int64 = 100

// Currency is one of the two currencies this build knows how to write.
type Currency string

const (
	// INR is the Indian rupee.
	INR Currency = "INR"
	// PHP is the Philippine peso.
	PHP Currency = "PHP"
)

// Symbol is the currency's sign.
func (c Currency) Symbol() string {
	switch c {
	case PHP:
		return "₱" // ₱
	default:
		return "₹" // ₹
	}
}

// Name is the currency written out, for the amount in a message.
func (c Currency) Name() string {
	switch c {
	case PHP:
		return "pesos"
	default:
		return "rupees"
	}
}

// DialCode is the country calling code the market's numbers carry.
func (c Currency) DialCode() string {
	switch c {
	case PHP:
		return "63"
	default:
		return "91"
	}
}

// indianGrouping reports whether amounts in this currency are grouped the
// Indian way — the last three digits, then twos, so one hundred thousand is
// written 1,00,000 and not 100,000.
func (c Currency) indianGrouping() bool { return c != PHP }

// Normalise brings an unknown or empty currency code back to a known one.
func (c Currency) Normalise() Currency {
	switch Currency(strings.ToUpper(strings.TrimSpace(string(c)))) {
	case PHP:
		return PHP
	default:
		return INR
	}
}

// Currencies is every currency this build carries, for the settings page.
func Currencies() []Currency { return []Currency{INR, PHP} }

// String writes the amount as a plain decimal with both places and no symbol
// and no grouping: what a machine reads back.
func (m Money) String() string {
	sign := ""
	n := int64(m)
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/MinorUnitsPerMajor, n%MinorUnitsPerMajor)
}

// Format writes the amount the way the market writes it: the currency sign,
// then the digits grouped, then both decimal places. A negative amount carries
// the sign before the symbol, because "-₹40.00" reads as a deduction and
// "₹-40.00" reads as a typing mistake.
func (m Money) Format(c Currency) string {
	sign := ""
	n := int64(m)
	if n < 0 {
		sign = "-"
		n = -n
	}
	major := strconv.FormatInt(n/MinorUnitsPerMajor, 10)
	minor := fmt.Sprintf("%02d", n%MinorUnitsPerMajor)
	return sign + c.Symbol() + group(major, c.indianGrouping()) + "." + minor
}

// group puts separators into a string of digits.
func group(digits string, indian bool) string {
	if len(digits) <= 3 {
		return digits
	}
	head, tail := digits[:len(digits)-3], digits[len(digits)-3:]

	var parts []string
	size := 3
	if indian {
		size = 2
	}
	for len(head) > size {
		parts = append([]string{head[len(head)-size:]}, parts...)
		head = head[:len(head)-size]
	}
	parts = append([]string{head}, parts...)
	return strings.Join(parts, ",") + "," + tail
}

// MarshalJSON sends the amount as a decimal string. A string, not a number,
// because a JSON number is a float by the time most readers have finished with
// it, and a price is not something to leave to a float.
func (m Money) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

// UnmarshalJSON reads an amount either as a string of the kind a person types —
// "320", "320.50", "₹1,23,456.78", "₱1,250.00", "1 250,00" is not accepted —
// or as a whole JSON number, which is read as minor units.
func (m *Money) UnmarshalJSON(raw []byte) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || trimmed == "" {
		*m = 0
		return nil
	}
	if trimmed[0] != '"' {
		var minor int64
		if err := json.Unmarshal(raw, &minor); err != nil {
			return fmt.Errorf("%s is not an amount", trimmed)
		}
		*m = Money(minor)
		return nil
	}

	var typed string
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	parsed, err := ParseMoney(typed)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// ParseMoney reads an amount as a person types it. A currency sign, spaces and
// grouping separators are ignored; anything else that is not a digit, a sign or
// the decimal point is refused rather than quietly dropped, because dropping a
// character out of a price is how a ₹1,320 jar becomes a ₹1,320,000 one.
func ParseMoney(typed string) (Money, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ',', ' ', ' ', '₹', '₱', ' ', '_':
			return -1
		}
		return r
	}, strings.TrimSpace(typed))
	cleaned = strings.TrimPrefix(cleaned, "Rs.")
	cleaned = strings.TrimPrefix(cleaned, "Rs")
	cleaned = strings.TrimPrefix(cleaned, "INR")
	cleaned = strings.TrimPrefix(cleaned, "PHP")
	cleaned = strings.TrimPrefix(cleaned, "P")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return 0, nil
	}

	negative := false
	switch cleaned[0] {
	case '-':
		negative = true
		cleaned = cleaned[1:]
	case '+':
		cleaned = cleaned[1:]
	}

	whole, fraction := cleaned, ""
	if dot := strings.IndexByte(cleaned, '.'); dot >= 0 {
		whole, fraction = cleaned[:dot], cleaned[dot+1:]
		if strings.ContainsRune(fraction, '.') {
			return 0, fmt.Errorf("%q has more than one decimal point", typed)
		}
	}
	if whole == "" {
		whole = "0"
	}

	for _, part := range []string{whole, fraction} {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("%q is not an amount", typed)
			}
		}
	}
	if len(fraction) > 2 {
		return 0, fmt.Errorf("%q has more than two decimal places; %s has two", typed, "an amount in paise or centavos")
	}

	major, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is too large to be an amount", typed)
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	minor, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not an amount", typed)
	}
	if major > (1<<62)/MinorUnitsPerMajor {
		return 0, errors.New("that amount is too large")
	}

	total := major*MinorUnitsPerMajor + minor
	if negative {
		total = -total
	}
	return Money(total), nil
}

// divRound divides and rounds half away from zero — 2.5 becomes 3 and −2.5
// becomes −3 — which is how a shopkeeper rounds a price and what a customer
// checking a bill expects to find. It is the only division in the money path.
func divRound(numerator, denominator int64) int64 {
	if denominator == 0 {
		return 0
	}
	if denominator < 0 {
		numerator, denominator = -numerator, -denominator
	}
	if numerator >= 0 {
		return (numerator + denominator/2) / denominator
	}
	return -((-numerator + denominator/2) / denominator)
}
