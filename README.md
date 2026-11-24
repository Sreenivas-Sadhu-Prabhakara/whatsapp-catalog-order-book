# Product catalogue and order book for a small shop

A free tool that keeps a small shop's catalogue and its order book on the
shop's own machine, adds everything up, and hands over the message a customer
needs **already written** — with a correctly encoded prefilled chat link beside
it.

A home kitchen takes its orders in a chat thread. The orders work; the thread
does not. There is no running total, no list of what tomorrow needs, and no
record of what each order was agreed at — and the message that has to go back
(what was ordered, what it comes to, when it is coming, how to pay) is typed out
by hand every single time, which is where the mistakes are.

## It sends nothing

**This tool cannot send a message, and does not try to.** It has no messaging
account, no payment account, no API key and no network connection of any kind:
it makes no outbound request, ever.

What it produces is text and links. You open the link, read what it is about to
send, and **you send it** yourself, from your own phone, under your own name.
Nothing is sent without you, and nothing can be — there is nothing in the binary
that could.

The same goes for money. "How a customer pays" is a line of text you write —
your UPI identifier, your GCash number, "cash on delivery". "Received" is what
you tell the tool arrived. It is connected to no payment provider and collects
nothing.

## What it does

**A catalogue.** A short code, a name, a price, a pack size, whether it is on the
shelf or made to order and how much notice it needs, the smallest quantity you
will make, and a note. From the pack size it works out the price per kilogramme,
per litre or per piece — the figure that makes a 500 g jar at ₹320 and a 1 kg jar
at ₹580 comparable.

**An order book.** Who ordered what, for which day and which part of it, delivery
or pickup, what it came to, what has been paid, and where the order has got to.
Running totals per order and per day.

**The messages.** For each order: a confirmation, a note that it is ready, a
request for the balance, a receipt. Each built from the book, each with a
`wa.me` link that opens the customer's chat with the message in it, each with an
email link and the text to copy as well.

**A shareable catalogue.** One block of text for a broadcast, a status or a
group, rebuilt from the catalogue every time it is asked for — because a price
list kept as text goes stale the moment a price moves.

**A day's sheet.** What to make, added up across every order: not five orders to
read through but "fourteen jars of mango, six packets of chakli, one box of
laddu". Then what goes out, in the order it goes out, with the money to collect
from each.

### The link is the product

Everything else is bookkeeping. The part that goes wrong silently is the link.

A `wa.me` link carries the message in its query string, and a query string can
be read two ways: the HTML form convention, in which a plus sign means a space,
and plain percent-decoding (RFC 3986), in which a plus sign means a plus sign. A
chat client applies the second. So encoding a space as a plus sign — the obvious
thing to do, and what the standard library's query escaper does — turns *every
space in the message into a plus sign*. "2 jars of mango pickle" arrives as
"2+jars+of+mango+pickle". Nothing errors. The link opens, the chat opens, and the
message is wrong.

This tool encodes to **RFC 3986 §2.3** and nothing less: every byte outside
`A–Z a–z 0–9 - _ . ~` becomes a percent-triplet in upper-case hexadecimal, over
the text's UTF-8 bytes. A space is `%20`; a plus sign is `%2B`. Nothing is left
for the two conventions to disagree about.

Which matters because a shop's own text is made of exactly the characters that
break a link: `&` in "Mango & Lime", `#` in "Order #14", `%` in "20% off", `+` in
"2+2 free", a newline between every line of a bill, a ₹ or ₱ on every figure, an
emoji at the end, and product names in Devanagari, Tamil or Tagalog. Every one of
those is in the test suite, built into a link and then **parsed back out with the
standard library's own URL parser under both readings**, required to come back
identical character for character.

Length is the other silent failure: a link that is too long does not error, it
just does not open. A character of Devanagari is nine characters of encoded link,
so a 375-character message in Hindi is nearly 3,000 characters of link while
looking short. The tool counts it, says so, and offers the text to copy instead.

### The rule the book rests on

A shop puts its prices up. **An order taken last week is still owed at last
week's price.**

So an order line does not point at a catalogue price — it carries its own copy,
of the price and of the name, the pack size and the unit as they were when the
order was taken. Change the catalogue, or delete the product, and no bill already
given moves.

What the tool does instead is tell you: for every line whose catalogue price has
since changed, the price it was taken at, the price now, and the difference.
Honouring the old price or ringing the customer is your decision; the tool's job
is to make sure you know there is one.

### What stops a message

An order with something wrong with it is still kept — a shop writes an order down
as it is spoken and tidies it afterwards. What is refused is the *message*,
because a message is what the customer holds you to.

