package orderbook

import "sort"

// Severity is how much a problem matters.
type Severity string

const (
	// Stop is a problem that makes the order wrong rather than merely untidy.
	// The record is still kept — a shop writes an order down before it is
	// tidy — but no message is built from it, because a message is what the
	// customer reads and holds the shop to.
	Stop Severity = "stop"
	// Check is something to look at, which does not make the order wrong.
	Check Severity = "check"
)

// Problem is one thing wrong, named precisely enough to act on: which rule, in
// plain words, and which line of the order if it is about a line.
type Problem struct {
	// Code identifies the rule, so the interface can style it and a test can
	// assert on it without matching English.
	Code string `json:"code"`
	// Message says what is wrong, in words a shopkeeper would use.
	Message string `json:"message"`
	// Severity is whether this stops a message being built.
	Severity Severity `json:"severity"`
	// Line is the line of the order it is about, counting from 1. Zero means
	// the order as a whole.
	Line int `json:"line,omitempty"`
}

// Every problem code this package can report. They are listed in one place so
// that the interface and the documentation cannot drift away from the code.
const (
	ProblemNoLines           = "no-lines"
	ProblemUnknownProduct    = "unknown-product"
	ProblemSoldOut           = "sold-out"
	ProblemZeroQuantity      = "zero-quantity"
	ProblemNegativeQuantity  = "negative-quantity"
	ProblemBelowMinimum      = "below-minimum"
	ProblemPriceChanged      = "price-changed"
	ProblemFreeLine          = "free-line"
	ProblemShortNotice       = "short-notice"
	ProblemDeliveryBefore    = "delivery-before-order"
	ProblemNoDeliveryDate    = "no-delivery-date"
	ProblemNoCustomerName    = "no-customer-name"
	ProblemUnreachable       = "unreachable"
	ProblemNotMobile         = "not-a-mobile"
	ProblemNoAddress         = "no-address"
	ProblemDiscountTooLarge  = "discount-over-subtotal"
	ProblemNegativeDiscount  = "negative-discount"
	ProblemNegativeFee       = "negative-delivery-fee"
	ProblemOverpaid          = "overpaid"
	ProblemNegativePaid      = "negative-payment"
	ProblemNoProductName     = "no-product-name"
	ProblemNoProductCode     = "no-product-code"
	ProblemNoProductPrice    = "no-product-price"
	ProblemDuplicateCode     = "duplicate-code"
	ProblemNoticeOnStockItem = "notice-on-a-stock-item"
	ProblemFormattingInName  = "formatting-character-in-name"
	ProblemNoShopName        = "no-shop-name"
	ProblemNoShopNumber      = "no-shop-number"
	ProblemCatalogueEmpty    = "catalogue-empty"
	ProblemNothingToShare    = "nothing-to-share"
	ProblemTooLongForStatus  = "too-long-for-a-status"
)

// blocked reports whether any of these problems stops a message being built.
func blocked(problems []Problem) bool {
	for _, p := range problems {
		if p.Severity == Stop {
			return true
		}
	}
	return false
}

// sortProblems puts the problems in the order a person would work through
// them: the ones that stop the order first, then by line, then by rule.
func sortProblems(problems []Problem) []Problem {
	sort.SliceStable(problems, func(i, j int) bool {
		a, b := problems[i], problems[j]
		if (a.Severity == Stop) != (b.Severity == Stop) {
			return a.Severity == Stop
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	return problems
}
