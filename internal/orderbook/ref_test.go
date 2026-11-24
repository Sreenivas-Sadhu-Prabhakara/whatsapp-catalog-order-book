package orderbook

import "testing"

func TestNextRefCountsForwardAndNeverReusesANumber(t *testing.T) {
	cases := []struct {
		name   string
		orders []Order
		want   string
	}{
		{"an empty book", nil, "ORD-0001"},
		{"one order", []Order{{Ref: "ORD-0001"}}, "ORD-0002"},
		{"a gap in the middle", []Order{{Ref: "ORD-0001"}, {Ref: "ORD-0003"}}, "ORD-0004"},
		{"out of order", []Order{{Ref: "ORD-0009"}, {Ref: "ORD-0002"}}, "ORD-0010"},
		{"a reference the shop typed itself", []Order{{Ref: "Meera's jars"}}, "ORD-0001"},
		{"a mix", []Order{{Ref: "Meera's jars"}, {Ref: "ORD-0007"}}, "ORD-0008"},
		{"lower case", []Order{{Ref: "ord-0011"}}, "ORD-0012"},
		{"past four digits", []Order{{Ref: "ORD-9999"}}, "ORD-10000"},
		{"a malformed one", []Order{{Ref: "ORD-abc"}}, "ORD-0001"},
		{"an empty reference", []Order{{Ref: ""}}, "ORD-0001"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextRef(c.orders); got != c.want {
				t.Fatalf("the next reference is %q, want %q", got, c.want)
			}
		})
	}
}

func TestADeletedOrdersNumberIsNotHandedOutAgain(t *testing.T) {
	// A reference said out loud to a customer must never come back meaning a
	// different order.
	orders := []Order{{Ref: "ORD-0001"}, {Ref: "ORD-0002"}, {Ref: "ORD-0003"}}

	// The middle one is deleted.
	orders = append(orders[:1], orders[2:]...)
	if got := NextRef(orders); got != "ORD-0004" {
		t.Fatalf("after deleting ORD-0002 the next reference is %q, want ORD-0004", got)
	}

	// And the last one.
	orders = orders[:1]
	if got := NextRef(orders); got != "ORD-0002" {
		// With only ORD-0001 left, 0002 is free again. That is the one case
		// where a number can come back, and it comes back only because
		// nothing above it was ever issued in this book — which is as much as
		// a list of orders can know.
		t.Fatalf("the next reference is %q, want ORD-0002", got)
	}
}

func TestFindOrderMatchesHoweverTheReferenceIsTyped(t *testing.T) {
	orders := []Order{
		{Ref: "ORD-0001", Customer: Customer{Name: "First"}},
		{Ref: "ORD-0002", Customer: Customer{Name: "Second"}},
	}

	for _, typed := range []string{"ORD-0002", "ord-0002", " ORD-0002 ", "Ord-0002"} {
		at, ok := FindOrder(orders, typed)
		if !ok {
			t.Fatalf("%q found nothing", typed)
		}
		if orders[at].Customer.Name != "Second" {
			t.Fatalf("%q found %q", typed, orders[at].Customer.Name)
		}
	}

	for _, typed := range []string{"", "  ", "ORD-0003", "ORD0002"} {
		if at, ok := FindOrder(orders, typed); ok {
			t.Fatalf("%q found %q", typed, orders[at].Ref)
		}
	}
}
