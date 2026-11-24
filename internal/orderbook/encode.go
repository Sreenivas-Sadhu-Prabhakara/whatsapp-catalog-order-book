package orderbook

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The links are the product.
//
// A shop that takes its orders in a chat thread sends the same four things over
// and over: what was ordered, what it comes to, when it is coming, and how to
// pay. Typing those out by hand is where the mistakes are — a missing line, a
// total that does not match the lines, a date written 11-12 and read as the
// 12th of November.
//
// So the tool builds the message from the order book and hands over a link that
// opens the customer's own chat with the message already in it. The shopkeeper
// reads it, taps send, and the message that goes out is the one the book
// holds.
//
// Which puts the whole weight of the tool on one function: percent-encoding.
// Get it wrong and nothing visibly breaks. The link opens, the chat opens, and
// the message is subtly not what the book says — a plus sign read back as a
// space, an ampersand that ended the message early, a total that lost its
// rupee sign. Nobody reports that. They just get the wrong jar.

// unreservedCharacters are the characters RFC 3986 §2.3 says never need
// encoding anywhere in a URI, and so the only ones this encoder leaves alone.
// Everything else goes out as a percent-triplet.
//
// Encoding more than strictly necessary is always safe. Encoding less is not,
// and the characters that are "usually fine" in a query string are exactly the
// ones a shop's own text is full of: & in "Mango & Lime", # in "Order #14",
// + in "Add-ons +2 jars", % in "20% off", and a newline between every line of
// a bill.
const unreservedCharacters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~"

const upperHex = "0123456789ABCDEF"

