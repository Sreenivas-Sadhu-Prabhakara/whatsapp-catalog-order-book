package orderbook

import (
	"net/url"
	"strings"
	"testing"
)

func TestTheConfirmationCarriesEveryFigureACustomerWouldCheck(t *testing.T) {
	shop := testShop()
	order := soundOrder()
	order.Lines = append(order.Lines,
		LineFor(mustFind(t, testCatalogue(), "LIME-400"), Whole(1), "no chilli"))
	order.Paid = 50000
	order.PaymentNote = "UPI"

	text := OrderText(shop, order, Confirmation)

	for _, want := range []string{
		"Aaji's Kitchen",
		"Meera Kulkarni",
		"ORD-0001",
		"Thursday, 26 November 2026",
		"morning",
		"Mango pickle (500 g)",
		"Lime pickle (400 g)",
		"no chilli",
		"₹640.00", // two mango jars
		"₹260.00", // one lime jar
		"Delivery: ₹40.00",
		"Total: ₹940.00",
		"Received: ₹500.00",
		"Still to pay: ₹440.00",
		"UPI test@examplebank",
		"Flat 3, Kothrud",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the confirmation does not carry %q\n---\n%s\n---", want, text)
		}
	}

	// The lines a customer adds up have to come to the total in the message.
	var sum Money
	for _, l := range order.Lines {
		sum += l.Amount()
	}
	if sum+order.DeliveryFee != order.Add().Total {
		t.Fatalf("the lines come to %s and the total says %s", sum+order.DeliveryFee, order.Add().Total)
	}
}

func TestEachKindOfMessageSaysItsOwnThing(t *testing.T) {
	shop := testShop()
	order := soundOrder()
	order.Paid = 20000

	cases := []struct {
		kind    MessageKind
		carries string
	}{
		{Confirmation, "Thank you for your order"},
		{Ready, "on its way"},
		{Reminder, "balance"},
		{Receipt, "paid in full"},
	}
	for _, c := range cases {
		text := OrderText(shop, order, c.kind)
		if !strings.Contains(text, c.carries) {
			t.Errorf("the %s message does not say %q\n---\n%s\n---", c.kind, c.carries, text)
		}
	}

	// A pickup is to collect, not on its way.
	order.Fulfilment = Pickup
	if text := OrderText(shop, order, Ready); !strings.Contains(text, "collect") {
		t.Errorf("a pickup's ready message does not mention collecting:\n%s", text)
	}
}

func TestTheMessageSaysCollectionForAPickupAndDeliveryForADelivery(t *testing.T) {
	shop := testShop()
	order := soundOrder()

	if text := OrderText(shop, order, Confirmation); !strings.Contains(text, "Delivery: Thursday") {
		t.Errorf("a delivery's date line reads wrong:\n%s", text)
	}
	order.Fulfilment = Pickup
	if text := OrderText(shop, order, Confirmation); !strings.Contains(text, "Collection: Thursday") {
		t.Errorf("a pickup's date line reads wrong:\n%s", text)
	}
}

func TestAProductNameWithWhatsAppFormattingIsSentPlain(t *testing.T) {
	// WhatsApp reads * _ ~ and ` as formatting. A name carrying one of them
	// must not be wrapped in asterisks, or the customer gets stray asterisks
	// and half a bold line.
	shop := testShop()
	shop.Name = "Aaji's *Kitchen*"

	order := soundOrder()
	text := OrderText(shop, order, Confirmation)

	if strings.Contains(text, "**") {
		t.Fatalf("a name with an asterisk in it was wrapped in asterisks:\n%s", text)
	}
	if !strings.Contains(text, "Aaji's *Kitchen*") {
		t.Fatalf("the shop's own name was changed:\n%s", text)
	}

	// And a name with none of them is emphasised.
	shop.Name = "Aaji's Kitchen"
	if text := OrderText(shop, order, Confirmation); !strings.Contains(text, "*Aaji's Kitchen*") {
		t.Fatalf("a plain name was not emphasised:\n%s", text)
	}

	for _, name := range []string{"a*b", "a_b", "a~b", "a`b"} {
		if !hasWhatsAppFormatting(name) {
			t.Errorf("%q was not recognised as carrying formatting", name)
		}
		if bold(name) != name {
			t.Errorf("%q was wrapped: %q", name, bold(name))
		}
	}
	if bold("plain") != "*plain*" {
		t.Errorf("plain text was not wrapped: %q", bold("plain"))
	}
	if bold("") != "" {
		t.Error("an empty string was wrapped")
	}
}

