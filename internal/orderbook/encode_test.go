package orderbook

import (
	"net/url"
	"strings"
	"testing"
)

// awkwardText is every kind of text a shop's own order actually carries, and
// every character that has ever quietly broken a link. Each one is used three
// ways: through the encoder on its own, through a wa.me link, and through a
// mail link.
var awkwardText = []struct {
	name string
	text string
}{
	{"plain", "Order confirmed"},
	{"a space", "2 jars"},
	{"many spaces", "2    jars   of   pickle"},
	{"an ampersand", "Mango & Lime pickle"},
	{"a plus sign", "2+2 jars free"},
	{"a plus between spaces", "Chakli + chutney"},
	{"a hash", "Order #14"},
	{"a percent sign", "20% off this week"},
	{"a percent-looking sequence", "100%25 cotton bag"},
	{"an already-encoded space", "two%20jars"},
	{"an equals sign", "Total = 960"},
	{"a question mark", "Collect on Friday?"},
	{"a slash", "Delivery 24/11 morning"},
	{"a colon", "Pay to: shop@upi"},
	{"a semicolon", "jar; box; packet"},
	{"a newline", "Mango pickle\nLime pickle"},
	{"a carriage return and newline", "Line one\r\nLine two"},
	{"a tab", "Mango\t320.00"},
	{"quotes", `He said "two jars" and 'no chilli'`},
	{"angle brackets", "<b>not html</b>"},
	{"a backtick and backslash", "path\\to\\nowhere `quoted`"},
	{"a rupee sign", "\u20b9960.00"},
	{"a peso sign", "\u20b11,250.00"},
	{"both currency signs", "\u20b9320 / \u20b1180"},
	{"an em dash and ellipsis", "Ready \u2014 collect after 5\u2026"},
	{"a non-breaking space", "\u20b9 960.00"},
	{"a food emoji", "Mango pickle \U0001f96d"},
	{"a joined emoji", "Thank you \U0001f469\u200d\U0001f373"},
	{"a flag emoji", "Made in India \U0001f1ee\U0001f1f3"},
	{"Devanagari", "\u0906\u092e \u0915\u093e \u0905\u0924\u093e\u0930"},
	{"Devanagari with digits and a sign", "\u0905\u0924\u093e\u0930 500 \u0917\u094d\u0930\u093e\u092e \u2014 \u20b9320"},
	{"Marathi with a nukta", "\u092c\u0947\u0938\u0928 \u0932\u093e\u0921\u0942 \u0921\u092c\u094d\u092c\u093e"},
	{"Tamil", "\u0bae\u0bbe\u0b99\u0bcd\u0b95\u0bbe\u0baf\u0bcd \u0b8a\u0bb1\u0bc1\u0b95\u0bbe\u0baf\u0bcd"},
	{"Tamil with a rupee sign", "\u0b95\u0b9f\u0bcd\u0b9f\u0ba3\u0bae\u0bcd: \u20b9960.00"},
	{"Tagalog in Latin script", "Mangga at kalamansi na palamig"},
	{"Tagalog with a tilde n", "Pi\u00f1a at ni\u00f1o \u2014 \u20b1180"},
	{"Tagalog in Baybayin", "\u1702\u1705\u1712\u1705\u1712\u1712"},
	{"Hindi, English and an emoji together", "\u0905\u0924\u093e\u0930 + pickle \U0001f96d \u20b9320"},
	{"a whole order body", "*Aaji's Kitchen*\n\nOrder #A-0003\n2 \u00d7 Mango pickle (500 g) \u2014 \u20b9640.00\n1 \u00d7 Chakli (250 g) \u2014 \u20b9140.00\n\n*Total \u20b9780.00*\nPay to: aaji@upi\n50% now & 50% on delivery?"},
	{"every delimiter at once", "?&=#+%/:;,@[]{}|^~`\\\"'<> \t\n"},
	{"a lone high rune", "\U0010ffff"},
	{"an empty string", ""},
}

func TestEveryAwkwardMessageSurvivesAWaMeLinkIntact(t *testing.T) {
	// This is the test the tool exists for. The link is built, then parsed back
	// out with the standard library's own URL parser — not with anything this
	// package wrote — and the message that comes out has to be the message that
	// went in, character for character.
	number := E164("919876543210")

	for _, c := range awkwardText {
		t.Run(c.name, func(t *testing.T) {
			link, err := WhatsAppLink(number, c.text, DefaultLimits())
			if err != nil {
				t.Fatalf("building a link for %q: %v", c.text, err)
			}

			parsed, err := url.Parse(link.URL)
			if err != nil {
				t.Fatalf("the link this tool built is not a URL: %v\n  %s", err, link.URL)
			}
			if parsed.Scheme != "https" || parsed.Host != "wa.me" {
				t.Fatalf("the link points at %s://%s", parsed.Scheme, parsed.Host)
			}
			if got := strings.TrimPrefix(parsed.Path, "/"); got != string(number) {
				t.Fatalf("the number came back as %q, want %q", got, number)
			}

			if c.text == "" {
				if parsed.RawQuery != "" {
					t.Fatalf("an empty message produced a query: %q", parsed.RawQuery)
				}
				return
			}

			// Query() runs the standard library's own percent-decoder over the
			// raw query, splitting on & exactly as any reader of this link
			// would.
			back := parsed.Query().Get("text")
			if back != c.text {
				t.Fatalf("the message did not survive the link.\n  sent: %q\n  back: %q\n   url: %s",
					c.text, back, link.URL)
			}
		})
	}
}

