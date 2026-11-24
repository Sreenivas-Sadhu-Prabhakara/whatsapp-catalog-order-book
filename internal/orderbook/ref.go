package orderbook

import (
	"fmt"
	"strconv"
	"strings"
)

// RefPrefix is what an order reference starts with. A reference has to be short
// enough to say over a phone and to write on a lid, and it has to be unique —
// "the mango order" stops being unique the second week.
const RefPrefix = "ORD-"

// refDigits is how wide the number is. Four digits is ten thousand orders,
// which is years of them for a home kitchen, and they sort as text in the order
// they were taken.
const refDigits = 4

// NextRef is the next reference after the highest one already in the book. It
// counts forward and never reuses a number, even after an order is deleted: a
// reference that has been said out loud to a customer must never come back
// meaning a different order.
func NextRef(orders []Order) string {
	highest := 0
	for _, o := range orders {
		if n, ok := refNumber(o.Ref); ok && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("%s%0*d", RefPrefix, refDigits, highest+1)
}

// refNumber reads the number out of a reference this tool made.
func refNumber(ref string) (int, bool) {
	trimmed := strings.TrimSpace(strings.ToUpper(ref))
	if !strings.HasPrefix(trimmed, RefPrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(trimmed, RefPrefix))
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// FindOrder looks an order up by its reference, ignoring case and surrounding
// space, and says where it is in the book.
func FindOrder(orders []Order, ref string) (int, bool) {
	wanted := strings.ToUpper(strings.TrimSpace(ref))
	if wanted == "" {
		return 0, false
	}
	for i, o := range orders {
		if strings.ToUpper(strings.TrimSpace(o.Ref)) == wanted {
			return i, true
		}
	}
	return 0, false
}