func TestAMessageWithAwkwardTextStillMakesALinkThatRoundTrips(t *testing.T) {
	// The whole chain: a real order with a rupee sign, an ampersand, a hash, a
	// plus, a percent, an emoji and three scripts in it, through the message
	// builder, through the link builder, and back out of a real URL parser
	// identical.
	shop := testShop()
	shop.Name = "Aaji's Kitchen & Co. #2"
	shop.PayTo = "UPI aaji@bank — 50% now + 50% on delivery"
	shop.Closing = "अमराइ घरी बनवलेले \U0001f96d"

	order := soundOrder()
	order.Customer.Name = "மீரா — Piña"
	order.Customer.Address = "Flat #3, 50% Lane\nKothrud & Paud Road"
	order.Note = "Ring the bell twice — 2+2 jars if possible?"
	order.Lines[0].Note = "अतार 100% तिखट"

	text := OrderText(shop, order, Confirmation)
	for _, want := range []string{"&", "#", "%", "+", "₹", "\U0001f96d", "अ", "ம", "ñ"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the message lost %q, so this test is not testing what it says it is", want)
		}
	}

	link, err := WhatsAppLink("919876543211", text, shop.Limits)
	if err != nil {
		t.Fatalf("building the link: %v", err)
	}
	parsed, err := url.Parse(link.URL)
	if err != nil {
		t.Fatalf("the link does not parse: %v", err)
	}
	if got := parsed.Query().Get("text"); got != text {
		t.Fatalf("the message did not survive the link.\n  sent: %q\n  back: %q", text, got)
	}
	if got, err := url.PathUnescape(parsed.RawQuery[len("text="):]); err != nil || got != text {
		t.Fatalf("read by plain percent-decoding the message became %q (%v)", got, err)
	}
}

func TestMessagesForAnOrderCarryLinksAndSayWhatDoesNotApply(t *testing.T) {
	catalogue, shop := testCatalogue(), testShop()
	order := soundOrder()

	messages := MessagesFor(catalogue, shop, order)
	byKind := map[MessageKind]Message{}
	for _, m := range messages {
		byKind[m.Kind] = m
	}

	confirmation := byKind[Confirmation]
	if !confirmation.Available {
		t.Fatalf("there is no confirmation to send: %q", confirmation.Why)
	}
	if confirmation.Text == "" {
		t.Fatal("the confirmation has no text")
	}
	if confirmation.WhatsApp.URL == "" {
		t.Fatal("the confirmation has no chat link")
	}
	if !strings.HasPrefix(confirmation.WhatsApp.URL, "https://wa.me/919876543211?text=") {
		t.Fatalf("the chat link is %q", confirmation.WhatsApp.URL)
	}
	if !confirmation.WhatsApp.Safe {
		t.Fatalf("a short confirmation produced an unsafe link of %d characters", confirmation.WhatsApp.Characters)
	}

	// Nothing has been paid, so the whole bill is outstanding: a reminder
	// applies and a receipt does not, and the tool says why rather than
	// offering an empty message.
	if !byKind[Reminder].Available {
		t.Fatalf("no reminder on an unpaid order: %q", byKind[Reminder].Why)
	}
	if byKind[Receipt].Available {
		t.Fatal("a receipt was offered on an unpaid order")
	}
	if byKind[Receipt].Why == "" {
		t.Error("the receipt is unavailable with no reason given")
	}

	// Part-paid: the reminder applies for the balance, the receipt does not.
	order.Paid = 20000
	byKind = map[MessageKind]Message{}
	for _, m := range MessagesFor(catalogue, shop, order) {
		byKind[m.Kind] = m
	}
	if !byKind[Reminder].Available {
		t.Fatalf("no reminder on a part-paid order: %q", byKind[Reminder].Why)
	}
	if !strings.Contains(byKind[Reminder].Text, "Still to pay: ₹480.00") {
		t.Fatalf("the reminder does not state the balance:\n%s", byKind[Reminder].Text)
	}
	if byKind[Receipt].Available {
		t.Fatal("a receipt was offered on a part-paid order")
	}

	// Paid in full: the receipt applies, the reminder does not.
	order.Paid = order.Add().Total
	byKind = map[MessageKind]Message{}
	for _, m := range MessagesFor(catalogue, shop, order) {
		byKind[m.Kind] = m
	}
	if byKind[Reminder].Available {
		t.Fatal("a reminder was offered on an order with nothing left to pay")
	}
	if !byKind[Receipt].Available {
		t.Fatalf("no receipt on a paid order: %q", byKind[Receipt].Why)
	}
}