Stopped: a line for something sold out; a line for no quantity, or a negative
one; a line below the smallest you will make; a line at no price; a made-to-order
item wanted sooner than its notice allows; a day before the order was taken; an
order with nothing on it; no customer name; a delivery with no address; a
discount larger than the bill; more money received than is owed.

Flagged: a price that has moved since the order was taken; a product no longer in
the catalogue; no phone number, or one that cannot make a link; a number that
does not look like a mobile; no day set; a name carrying a WhatsApp formatting
character; two products sharing a code.

## Who it's for

The shop that takes its orders in a chat thread and has outgrown it: a home
baker, a pickle or snack maker, a tiffin seller, a small shop with a delivery
round, in India or the Philippines. Anybody losing an order a week in a
scroll-back, or who cannot answer "what do I owe you?" without reading four days
of messages.

## The standards it implements

No statute governs a shop's own catalogue or its own order book, and this tool
invents none. What it implements precisely is the handful of technical standards
that decide whether a link works:

- **RFC 3986** (*Uniform Resource Identifier: Generic Syntax*) — §2.3 unreserved
  characters, §2.1 percent-encoding over octets, §6.2.2.1 upper-case hexadecimal.
- **ITU-T Recommendation E.164** — a number is at most **15 digits** including the
  country code, written without a prefix, and never begins with a zero.
- **ISO 4217** — both the Indian rupee and the Philippine peso have **two**
  decimal places, which is why money is a whole number of paise or centavos.
- **WhatsApp's own conventions** for the `wa.me` link and the four characters it
  reads as formatting. Not a standard and not a published guarantee: they have
  changed before, so every figure that depends on them is overridable in the
  settings.

No statutory rate, fee or threshold is used anywhere in this tool: it computes no
tax and no duty.

A home food business in India needs an **FSSAI registration or licence**; one in
the Philippines deals with **DTI or SEC registration, a mayor's permit, BIR
registration, and FDA requirements for processed food**. This tool does not
check, hold, record or file any of that, and keeping a catalogue in it is not
evidence of anything. Confirm the current position with the authority or your own
adviser.

## Install & run

One binary, standard library only, no external dependencies at all.

```sh
git clone https://github.com/sreenivas-sadhu-prabhakara/whatsapp-catalog-order-book.git
cd whatsapp-catalog-order-book
go build -o catalog-order-book .
./catalog-order-book

# or, without building first
go run .
```

Then open `http://127.0.0.1:8080`.

```sh
./catalog-order-book -addr 127.0.0.1:9000 -data /path/to/order-book.json
```

| Flag | Default | What it is |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | Where to serve the interface. Loopback only by default. |
| `-data` | `./catalog-order-book-data.json` | The file the catalogue and the book live in. |

Load one of the two worked shops first, from **The shop** tab, to see the whole
tool working before typing anything of your own: a pickle and snack kitchen in
Pune pricing in rupees, and a home bakery in Quezon City pricing in pesos. Each
has something available, something made to order, something sold out and
something kept off the shared list, and a book with orders at several stages —
including one taken at a price the catalogue has since changed, and one wanted
sooner than its notice allows, so both rules can be seen working.

## Where your data is stored

One JSON file beside the binary, written atomically — a temporary file, flushed,
then renamed over the old one, so a crash leaves either the old file or the new
one and never half of either. It is readable in any text editor, because it is
your record and not a database. Money in it is text, never a float, so nothing
that reads the file can round a price.

The file holds your customers' names, phone numbers and addresses, so it is
written readable only by the user who runs the tool (mode `0600`).

**Nothing leaves the machine.** The tool binds to `127.0.0.1`, makes no outbound
network call of any kind, and has no telemetry, no analytics, and no fonts,
scripts or styles from anywhere else. A request arriving from another website is
refused outright.

## Scope and accuracy

It keeps a record and writes messages. It is not advice and it is not a filing.

- It **sends nothing and collects nothing**. Check every message before you send
  it — that is what the link is for, and why the text is always offered beside it.
- It cannot tell you whether a figure is *right*. A price entered wrong once is
  on every order taken at it.
- It does not check that a phone number is anybody's number, and it cannot: it
  makes no network call. It checks the shape, which is what stops a link failing
  silently.
- It keeps no stock count and computes no tax.
- The lengths it assumes — 2,000 characters of link, 700 of status, 65,536 of
  message — are platform conventions rather than published guarantees. They are
  stated on the explainer page and are yours to change in the settings.

MIT licensed.