func TestEveryAwkwardMessageSurvivesAMailLinkIntact(t *testing.T) {
	for _, c := range awkwardText {
		t.Run(c.name, func(t *testing.T) {
			subject := "Your order \u2014 Aaji's Kitchen & Co. #14 (50% paid)"

			link, err := MailtoLink("shop.name+orders@example.co.in", subject, c.text, DefaultLimits())
			if err != nil {
				t.Fatalf("building a mail link: %v", err)
			}
			parsed, err := url.Parse(link.URL)
			if err != nil {
				t.Fatalf("the mail link is not a URL: %v\n  %s", err, link.URL)
			}
			if parsed.Scheme != "mailto" {
				t.Fatalf("the mail link has scheme %q", parsed.Scheme)
			}
			if parsed.Opaque != "shop.name%2Borders@example.co.in" {
				t.Fatalf("the address came back as %q; a plus in a local part must not become a space", parsed.Opaque)
			}

			query, err := url.ParseQuery(parsed.RawQuery)
			if err != nil {
				t.Fatalf("the mail link's query does not parse: %v", err)
			}
			if got := query.Get("subject"); got != subject {
				t.Fatalf("the subject came back as %q, want %q", got, subject)
			}
			if c.text == "" {
				if query.Has("body") {
					t.Fatalf("an empty body produced body=%q", query.Get("body"))
				}
				return
			}
			if got := query.Get("body"); got != c.text {
				t.Fatalf("the body did not survive.\n  sent: %q\n  back: %q", c.text, got)
			}
		})
	}
}

func TestTheMessageSurvivesBothWaysALinkCanBeRead(t *testing.T) {
	// A query string can be read two different ways, and this is the whole
	// problem:
	//
	//   - The HTML form convention, application/x-www-form-urlencoded, in which
	//     a plus sign means a space. That is what url.Values, and every web
	//     framework, applies.
	//   - Plain percent-decoding, RFC 3986, in which a plus sign means a plus
	//     sign and nothing else. That is what a chat client does with the text
	//     it has been handed in a link.
	//
	// A wa.me link is read the second way. So the text has to come back
	// identical under *both* readings, because the shopkeeper has no way of
	// knowing which one the thing at the other end applied. This encoder
	// manages that, because it leaves no plus sign and no raw space for either
	// reading to disagree about.
	for _, c := range awkwardText {
		if c.text == "" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			encoded := EncodeComponent(c.text)

			asForm, err := url.ParseQuery("text=" + encoded)
			if err != nil {
				t.Fatalf("read as a form, the query does not parse: %v", err)
			}
			if got := asForm.Get("text"); got != c.text {
				t.Fatalf("read as a form the message became %q, want %q", got, c.text)
			}

			asPercent, err := url.PathUnescape(encoded)
			if err != nil {
				t.Fatalf("read by plain percent-decoding, the query does not decode: %v", err)
			}
			if asPercent != c.text {
				t.Fatalf("read by plain percent-decoding the message became %q, want %q", asPercent, c.text)
			}
		})
	}
}

func TestTheNaiveEncodersAreWhyThisOneExists(t *testing.T) {
	// A live demonstration that the obvious choices corrupt an order, so that
	// nobody later "simplifies" the encoder back to one of them.
	//
	// url.QueryEscape writes a space as a plus sign. Read back by a form
	// decoder that is harmless, and read back by plain percent-decoding — which
	// is what the chat client does — every space in the message has become a
	// plus sign. "2 jars of mango pickle" arrives as
	// "2+jars+of+mango+pickle".
	cases := []string{
		"2 jars",
		"Chakli + chutney",
		"50% now + 50% on delivery",
		"Mango pickle\nLime pickle",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			naive, err := url.PathUnescape(url.QueryEscape(text))
			if err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if naive == text {
				t.Fatalf("QueryEscape round-tripped %q through plain percent-decoding unchanged; "+
					"the plus-for-space rule it applies is the whole reason this package has its own encoder", text)
			}
			if !strings.Contains(naive, "+") {
				t.Fatalf("QueryEscape corrupted %q into %q, but not in the expected way", text, naive)
			}

			ours, err := url.PathUnescape(EncodeComponent(text))
			if err != nil {
				t.Fatalf("decoding ours: %v", err)
			}
			if ours != text {
				t.Fatalf("our own encoder lost it too: %q became %q", text, ours)
			}
		})
	}

	// PathEscape is the other tempting choice, and it leaves in place the very
	// characters that end a query string.
	raw := url.PathEscape("Mango & Lime #14 50%")
	if !strings.Contains(raw, "&") {
		t.Fatalf("PathEscape encoded the ampersand in %q; the note about it is out of date", raw)
	}
}

