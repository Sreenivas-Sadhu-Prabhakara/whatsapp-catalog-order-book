package orderbook

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// WhatsApp reads four characters in a message as formatting: an asterisk for
// bold, an underscore for italic, a tilde for strikethrough and a backtick for
// monospace. A shop that sells "Kolhapuri ~extra hot~ chutney" and has the tool
// wrap that name in asterisks gets a customer message with the words struck
// through and the asterisks showing. So a name carrying one of them is sent
// without emphasis, and the catalogue reports it as something to look at.
const whatsAppFormattingCharacters = "*_~`"

// hasWhatsAppFormatting reports whether text carries a character WhatsApp would
// read as formatting.
func hasWhatsAppFormatting(text string) bool {
	return strings.ContainsAny(text, whatsAppFormattingCharacters)
}

// bold wraps text so WhatsApp shows it in bold, unless the text itself carries
// a formatting character, in which case it is left plain — a half-bold line
// with stray asterisks in it reads worse than a plain one.
func bold(text string) string {
	if text == "" || hasWhatsAppFormatting(text) {
		return text
	}
	return "*" + text + "*"
}

// builder assembles a message line by line.
type builder struct {
	lines []string
}

// line adds one line of text exactly as it is.
func (b *builder) line(text string) { b.lines = append(b.lines, text) }

// addf adds one line built from a format.
func (b *builder) addf(format string, args ...any) {
	b.lines = append(b.lines, fmt.Sprintf(format, args...))
}

// blank adds an empty line, but never two in a row and never one at the start.
func (b *builder) blank() {
	if len(b.lines) == 0 || b.lines[len(b.lines)-1] == "" {
		return
	}
	b.lines = append(b.lines, "")
}

func (b *builder) String() string {
	out := strings.Join(b.lines, "\n")
	return strings.TrimRight(out, "\n ")
}

// MessageKind is which message is being built.
type MessageKind string

const (
	// Confirmation is the message that goes out when an order is taken: what
	// was ordered, what it comes to, and when it is coming.
	Confirmation MessageKind = "confirmation"
	// Ready is the message that goes out when it is made.
	Ready MessageKind = "ready"
	// Reminder is the message that asks for what is still owed.
	Reminder MessageKind = "reminder"
	// Receipt is the message that goes out when it has been paid for in full.
	Receipt MessageKind = "receipt"
)

// Message is one piece of text with the links to send it by.
type Message struct {
	Kind     MessageKind `json:"kind"`
	Title    string      `json:"title"`
	Text     string      `json:"text"`
	WhatsApp Link        `json:"whatsapp"`
	Mail     Link        `json:"mail"`
	// Available is false where this message does not apply to this order — a
	// payment reminder on an order with nothing left to pay.
	Available bool `json:"available"`
	// Why says why it does not apply.
	Why string `json:"why,omitempty"`
}

