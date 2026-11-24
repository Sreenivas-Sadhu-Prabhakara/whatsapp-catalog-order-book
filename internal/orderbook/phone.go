package orderbook

import (
	"fmt"
	"strings"
)

// E164 is a telephone number in the shape wa.me takes: the country calling code
// followed by the national number, digits only, with no plus sign, no spaces,
// no brackets and no leading zero. It is what ITU-T Recommendation E.164 calls
// the international public telecommunication number, written without its
// prefix.
//
// It exists as its own type so that nothing can hand a half-cleaned number to
// a link builder. NormalisePhone is the only thing that makes one.
type E164 string

// E.164 bounds: a number is at most fifteen digits, and the shortest useful one
// is a one-digit country code and a short national number. A number below the
// minimum is a typing mistake rather than a short number.
const (
	// MaxPhoneDigits is E.164's ceiling on the whole number.
	MaxPhoneDigits = 15
	// MinPhoneDigits is below any real international number, and above the
	// three or four digits a half-typed one has.
	MinPhoneDigits = 8
)

// Validate reports whether this is the shape wa.me takes. It is a check on the
// shape and nothing more: a number of the right shape may still not be anyone's
// number, and this tool has no way of finding that out — it makes no network
// call of any kind. What it does catch is every shape that silently fails:
// wa.me answers a malformed number with an error page, not with a chat, and the
// shopkeeper's message is never sent.
func (e E164) Validate() error {
	if e == "" {
		return fmt.Errorf("there is no phone number")
	}
	for _, r := range string(e) {
		if r < '0' || r > '9' {
			return fmt.Errorf("%q is not a number a chat link can use: it has to be digits only, "+
				"country code first, with no plus sign and no spaces", string(e))
		}
	}
	if e[0] == '0' {
		return fmt.Errorf("%q starts with a zero: a chat link needs the country code, not the trunk prefix", string(e))
	}
	if len(e) < MinPhoneDigits {
		return fmt.Errorf("%q is only %d digits; an international number is at least %d", string(e), len(e), MinPhoneDigits)
	}
	if len(e) > MaxPhoneDigits {
		return fmt.Errorf("%q is %d digits; no number is longer than %d", string(e), len(e), MaxPhoneDigits)
	}
	return nil
}

// Display writes the number back the way a person reads it: a plus, the country
// code, then the rest. It is for showing, never for a link.
func (e E164) Display(dialCode string) string {
	if e == "" {
		return ""
	}
	if dialCode != "" && strings.HasPrefix(string(e), dialCode) {
		return "+" + dialCode + " " + string(e)[len(dialCode):]
	}
	return "+" + string(e)
}

// nationalDigits is how many digits a national number has in each market this
// build knows, and what a national mobile number begins with. It is what lets
// the tool tell a number that already carries its country code from one that
// does not — the thing a shopkeeper never types consistently and the commonest
// reason a chat link opens an error page.
var nationalDigits = map[string]struct {
	length      int
	mobileFirst string
	market      string
}{
	// An Indian mobile number is ten digits beginning 6, 7, 8 or 9.
	"91": {length: 10, mobileFirst: "6789", market: "India"},
	// A Philippine mobile number is ten digits beginning 9 — the national
	// significant number, written 0917... with the trunk prefix.
	"63": {length: 10, mobileFirst: "9", market: "the Philippines"},
}

