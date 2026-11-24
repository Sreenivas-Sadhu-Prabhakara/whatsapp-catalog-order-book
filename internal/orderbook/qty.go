package orderbook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Qty is a quantity in thousandths of whatever is being counted. A shop sells
// three jars and it sells half a kilogramme, and both have to be the same kind
// of number, so the whole-number scale has to be fine enough for the smaller
// of the two. Thousandths hold a gram out of a kilogramme and a millilitre out
// of a litre, which is as fine as a shop ever weighs.
type Qty int64

// QtyScale is how many of Qty's units make one whole thing.
const QtyScale int64 = 1000

// Whole makes a quantity out of a plain count.
func Whole(count int64) Qty { return Qty(count * QtyScale) }

// String writes the quantity with no more decimal places than it needs: 3, not
// 3.000, and 0.5 rather than 0.500.
func (q Qty) String() string {
	sign := ""
	n := int64(q)
	if n < 0 {
		sign = "-"
		n = -n
	}
	whole, fraction := n/QtyScale, n%QtyScale
	if fraction == 0 {
		return sign + strconv.FormatInt(whole, 10)
	}
	text := fmt.Sprintf("%s%d.%03d", sign, whole, fraction)
	return strings.TrimRight(text, "0")
}

// MarshalJSON sends the quantity as a decimal string, for the same reason a
// price goes out as one.
func (q Qty) MarshalJSON() ([]byte, error) { return json.Marshal(q.String()) }

// UnmarshalJSON reads a quantity from a string of the kind a person types, or
// from a whole JSON number read as thousandths.
func (q *Qty) UnmarshalJSON(raw []byte) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || trimmed == "" {
		*q = 0
		return nil
	}
	if trimmed[0] != '"' {
		var thousandths int64
		if err := json.Unmarshal(raw, &thousandths); err != nil {
			return fmt.Errorf("%s is not a quantity", trimmed)
		}
		*q = Qty(thousandths)
		return nil
	}

	var typed string
	if err := json.Unmarshal(raw, &typed); err != nil {
		return err
	}
	parsed, err := ParseQty(typed)
	if err != nil {
		return err
	}
	*q = parsed
	return nil
}

// ParseQty reads a quantity as a person types it: "3", "0.5", "2.25",
// "1 1/2" is not accepted and says so.
func ParseQty(typed string) (Qty, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ',', ' ', ' ', '_':
			return -1
		}
		return r
	}, strings.TrimSpace(typed))
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
				return 0, fmt.Errorf("%q is not a quantity", typed)
			}
		}
	}
	if len(fraction) > 3 {
		return 0, fmt.Errorf("%q is finer than a thousandth, which is finer than a shop weighs", typed)
	}

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is too large to be a quantity", typed)
	}
	if units > (1<<62)/QtyScale {
		return 0, fmt.Errorf("%q is too large to be a quantity", typed)
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	thousandths, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a quantity", typed)
	}

	total := units*QtyScale + thousandths
	if negative {
		total = -total
	}
	return Qty(total), nil
}

// Times multiplies a unit price by a quantity and rounds the result once, here,
// where it becomes a figure on a bill. Half a kilogramme at ₹300.00 the
// kilogramme is ₹150.00 exactly; a third of one is ₹100.00 after rounding, and
// the rounding happens here and nowhere else so that the bill's lines add up to
// the bill's total.
func (q Qty) Times(unit Money) Money {
	return Money(divRound(int64(unit)*int64(q), QtyScale))
}