// OrderText writes the message that goes to the customer about an order.
//
// Nothing here is sent. The text and the links are handed to the shopkeeper,
// who reads them and sends them from their own phone, under their own name.
// This tool has no messaging account, holds no credentials and makes no network
// call of any kind.
func OrderText(shop Shop, o Order, kind MessageKind) string {
	shop = shop.Normalise()
	o = o.Normalise()
	totals := o.Add()
	currency := shop.Currency

	var b builder

	if shop.Name != "" {
		b.line(bold(shop.Name))
		b.blank()
	}

	if o.Customer.Name != "" {
		b.addf("Hello %s,", o.Customer.Name)
		b.blank()
	}

	switch kind {
	case Ready:
		if o.Fulfilment.Normalise() == Pickup {
			b.line("Your order is ready to collect.")
		} else {
			b.line("Your order is ready and on its way.")
		}
	case Reminder:
		b.line("A reminder about the balance on your order.")
	case Receipt:
		b.line("Thank you — your order is paid in full.")
	default:
		b.line("Thank you for your order. Here it is, so you can check it.")
	}
	b.blank()

	if o.Ref != "" {
		b.addf("Order %s", o.Ref)
	}
	if !o.Wanted.IsZero() {
		word := "Delivery"
		if o.Fulfilment.Normalise() == Pickup {
			word = "Collection"
		}
		if o.Slot.Normalise() == SlotAny {
			b.addf("%s: %s", word, o.Wanted.Pretty())
		} else {
			b.addf("%s: %s, %s", word, o.Wanted.Pretty(), o.Slot.Name())
		}
	}
	b.blank()

	// The lines, then the figures, in the order a customer reads them.
	for _, l := range o.Lines {
		if l.Qty <= 0 {
			continue
		}
		line := fmt.Sprintf("%s × %s — %s", l.Qty, l.Label(), l.Amount().Format(currency))
		if l.Note != "" {
			line += " (" + l.Note + ")"
		}
		b.line(line)
	}

	b.blank()
	if totals.DeliveryFee != 0 || totals.Discount != 0 {
		b.addf("Items: %s", totals.Subtotal.Format(currency))
		if totals.DeliveryFee != 0 {
			b.addf("Delivery: %s", totals.DeliveryFee.Format(currency))
		}
		if totals.Discount != 0 {
			reason := o.DiscountReason
			if reason != "" {
				reason = " (" + reason + ")"
			}
			b.addf("Discount: -%s%s", totals.Discount.Format(currency), reason)
		}
	}
	b.line(bold("Total: " + totals.Total.Format(currency)))

	if totals.Paid > 0 {
		b.addf("Received: %s", totals.Paid.Format(currency))
		if totals.Balance > 0 {
			b.line(bold("Still to pay: " + totals.Balance.Format(currency)))
		} else {
			b.line("Paid in full. Thank you.")
		}
	}

	if totals.Balance > 0 && shop.PayTo != "" && kind != Receipt {
		b.blank()
		b.addf("To pay: %s", shop.PayTo)
	}

	if o.Note != "" {
		b.blank()
		b.line(o.Note)
	}

	if o.Fulfilment.Normalise() == Delivery && o.Customer.Address != "" && kind != Receipt {
		b.blank()
		b.addf("Delivering to: %s", strings.ReplaceAll(o.Customer.Address, "\n", ", "))
	}

	if shop.Closing != "" {
		b.blank()
		b.line(shop.Closing)
	}
	if shop.Name != "" {
		b.blank()
		b.addf("— %s", shop.Name)
		if shop.Area != "" {
			b.lines[len(b.lines)-1] += ", " + shop.Area
		}
	}

	return b.String()
}

// MessagesFor builds every message that applies to an order, each with its
// links, or says why one does not apply.
//
// An order with a problem that stops it gets no customer message at all. The
// record is kept either way: a shop writes an order down as it is spoken and
// tidies it afterwards, and the book must not refuse to hold it. What it
// refuses to do is hand over a message promising a customer something the order
// cannot deliver.
func MessagesFor(c Catalogue, shop Shop, o Order) []Message {
	shop = shop.Normalise()
	o = o.Normalise()
	totals := o.Add()
	problems := o.Problems(c, shop)

	kinds := []struct {
		kind  MessageKind
		title string
		apply bool
		why   string
	}{
		{Confirmation, "Confirm the order", true, ""},
		{Ready, "Say it is ready", o.Status.Normalise() != Cancelled,
			"this order has been cancelled"},
		{Reminder, "Ask for the balance", totals.Balance > 0 && o.Status.Normalise() != Cancelled,
			"there is nothing left to pay on this order"},
		{Receipt, "Confirm it is paid", totals.Paid > 0 && totals.Balance <= 0,
			"this order has not been paid in full"},
	}

	out := make([]Message, 0, len(kinds))
	for _, k := range kinds {
		message := Message{Kind: k.kind, Title: k.title, Available: k.apply, Why: ""}
		if !k.apply {
			message.Why = k.why
			out = append(out, message)
			continue
		}
		if blocked(problems) {
			message.Available = false
			message.Why = "this order has something on it that has to be fixed first"
			out = append(out, message)
			continue
		}

		message.Text = OrderText(shop, o, k.kind)
		message.WhatsApp = linkTo(o.Customer.Phone, shop, message.Text)
		message.Mail = mailTo(o.Customer.Email, shop, subjectFor(shop, o, k.kind), message.Text)
		out = append(out, message)
	}
	return out
}