// NormalisePhone turns a number as a shopkeeper types it into the shape a chat
// link takes, using the shop's own country code to fill in what was left out.
//
// The rules, in the order they are applied:
//
//   - Spaces, brackets, hyphens, dots and dashes are punctuation and go.
//   - A leading plus means the number is already international: take it as it
//     is.
//   - A leading 00 is the international prefix in both markets: drop it and
//     take the rest as international.
//   - A single leading 0 is the trunk prefix: drop it and put the shop's own
//     country code in front. 098765 43210 is the same number as +91 98765
//     43210.
//   - A number of exactly the market's national length is a national number:
//     put the country code in front.
//   - A number of the country code's length plus the national length, which
//     starts with the country code, is already international.
//   - Anything else is ambiguous, and is refused with the reason, because
//     guessing produces a number that looks right and reaches nobody.
func NormalisePhone(typed, dialCode string) (E164, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', ' ', '-', '‐', '–', '—', '(', ')', '.', '/', '_':
			return -1
		}
		return r
	}, strings.TrimSpace(typed))

	if cleaned == "" {
		return "", fmt.Errorf("there is no phone number")
	}

	lower := strings.ToLower(cleaned)
	for _, marker := range []string{"ext", "x"} {
		if at := strings.Index(lower, marker); at > 0 {
			return "", fmt.Errorf("%q carries an extension; a chat link cannot dial one — give the mobile number instead", typed)
		}
	}

	international := false
	switch {
	case strings.HasPrefix(cleaned, "+"):
		cleaned, international = cleaned[1:], true
	case strings.HasPrefix(cleaned, "00"):
		cleaned, international = cleaned[2:], true
	}

	for _, r := range cleaned {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%q is not a phone number", typed)
		}
	}
	if cleaned == "" {
		return "", fmt.Errorf("%q has no digits in it", typed)
	}

	dialCode = strings.TrimSpace(dialCode)
	if dialCode == "" {
		dialCode = INR.DialCode()
	}

	if international {
		candidate := E164(cleaned)
		if err := candidate.Validate(); err != nil {
			return "", err
		}
		return candidate, nil
	}

	// A single leading zero is the trunk prefix, which is national and never
	// part of the number a chat link takes.
	if strings.HasPrefix(cleaned, "0") {
		cleaned = strings.TrimLeft(cleaned, "0")
		if cleaned == "" {
			return "", fmt.Errorf("%q is all zeroes", typed)
		}
		candidate := E164(dialCode + cleaned)
		if err := candidate.Validate(); err != nil {
			return "", err
		}
		return candidate, nil
	}

	shape, known := nationalDigits[dialCode]
	switch {
	case known && len(cleaned) == shape.length:
		// A national number: the country code goes in front. Note that this is
		// decided by length and not by the leading digits, because a number
		// that begins with the country code's own digits — 91 98765 432 in
		// India — is still a national number if it is the national length.
		candidate := E164(dialCode + cleaned)
		if err := candidate.Validate(); err != nil {
			return "", err
		}
		return candidate, nil

	case known && len(cleaned) == len(dialCode)+shape.length && strings.HasPrefix(cleaned, dialCode):
		candidate := E164(cleaned)
		if err := candidate.Validate(); err != nil {
			return "", err
		}
		return candidate, nil

	case known && len(cleaned) < shape.length:
		return "", fmt.Errorf("%q is %d digits; a number in %s is %d, or write it in full with a + and the country code",
			typed, len(cleaned), shape.market, shape.length)

	case known:
		return "", fmt.Errorf("%q is %d digits, which is neither a %d-digit number in %s nor that number with its "+
			"country code in front — write it in full with a + so there is nothing to guess",
			typed, len(cleaned), shape.length, shape.market)
	}

	// A market this build carries no national shape for: take the number as
	// international, because that is the only reading that does not invent
	// digits.
	candidate := E164(cleaned)
	if err := candidate.Validate(); err != nil {
		return "", err
	}
	return candidate, nil
}

// LooksMobile reports whether a normalised number could be a mobile number in
// its own market, which is worth knowing because a chat link to a landline
// opens a chat nobody will ever read.
//
// It is a weak check and deliberately errs towards yes. In the Philippines a
// mobile number begins with 9 and an area code does not, so the check is sound.
// In India it only rules out a number beginning 1 to 5: mobiles begin 6 to 9,
// but so do several of the longer area codes, so an Indian landline on a
// Bengaluru or Chennai code cannot be told from a mobile by its digits at all.
// The answer is therefore reported as something to check and never as a
// refusal.
func LooksMobile(e E164) bool {
	// 91 and 63 are each a whole country code with nothing assigned beneath
	// them, so a number starting with one of them is from that market and can
	// be judged against its national shape.
	for dialCode, shape := range nationalDigits {
		if !strings.HasPrefix(string(e), dialCode) {
			continue
		}
		rest := string(e)[len(dialCode):]
		if len(rest) != shape.length {
			// Not the length a mobile number has in this market: a landline on
			// an area code, most likely.
			return false
		}
		return strings.ContainsRune(shape.mobileFirst, rune(rest[0]))
	}
	// Outside the two markets this build knows the shapes of, say nothing
	// rather than guess.
	return true
}
