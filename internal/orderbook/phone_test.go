package orderbook

import (
	"net/url"
	"strings"
	"testing"
)

func TestNormalisePhoneReadsEveryShapeAShopkeeperTypes(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		dialCode string
		want     E164
	}{
		// India
		{"a plain Indian mobile", "9876543210", "91", "919876543210"},
		{"with the trunk prefix", "09876543210", "91", "919876543210"},
		{"with the trunk prefix and spaces", "098765 43210", "91", "919876543210"},
		{"with a plus and spaces", "+91 98765 43210", "91", "919876543210"},
		{"with a plus and no spaces", "+919876543210", "91", "919876543210"},
		{"with the international prefix", "00919876543210", "91", "919876543210"},
		{"with hyphens", "98765-43210", "91", "919876543210"},
		{"with brackets and a hyphen", "(+91) 98765-43210", "91", "919876543210"},
		{"with dots", "98765.43210", "91", "919876543210"},
		{"with the country code and no plus", "919876543210", "91", "919876543210"},
		{"a mobile whose own digits start 91", "9198765432", "91", "919198765432"},
		{"an Indian landline in full", "+91 80 2345 6789", "91", "918023456789"},
		{"an en dash", "98765–43210", "91", "919876543210"},
		{"a non-breaking space", "98765 43210", "91", "919876543210"},

		// The Philippines
		{"a plain Philippine mobile", "9171234567", "63", "639171234567"},
		{"with the trunk prefix", "09171234567", "63", "639171234567"},
		{"with a plus and spaces", "+63 917 123 4567", "63", "639171234567"},
		{"with the international prefix", "00639171234567", "63", "639171234567"},
		{"a Manila landline with its area code", "+63 2 8123 4567", "63", "63281234567"},
		{"a Manila landline with the trunk prefix", "(02) 8123 4567", "63", "63281234567"},
		{"with the country code and no plus", "639171234567", "63", "639171234567"},

		// A number outside the two markets, written in full.
		{"a Singapore number in full", "+65 6123 4567", "91", "6561234567"},
		{"a UK number with the international prefix", "0044 20 7123 4567", "91", "442071234567"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalisePhone(c.typed, c.dialCode)
			if err != nil {
				t.Fatalf("%q with dial code %s was refused: %v", c.typed, c.dialCode, err)
			}
			if got != c.want {
				t.Fatalf("%q with dial code %s became %q, want %q", c.typed, c.dialCode, got, c.want)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("%q produced %q, which is not a shape a link can use: %v", c.typed, got, err)
			}
		})
	}
}

func TestNormalisePhoneRefusesWhatItCannotReadRatherThanGuessing(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		dialCode string
		because  string
	}{
		{"nothing", "", "91", "no phone number"},
		{"only spaces", "   ", "91", "no phone number"},
		{"a name", "Ramesh", "91", "not a phone number"},
		{"a half-typed number", "98765", "91", "5 digits"},
		{"a number with a letter in it", "98765a3210", "91", "not a phone number"},
		{"an extension", "080 2345 6789 ext 12", "91", "extension"},
		{"an extension written with x", "08023456789x12", "91", "extension"},
		{"eleven digits in India", "98765432100", "91", "neither"},
		{"thirteen digits in India", "9876543210000", "91", "neither"},
		{"all zeroes", "0000", "91", "zero"},
		{"a single zero", "0", "91", "all zeroes"},
		{"a plus and nothing", "+", "91", "digits"},
		{"too long for E.164", "+9198765432100000", "91", "15"},
		{"only a country code", "+91", "91", "at least"},
		{"eight digits in the Philippines", "81234567", "63", "8 digits"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalisePhone(c.typed, c.dialCode)
			if err == nil {
				t.Fatalf("%q was read as %q; it should have been refused", c.typed, got)
			}
			if !strings.Contains(err.Error(), c.because) {
				t.Fatalf("%q was refused with %q, which does not mention %q", c.typed, err, c.because)
			}
		})
	}
}

func TestNormalisePhoneFallsBackToIndiaWhenNoDialCodeIsSet(t *testing.T) {
	got, err := NormalisePhone("9876543210", "")
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got != "919876543210" {
		t.Fatalf("with no dial code set, %q became %q", "9876543210", got)
	}
}

func TestEveryNormalisedNumberMakesALinkThatParses(t *testing.T) {
	// The whole point of the type: anything NormalisePhone returns can go
	// straight into a link, and the link is a URL whose path is the number.
	typed := []string{"9876543210", "098765 43210", "+91 98765 43210", "09171234567", "+63 917 123 4567"}
	codes := []string{"91", "91", "91", "63", "63"}

	for i, raw := range typed {
		number, err := NormalisePhone(raw, codes[i])
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		link, err := WhatsAppLink(number, "Your order is ready — ₹960.00", DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		parsed, err := url.Parse(link.URL)
		if err != nil {
			t.Fatalf("%q produced a link that will not parse: %v", raw, err)
		}
		if strings.TrimPrefix(parsed.Path, "/") != string(number) {
			t.Fatalf("%q produced a link to %q, want %q", raw, parsed.Path, number)
		}
	}
}

func TestPhoneValidationCatchesEveryShapeThatFailsSilently(t *testing.T) {
	cases := []struct {
		name   string
		number E164
		valid  bool
	}{
		{"an Indian mobile", "919876543210", true},
		{"a Philippine mobile", "639171234567", true},
		{"a short international number", "6561234567", true},
		{"exactly fifteen digits", "123456789012345", true},
		{"nothing", "", false},
		{"a leading plus", "+919876543210", false},
		{"a space inside", "91 9876543210", false},
		{"a hyphen inside", "91-9876543210", false},
		{"a leading zero", "0919876543210", false},
		{"seven digits", "1234567", false},
		{"sixteen digits", "1234567890123456", false},
		{"letters", "nineonenine", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.number.Validate()
			if c.valid && err != nil {
				t.Fatalf("%q was refused: %v", c.number, err)
			}
			if !c.valid && err == nil {
				t.Fatalf("%q was accepted", c.number)
			}
		})
	}
}

func TestPhoneDisplaySeparatesTheCountryCode(t *testing.T) {
	cases := []struct {
		number   E164
		dialCode string
		want     string
	}{
		{"919876543210", "91", "+91 9876543210"},
		{"639171234567", "63", "+63 9171234567"},
		{"6561234567", "91", "+6561234567"},
		{"", "91", ""},
	}
	for _, c := range cases {
		if got := c.number.Display(c.dialCode); got != c.want {
			t.Errorf("%q displayed as %q, want %q", c.number, got, c.want)
		}
	}
}

func TestLooksMobileRulesOutWhatItCan(t *testing.T) {
	cases := []struct {
		name   string
		number E164
		mobile bool
	}{
		{"an Indian mobile on 9", "919876543210", true},
		{"an Indian mobile on 6", "916876543210", true},
		{"an Indian number on 2 is a landline", "912234567890", false},
		{"an Indian number on 4 is a landline", "914434567890", false},
		{"a Philippine mobile on 9", "639171234567", true},
		{"a Manila landline is not a mobile", "63281234567", false},
		{"a number outside both markets gets the benefit of the doubt", "6561234567", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := LooksMobile(c.number); got != c.mobile {
				t.Fatalf("LooksMobile(%q) = %v, want %v", c.number, got, c.mobile)
			}
		})
	}
}