func TestAnOrderWithSomethingWrongGetsNoCustomerMessage(t *testing.T) {
	catalogue, shop := testCatalogue(), testShop()

	order := soundOrder()
	order.Lines = append(order.Lines, LineFor(mustFind(t, catalogue, "PODI-200"), Whole(1), ""))

	for _, m := range MessagesFor(catalogue, shop, order) {
		if m.Available {
			t.Fatalf("the %s message was offered for an order with a sold-out line:\n%s", m.Kind, m.Text)
		}
		if m.Text != "" {
			t.Fatalf("the %s message carries text for a blocked order", m.Kind)
		}
		if m.WhatsApp.URL != "" {
			t.Fatalf("the %s message carries a link for a blocked order", m.Kind)
		}
		if m.Why == "" {
			t.Fatalf("the %s message is unavailable with no reason", m.Kind)
		}
	}

	// The record itself is untouched: a shop writes an order down as it is
	// spoken and tidies it afterwards.
	if len(order.Lines) != 2 {
		t.Fatal("the order lost a line")
	}
	if order.Add().Subtotal != 64000+18000 {
		t.Fatalf("the order's own figures came to %s", order.Add().Subtotal)
	}
}

func TestACustomerWithNoNumberStillGetsAMessageToCopy(t *testing.T) {
	catalogue, shop := testCatalogue(), testShop()
	order := soundOrder()
	order.Customer.Phone = ""

	var confirmation Message
	for _, m := range MessagesFor(catalogue, shop, order) {
		if m.Kind == Confirmation {
			confirmation = m
		}
	}
	if !confirmation.Available {
		t.Fatalf("no confirmation at all: %q", confirmation.Why)
	}
	if confirmation.Text == "" {
		t.Fatal("there is no text to copy")
	}
	if confirmation.WhatsApp.URL != "" {
		t.Fatalf("a link was built with no number: %q", confirmation.WhatsApp.URL)
	}
	if confirmation.WhatsApp.Text != confirmation.Text {
		t.Fatal("the copy-only link does not carry the message")
	}
	if confirmation.WhatsApp.Note == "" {
		t.Fatal("the copy-only link gives no reason")
	}
	if confirmation.Mail.Note == "" {
		t.Fatal("the mail link gives no reason for being unavailable")
	}
}