// EncodeComponent percent-encodes text for use as one component of a URL, to
// RFC 3986: every byte outside the unreserved set becomes %XX in upper-case
// hexadecimal, over the text's UTF-8 bytes.
//
// It is deliberately not the standard library's query escaper, which writes a
// space as a plus sign. That is correct for an HTML form submission, where the
// reader at the other end turns the plus back into a space, and wrong here: a
// chat client handed the text in a link applies plain percent-decoding, in
// which a plus sign is a plus sign. Every space in "2 jars of mango pickle"
// then arrives as a plus, and the customer is sent
// "2+jars+of+mango+pickle".
//
// A space therefore goes out as %20, which both readings agree on, and a plus
// sign that really is in the shop's own text goes out as %2B and comes back as
// a plus sign either way. The encoder leaves nothing for the two conventions to
// disagree about.
func EncodeComponent(text string) string {
	var out strings.Builder
	out.Grow(len(text) * 3)

	for i := 0; i < len(text); i++ {
		b := text[i]
		if strings.IndexByte(unreservedCharacters, b) >= 0 {
			out.WriteByte(b)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(upperHex[b>>4])
		out.WriteByte(upperHex[b&0x0f])
	}
	return out.String()
}

// Limits are the lengths this build assumes. None of them is a law or a
// published guarantee: they are what the platforms and the browsers are
// observed to accept, and WhatsApp has changed its own figures before. They are
// defined here once, stated in the documentation, and every one of them is
// overridable from the settings page, so a shop is never stuck behind a figure
// this build happened to ship with.
type Limits struct {
	// SafeLinkCharacters is how long a link may be before this tool stops
	// offering it and offers the text to copy instead. Two thousand characters
	// is inside what every browser, chat client and QR reader in common use
	// handles; above it, behaviour stops being predictable and the failure is
	// silent — the link simply does not open.
	SafeLinkCharacters int `json:"safeLinkCharacters"`

	// StatusCharacters is how much text a WhatsApp status holds. A catalogue
	// longer than this has to go out as a message or an image instead.
	StatusCharacters int `json:"statusCharacters"`

	// MessageCharacters is how much text one WhatsApp message holds.
	MessageCharacters int `json:"messageCharacters"`
}

// These are the figures this build ships with.
const (
	// DefaultSafeLinkCharacters is the length of link this tool will offer.
	DefaultSafeLinkCharacters = 2000
	// DefaultStatusCharacters is the text a WhatsApp status is taken to hold.
	DefaultStatusCharacters = 700
	// DefaultMessageCharacters is the text one message is taken to hold.
	DefaultMessageCharacters = 65536
)

// DefaultLimits are the figures this build ships with.
func DefaultLimits() Limits {
	return Limits{
		SafeLinkCharacters: DefaultSafeLinkCharacters,
		StatusCharacters:   DefaultStatusCharacters,
		MessageCharacters:  DefaultMessageCharacters,
	}
}

// Normalise replaces a missing or impossible figure with the one this build
// ships with, so a hand-edited data file cannot leave the tool refusing every
// link.
func (l Limits) Normalise() Limits {
	if l.SafeLinkCharacters <= 0 {
		l.SafeLinkCharacters = DefaultSafeLinkCharacters
	}
	if l.StatusCharacters <= 0 {
		l.StatusCharacters = DefaultStatusCharacters
	}
	if l.MessageCharacters <= 0 {
		l.MessageCharacters = DefaultMessageCharacters
	}
	return l
}

// Link is a link the shopkeeper can open, with everything needed to decide
// whether to open it or to copy the text instead.
type Link struct {
	// URL is the link itself, built whether or not it is within the length
	// this build will vouch for — a shop that knows its own client handles a
	// longer one should not be stopped.
	URL string `json:"url"`
	// Characters is how long the link is.
	Characters int `json:"characters"`
	// Safe reports whether the link is inside the length limit.
	Safe bool `json:"safe"`
	// Text is the message the link carries, for copying instead.
	Text string `json:"text"`
	// TextCharacters is how long the message is, counted in characters and not
	// in bytes: a bill in Tamil is three bytes a character and a quarter of
	// the length it looks.
	TextCharacters int `json:"textCharacters"`
	// Note says what is wrong, where something is.
	Note string `json:"note,omitempty"`
}

// whatsAppBase is where a wa.me link points. It is a link, not a call: nothing
// in this tool contacts it, and nothing is sent until a person taps it.
const whatsAppBase = "https://wa.me/"

// WhatsAppLink builds the link that opens a chat with a number, with the
// message already written.
//
// The number goes in the path and must be digits only with the country code
// first and no plus sign, no spaces and no leading zero — that is what wa.me
// takes, and a number in any other shape opens an error page rather than a
// chat. Use NormalisePhone to get one.
func WhatsAppLink(number E164, text string, limits Limits) (Link, error) {
	limits = limits.Normalise()
	if err := number.Validate(); err != nil {
		return Link{Text: text, TextCharacters: utf8.RuneCountInString(text)}, err
	}

	url := whatsAppBase + string(number)
	if text != "" {
		url += "?text=" + EncodeComponent(text)
	}

	link := Link{
		URL:            url,
		Characters:     len(url),
		Safe:           len(url) <= limits.SafeLinkCharacters,
		Text:           text,
		TextCharacters: utf8.RuneCountInString(text),
	}
	if !link.Safe {
		link.Note = fmt.Sprintf(
			"this link is %d characters, over the %d this tool will vouch for — copy the message and paste it into the chat instead",
			link.Characters, limits.SafeLinkCharacters)
	}
	if link.TextCharacters > limits.MessageCharacters {
		link.Note = fmt.Sprintf("this message is %d characters, over the %d one message holds",
			link.TextCharacters, limits.MessageCharacters)
	}
	return link, nil
}

// MailtoLink builds a mail link with the subject and the body already written,
// for a customer who orders by email. The address goes through the same
// encoder, because a local part may legitimately hold characters a URL reads as
// punctuation.
func MailtoLink(address, subject, body string, limits Limits) (Link, error) {
	limits = limits.Normalise()
	address = strings.TrimSpace(address)
	if address == "" {
		return Link{Text: body, TextCharacters: utf8.RuneCountInString(body)}, errors.New("there is no email address to write to")
	}
	at := strings.IndexByte(address, '@')
	if at <= 0 || at == len(address)-1 || strings.ContainsAny(address, " \t\r\n,;") {
		return Link{Text: body, TextCharacters: utf8.RuneCountInString(body)},
			fmt.Errorf("%q is not an email address", address)
	}

	// The address is split at the @ and each half encoded on its own: the @
	// itself has to survive as an @, or no mail client recognises the link.
	url := "mailto:" + EncodeComponent(address[:at]) + "@" + EncodeComponent(address[at+1:])

	var query []string
	if subject != "" {
		query = append(query, "subject="+EncodeComponent(subject))
	}
	if body != "" {
		query = append(query, "body="+EncodeComponent(body))
	}
	if len(query) > 0 {
		url += "?" + strings.Join(query, "&")
	}

	link := Link{
		URL:            url,
		Characters:     len(url),
		Safe:           len(url) <= limits.SafeLinkCharacters,
		Text:           body,
		TextCharacters: utf8.RuneCountInString(body),
	}
	if !link.Safe {
		link.Note = fmt.Sprintf(
			"this link is %d characters, over the %d this tool will vouch for — copy the message into the email instead",
			link.Characters, limits.SafeLinkCharacters)
	}
	return link, nil
}

// CopyOnly is what the tool offers where there is no number to link to: the
// message, ready to copy, and the reason there is no link.
func CopyOnly(text, why string, limits Limits) Link {
	limits = limits.Normalise()
	return Link{
		Text:           text,
		TextCharacters: utf8.RuneCountInString(text),
		Safe:           false,
		Note:           why,
	}
}