func TestTheEncodedLinkCarriesNoRawDelimiterAtAll(t *testing.T) {
	// Belt and braces: whatever a reader does with the link, there must be no
	// character in the query that could end it, split it or be read as a space.
	for _, c := range awkwardText {
		if c.text == "" {
			continue
		}
		link, err := WhatsAppLink("639171234567", c.text, DefaultLimits())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		query := strings.TrimPrefix(link.URL, whatsAppBase+"639171234567?text=")

		for _, forbidden := range []string{" ", "+", "&", "#", "\n", "\r", "\t", "?", "=", "/", ";", "\"", "'", "<", ">", "\\", "`", "[", "]", "{", "}", "|", "^"} {
			if strings.Contains(query, forbidden) {
				t.Fatalf("%s: the encoded query holds a raw %q\n  %s", c.name, forbidden, link.URL)
			}
		}
		for _, r := range query {
			if r > 127 {
				t.Fatalf("%s: the encoded query holds a raw non-ASCII rune %q", c.name, r)
			}
		}
	}
}

func TestEncodeComponentLeavesOnlyTheUnreservedSetAlone(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"letters and digits and the four unreserved marks", "abcXYZ019-_.~", "abcXYZ019-_.~"},
		{"a space", "a b", "a%20b"},
		{"a plus sign", "a+b", "a%2Bb"},
		{"an ampersand", "a&b", "a%26b"},
		{"a hash", "a#b", "a%23b"},
		{"a percent sign", "50%", "50%25"},
		{"a newline", "a\nb", "a%0Ab"},
		{"a carriage return", "a\rb", "a%0Db"},
		{"a tab", "a\tb", "a%09b"},
		{"a slash", "a/b", "a%2Fb"},
		{"a rupee sign is three UTF-8 bytes", "\u20b9", "%E2%82%B9"},
		{"a peso sign is three UTF-8 bytes", "\u20b1", "%E2%82%B1"},
		{"a Devanagari letter", "\u0905", "%E0%A4%85"},
		{"a Tamil letter", "\u0bae", "%E0%AE%AE"},
		{"a Baybayin letter", "\u1702", "%E1%9C%82"},
		{"an emoji is four UTF-8 bytes", "\U0001f96d", "%F0%9F%A5%AD"},
		{"empty", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EncodeComponent(c.in); got != c.want {
				t.Fatalf("EncodeComponent(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestEncodeComponentUsesUpperCaseHexThroughout(t *testing.T) {
	// Lower-case triplets decode identically, but upper-case is what RFC 3986
	// §6.2.2.1 says to produce, and a link that is byte-identical every time it
	// is built is a link a shopkeeper can compare against the last one.
	got := EncodeComponent("\u20b9 \u0bae \U0001f96d \u00f1")
	if strings.ToUpper(got) != got {
		t.Fatalf("the encoder produced lower-case hexadecimal: %q", got)
	}
	// Three bytes for the rupee sign, three for the Tamil letter, four for the
	// emoji, two for the n-tilde, and one for each of the three spaces.
	if strings.Count(got, "%") != 3+1+3+1+4+1+2 {
		t.Fatalf("unexpected triplet count in %q", got)
	}
}

func TestALinkIsRefusedForANumberWaMeCannotUse(t *testing.T) {
	cases := []struct {
		name   string
		number E164
	}{
		{"nothing", ""},
		{"a leading plus", "+919876543210"},
		{"spaces inside", "91 98765 43210"},
		{"a leading zero", "09876543210"},
		{"far too short", "123"},
		{"only a country code", "91"},
		{"longer than E.164 allows", "1234567890123456"},
		{"letters", "91987654ABCD"},
		{"a plus in the middle", "919+876543210"},
		{"an extension marker", "919876543210x12"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			link, err := WhatsAppLink(c.number, "hello", DefaultLimits())
			if err == nil {
				t.Fatalf("a link was built for %q: %s", c.number, link.URL)
			}
			if link.URL != "" {
				t.Fatalf("a refused link still carries a URL: %s", link.URL)
			}
			if link.Text != "hello" {
				t.Fatalf("a refused link should still carry the message to copy, got %q", link.Text)
			}
		})
	}
}

func TestALinkTooLongToTrustIsFlaggedAndTheTextStillOffered(t *testing.T) {
	// Devanagari is the worst case: three bytes a character, nine characters of
	// link each. A 375-character message in Hindi is nearly 3,000 characters of
	// link, which is over the limit while looking like a short message.
	hindi := strings.Repeat("\u0905\u0924\u093e\u0930 ", 75) // five runes each
	link, err := WhatsAppLink("919876543210", hindi, DefaultLimits())
	if err != nil {
		t.Fatalf("building the link: %v", err)
	}
	if link.TextCharacters != 375 {
		t.Fatalf("the message is %d characters, want 375", link.TextCharacters)
	}
	if link.Safe {
		t.Fatalf("a %d-character link was called safe", link.Characters)
	}
	if link.Note == "" {
		t.Fatal("an over-long link carries no explanation")
	}
	if link.Text != hindi {
		t.Fatal("an over-long link does not carry the message to copy instead")
	}
	if link.URL == "" {
		t.Fatal("the link should still be built; a shop that knows its own client handles it may still use it")
	}

	// The same message in English is well inside the limit, which is the point:
	// the limit is on the link and not on the message.
	english := strings.Repeat("pickle ", 42) // 294 characters
	short, err := WhatsAppLink("919876543210", english, DefaultLimits())
	if err != nil {
		t.Fatalf("building the short link: %v", err)
	}
	if !short.Safe {
		t.Fatalf("a %d-character link of English was called unsafe", short.Characters)
	}
	if short.Note != "" {
		t.Fatalf("a safe link carries a note: %q", short.Note)
	}
}

func TestTheLengthLimitIsOverridable(t *testing.T) {
	text := strings.Repeat("a", 500)
	tight := Limits{SafeLinkCharacters: 100}
	link, err := WhatsAppLink("919876543210", text, tight)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if link.Safe {
		t.Fatal("a 500-character message passed a 100-character limit")
	}
	if !strings.Contains(link.Note, "100") {
		t.Fatalf("the note does not state the limit in force: %q", link.Note)
	}

	generous := Limits{SafeLinkCharacters: 100000}
	link, err = WhatsAppLink("919876543210", text, generous)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if !link.Safe {
		t.Fatal("a 500-character message failed a 100,000-character limit")
	}
}

func TestLimitsNormaliseToTheShippedFigures(t *testing.T) {
	got := Limits{}.Normalise()
	if got != DefaultLimits() {
		t.Fatalf("empty limits normalised to %+v, want %+v", got, DefaultLimits())
	}
	got = Limits{SafeLinkCharacters: -5, StatusCharacters: 0, MessageCharacters: 10}.Normalise()
	if got.SafeLinkCharacters != DefaultSafeLinkCharacters {
		t.Fatalf("a negative link limit became %d", got.SafeLinkCharacters)
	}
	if got.StatusCharacters != DefaultStatusCharacters {
		t.Fatalf("a zero status limit became %d", got.StatusCharacters)
	}
	if got.MessageCharacters != 10 {
		t.Fatalf("a deliberately small message limit was overridden to %d", got.MessageCharacters)
	}
}

func TestMailLinkRefusesSomethingThatIsNotAnAddress(t *testing.T) {
	cases := []string{"", "   ", "shop", "@example.com", "shop@", "shop @example.com", "a@b,c@d", "shop@example.com\nbcc:someone@else"}
	for _, address := range cases {
		if link, err := MailtoLink(address, "subject", "body", DefaultLimits()); err == nil {
			t.Errorf("%q was accepted as an address: %s", address, link.URL)
		}
	}
	if _, err := MailtoLink("shop@example.com", "subject", "body", DefaultLimits()); err != nil {
		t.Fatalf("a real address was refused: %v", err)
	}
}

func TestMailLinkOmitsAnEmptySubjectAndBody(t *testing.T) {
	link, err := MailtoLink("shop@example.com", "", "", DefaultLimits())
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if link.URL != "mailto:shop@example.com" {
		t.Fatalf("an empty mail link is %q", link.URL)
	}
}

func TestCopyOnlyCarriesTheMessageAndTheReason(t *testing.T) {
	link := CopyOnly("the bill", "there is no number for this customer", DefaultLimits())
	if link.URL != "" {
		t.Fatalf("a copy-only link has a URL: %q", link.URL)
	}
	if link.Text != "the bill" || link.TextCharacters != 8 {
		t.Fatalf("copy-only link: %+v", link)
	}
	if link.Safe {
		t.Fatal("a copy-only link should not claim to be a usable link")
	}
	if link.Note == "" {
		t.Fatal("a copy-only link gives no reason")
	}
}