func TestACustomerWithAnEmailGetsAMailLink(t *testing.T) {
	catalogue, shop := testCatalogue(), testShop()
	order := soundOrder()
	order.Customer.Email = "meera@example.co.in"

	var confirmation Message
	for _, m := range MessagesFor(catalogue, shop, order) {
		if m.Kind == Confirmation {
			confirmation = m
		}
	}
	if !strings.HasPrefix(confirmation.Mail.URL, "mailto:meera@example.co.in?") {
		t.Fatalf("the mail link is %q", confirmation.Mail.URL)
	}

	parsed, err := url.Parse(confirmation.Mail.URL)
	if err != nil {
		t.Fatalf("the mail link does not parse: %v", err)
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		t.Fatalf("the mail link's query does not parse: %v", err)
	}
	if got := query.Get("body"); got != confirmation.Text {
		t.Fatalf("the body is not the message:\n  want: %q\n   got: %q", confirmation.Text, got)
	}
	if subject := query.Get("subject"); !strings.Contains(subject, "ORD-0001") || !strings.Contains(subject, "Aaji's Kitchen") {
		t.Fatalf("the subject is %q", subject)
	}
}

func TestTheDaySheetTextIsForTheKitchenAndCarriesNoCustomerNumber(t *testing.T) {
	on := Date{2026, 11, 26}
	shop := testShop()
	sheet := Day(testCatalogue(), dayOrders(on), on)

	text := DayText(shop, sheet)

	for _, want := range []string{
		"Thursday, 26 November 2026",
		"To make",
		"14 × Mango pickle (500 g)",
		"2 orders",
		"marked sold out",
		"Going out",
		"Cancelled, do not make",
		"to collect",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the day sheet does not carry %q\n---\n%s\n---", want, text)
		}
	}

	// No customer's phone number is in a sheet that may go to a helper.
	for _, number := range []string{"9876543211", "9876543212", "9876543213"} {
		if strings.Contains(text, number) {
			t.Errorf("the day sheet carries a customer's number %s", number)
		}
	}

	message := DayMessage(shop, sheet)
	if message.Text != text {
		t.Fatal("the day message is not the day text")
	}
	// It goes to the shop's own number, not a customer's.
	if !strings.HasPrefix(message.WhatsApp.URL, "https://wa.me/919876543210?text=") {
		t.Fatalf("the day sheet's link is %q", message.WhatsApp.URL)
	}

	parsed, err := url.Parse(message.WhatsApp.URL)
	if err != nil {
		t.Fatalf("the day sheet's link does not parse: %v", err)
	}
	if got := parsed.Query().Get("text"); got != text {
		t.Fatalf("the day sheet did not survive its own link")
	}
}

func TestADayWithNothingOnItSaysSo(t *testing.T) {
	sheet := Day(testCatalogue(), nil, Date{2026, 11, 26})
	text := DayText(testShop(), sheet)
	if !strings.Contains(text, "Nothing to make") {
		t.Fatalf("an empty day reads:\n%s", text)
	}
}

func TestMessagesNeverRunTwoBlankLinesTogether(t *testing.T) {
	// A message with a double blank line in it looks like a mistake, and the
	// builder is the only thing stopping one.
	shop := testShop()
	shop.Closing = ""
	shop.PayTo = ""

	order := soundOrder()
	order.Note = ""
	order.Ref = ""
	order.Wanted = Date{}
	order.Customer.Address = ""

	for _, kind := range []MessageKind{Confirmation, Ready, Reminder, Receipt} {
		text := OrderText(shop, order, kind)
		if strings.Contains(text, "\n\n\n") {
			t.Errorf("the %s message runs blank lines together:\n%q", kind, text)
		}
		if strings.HasPrefix(text, "\n") || strings.HasSuffix(text, "\n") {
			t.Errorf("the %s message starts or ends on a blank line:\n%q", kind, text)
		}
	}
}

func TestCharacterCountsCountCharactersAndNotBytes(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"₹100", 4},
		{"अतार", 4},
		{"மாங்", 4},
		{"\U0001f96d", 1},
	}
	for _, c := range cases {
		if got := characters(c.text); got != c.want {
			t.Errorf("characters(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}