// subjectFor is the subject line of the email version.
func subjectFor(shop Shop, o Order, kind MessageKind) string {
	what := "Your order"
	switch kind {
	case Ready:
		what = "Your order is ready"
	case Reminder:
		what = "Balance on your order"
	case Receipt:
		what = "Your order is paid"
	}
	if o.Ref != "" {
		what += " " + o.Ref
	}
	if shop.Name != "" {
		what += " — " + shop.Name
	}
	return what
}

// linkTo builds the chat link for a number as typed, or a copy-only offering
// with the reason there is no link.
func linkTo(phone string, shop Shop, text string) Link {
	if strings.TrimSpace(phone) == "" {
		return CopyOnly(text, "there is no number for this customer — copy the message instead", shop.Limits)
	}
	number, err := NormalisePhone(phone, shop.DialCode)
	if err != nil {
		return CopyOnly(text, err.Error(), shop.Limits)
	}
	link, err := WhatsAppLink(number, text, shop.Limits)
	if err != nil {
		return CopyOnly(text, err.Error(), shop.Limits)
	}
	return link
}

// mailTo builds the mail link, or a copy-only offering.
func mailTo(address string, shop Shop, subject, text string) Link {
	if strings.TrimSpace(address) == "" {
		return CopyOnly(text, "there is no email address for this customer", shop.Limits)
	}
	link, err := MailtoLink(address, subject, text, shop.Limits)
	if err != nil {
		return CopyOnly(text, err.Error(), shop.Limits)
	}
	return link
}

// DayText writes the day's own sheet as text, for the shopkeeper's own notes or
// to send to whoever is helping in the kitchen. It is the one message here that
// is not for a customer, and it carries no customer's phone number.
func DayText(shop Shop, sheet DaySheet) string {
	shop = shop.Normalise()
	currency := shop.Currency

	var b builder
	b.line(bold(sheet.Pretty))
	if shop.Name != "" {
		b.line(shop.Name)
	}
	b.blank()

	if len(sheet.Make) == 0 {
		b.line("Nothing to make.")
	} else {
		b.line(bold("To make"))
		for _, m := range sheet.Make {
			label := m.Name
			if m.Pack != "" {
				label += " (" + m.Pack + ")"
			}
			line := fmt.Sprintf("%s × %s", m.Qty, label)
			if m.Orders > 1 {
				line += fmt.Sprintf(" — %d orders", m.Orders)
			}
			if m.SoldOut {
				line += " — marked sold out in the catalogue"
			}
			b.line(line)
		}
	}

	if len(sheet.Orders) > 0 {
		b.blank()
		b.line(bold("Going out"))
		for _, row := range sheet.Orders {
			line := fmt.Sprintf("%s — %s, %s %s", row.SlotName, row.Customer, row.Where, row.Total.Format(currency))
			if row.Balance > 0 {
				line += fmt.Sprintf(" (collect %s)", row.Balance.Format(currency))
			} else {
				line += " (paid)"
			}
			if row.Ref != "" {
				line += " [" + row.Ref + "]"
			}
			b.line(line)
		}
	}

	if len(sheet.Cancelled) > 0 {
		b.blank()
		b.line(bold("Cancelled, do not make"))
		for _, row := range sheet.Cancelled {
			b.addf("%s — %s", row.Customer, row.Ref)
		}
	}

	b.blank()
	b.addf("%d order(s), %s to collect of %s",
		sheet.Totals.Orders, sheet.Totals.ToCollect.Format(currency), sheet.Totals.Value.Format(currency))

	return b.String()
}

// DayMessage is the day's sheet with the shop's own chat link, so it can be
// sent to whoever is helping.
func DayMessage(shop Shop, sheet DaySheet) Message {
	shop = shop.Normalise()
	text := DayText(shop, sheet)

	message := Message{
		Kind:      "day",
		Title:     "The day's sheet",
		Text:      text,
		Available: true,
		Mail:      CopyOnly(text, "the day's sheet is for the shop, not for a customer", shop.Limits),
	}
	message.WhatsApp = linkTo(shop.Phone, shop, text)
	if shop.Email != "" {
		message.Mail = mailTo(shop.Email, shop, "Orders for "+sheet.On.String(), text)
	}
	return message
}

// characters counts what a person would call characters, which is runes and not
// bytes. A bill in Tamil is three bytes a character; counted in bytes it would
// look three times as long as it is.
func characters(text string) int { return utf8.RuneCountInString(text) }
