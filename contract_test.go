package main

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/sreenivas-sadhu-prabhakara/whatsapp-catalog-order-book/internal/orderbook"
)

// The page and the tool are two files in two languages, and nothing but a test
// holds them together. A name changed on one side and not the other leaves a
// panel that silently never fills in, or a figure the shopkeeper thinks they
// entered and did not.

// elementPattern finds the elements the page's script looks up by name: $("id").
var elementPattern = regexp.MustCompile(`\$\("([a-zA-Z0-9-]+)"\)`)

// assetPattern finds the files a page loads.
var assetPattern = regexp.MustCompile(`(?:href|src)="(assets/[^"]+)"`)

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(raw)
}

func TestEveryElementTheInterfaceLooksUpIsOnItsPage(t *testing.T) {
	script := readFile(t, "web/assets/app.js")
	page := readFile(t, "web/index.html")

	matches := elementPattern.FindAllStringSubmatch(script, -1)
	if len(matches) < 30 {
		t.Fatalf("only %d element lookups found; the pattern has probably stopped matching", len(matches))
	}

	var missing []string
	seen := map[string]bool{}
	for _, match := range matches {
		id := match[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		// The script makes one field for itself, which is not on the page.
		if id == "pay-amount" {
			continue
		}
		if !strings.Contains(page, `id="`+id+`"`) {
			missing = append(missing, id)
		}
	}

	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("the interface looks up %d element(s) that are not on the page:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestEveryPanelTheInterfaceSwitchesBetweenExists(t *testing.T) {
	script := readFile(t, "web/assets/app.js")
	page := readFile(t, "web/index.html")

	pattern := regexp.MustCompile(`var PANELS = \[([^\]]*)\]`)
	match := pattern.FindStringSubmatch(script)
	if match == nil {
		t.Fatal("the interface's list of panels could not be found")
	}

	names := regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(match[1], -1)
	if len(names) < 5 {
		t.Fatalf("the interface switches between only %d panels", len(names))
	}
	for _, name := range names {
		if !strings.Contains(page, `id="panel-`+name[1]+`"`) {
			t.Errorf("the interface switches to the panel %q, which is not on the page", name[1])
		}
		if !strings.Contains(page, `data-tab="`+name[1]+`"`) {
			t.Errorf("the panel %q has no button to reach it", name[1])
		}
	}

	// And no button leads to a panel the script does not know about.
	buttons := regexp.MustCompile(`data-tab="([a-z]+)"`).FindAllStringSubmatch(page, -1)
	known := map[string]bool{}
	for _, name := range names {
		known[name[1]] = true
	}
	for _, button := range buttons {
		if !known[button[1]] {
			t.Errorf("the page has a button for %q, which the interface does not switch to", button[1])
		}
	}
}

// endpointsTheInterfaceUses is every /api path the page fetches or links to.
func endpointsTheInterfaceUses(t *testing.T) map[string]bool {
	t.Helper()

	script := readFile(t, "web/assets/app.js")
	page := readFile(t, "web/index.html")

	matches := append(
		regexp.MustCompile(`"(/api/[a-z./]+)`).FindAllStringSubmatch(script, -1),
		regexp.MustCompile(`href="(/api/[a-z./]+)`).FindAllStringSubmatch(page, -1)...,
	)
	if len(matches) < 8 {
		t.Fatalf("only %d endpoint references found; the pattern has probably stopped matching", len(matches))
	}

	out := map[string]bool{}
	for _, match := range matches {
		out[match[1]] = true
	}
	return out
}

// endpointsTheToolServes is every path registered on the mux.
func endpointsTheToolServes(t *testing.T) map[string]bool {
	t.Helper()

	source := readFile(t, "api.go")
	matches := regexp.MustCompile(`mux\.HandleFunc\("[A-Z]+ (/api/[a-z./]+)"`).FindAllStringSubmatch(source, -1)
	if len(matches) < 10 {
		t.Fatalf("only %d endpoints found; the pattern has probably stopped matching", len(matches))
	}

	out := map[string]bool{}
	for _, match := range matches {
		out[match[1]] = true
	}
	return out
}

func TestEveryEndpointTheInterfaceCallsIsRegistered(t *testing.T) {
	served := endpointsTheToolServes(t)
	for path := range endpointsTheInterfaceUses(t) {
		if !served[path] {
			t.Errorf("the interface calls %s, which the tool does not serve", path)
		}
	}
}

func TestEveryEndpointTheToolServesIsReachableFromTheInterface(t *testing.T) {
	// The other direction, which is the one that rots quietly: an endpoint with
	// no way to it from the page is code nobody runs, and it will be wrong by
	// the time somebody wires it up.
	used := endpointsTheInterfaceUses(t)

	var unreachable []string
	for path := range endpointsTheToolServes(t) {
		if !used[path] {
			unreachable = append(unreachable, path)
		}
	}
	sort.Strings(unreachable)
	if len(unreachable) > 0 {
		t.Fatalf("the tool serves %d endpoint(s) with no way to them from the page:\n  %s",
			len(unreachable), strings.Join(unreachable, "\n  "))
	}
}

func TestEveryFieldTheInterfaceSendsIsAFieldTheToolTakes(t *testing.T) {
	// The tool refuses an unknown field rather than ignoring it, so anything
	// the page sends has to be something the tool will take. These are the
	// bodies the page builds, sent through the same decoder the API uses.
	cases := []struct {
		name string
		body string
		into func() any
	}{
		{"the shop", `{
			"name":"X","area":"Y","currency":"INR","dialCode":"91","phone":"+91 98765 43210",
			"email":"a@b.com","payTo":"UPI x@y","howToOrder":"reply","closing":"thanks",
			"limits":{"safeLinkCharacters":2000,"statusCharacters":700,"messageCharacters":65536}
		}`, func() any { return new(orderbook.Shop) }},

		{"a product", `{
			"code":"X","name":"Y","unit":"jar","price":"320","packAmount":"500","packMeasure":"g",
			"availability":"available","noticeDays":2,"minQty":"2","note":"n","hidden":false
		}`, func() any { return new(orderbook.Product) }},

		{"a new order", `{
			"customer":{"name":"X","phone":"9876543211","email":"a@b.com","address":"somewhere"},
			"fulfilment":"delivery","wanted":"2026-11-26","slot":"morning",
			"deliveryFee":"40","discount":"25.50","discountReason":"regular","note":"n",
			"lines":[{"code":"X","qty":"2","note":"n","price":null}]
		}`, func() any { return new(newOrder) }},

		{"an order sent back", `{
			"ref":"ORD-0001","placedOn":"2026-11-24",
			"customer":{"name":"X","phone":"9","email":"","address":""},
			"fulfilment":"pickup","wanted":"2026-11-26","slot":"morning",
			"lines":[{"code":"X","name":"Y","pack":"500 g","unit":"jar","qty":"2","unitPrice":"320","note":""}],
			"deliveryFee":"0","discount":"0","discountReason":"","paid":"0","paymentNote":"",
			"status":"taken","note":""
		}`, func() any { return new(orderbook.Order) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			into := c.into()
			if err := decodeStrictly(c.body, into); err != nil {
				t.Fatalf("the tool will not take what the page sends: %v", err)
			}
		})
	}
}

func TestTheShopSurvivesTheRoundTripTheInterfaceMakes(t *testing.T) {
	// The page reads the shop the tool sent, changes a field or two and sends
	// the whole thing back. The tool refuses unknown fields, so anything it
	// sends must be something it will take back.
	shop, catalogue, orders := orderbook.Sample(orderbook.IN, orderbook.Today())

	for _, c := range []struct {
		name  string
		value any
		into  func() any
	}{
		{"the shop", shop, func() any { return new(orderbook.Shop) }},
		{"a product", catalogue.Products[0], func() any { return new(orderbook.Product) }},
		{"an order", orders[0], func() any { return new(orderbook.Order) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			sent := encode(t, c.value)
			into := c.into()
			if err := decodeStrictly(sent, into); err != nil {
				t.Fatalf("the tool will not take back what it sent: %v\n%s", err, sent)
			}
		})
	}
}

func TestFiguresGoBackAsTheTextThatWasTyped(t *testing.T) {
	// The tool is the only thing here that knows what a price is, so a figure
	// travels as text and is read by the tool. These are the forms a
	// shopkeeper actually types.
	cases := []struct {
		typed string
		want  orderbook.Money
	}{
		{"320", 32000},
		{"320.50", 32050},
		{"320.5", 32050},
		{"₹320.50", 32050},
		{"1,23,456.78", 12345678},
		{"", 0},
	}

	for _, c := range cases {
		t.Run(c.typed, func(t *testing.T) {
			var into orderbook.Product
			if err := decodeStrictly(`{"code":"X","name":"Y","price":"`+c.typed+`"}`, &into); err != nil {
				t.Fatalf("the tool would not read %q: %v", c.typed, err)
			}
			if into.Price != c.want {
				t.Fatalf("%q read as %s, want %s", c.typed, into.Price, c.want)
			}
		})
	}
}

func TestEveryAssetThePagesLoadIsThere(t *testing.T) {
	for _, page := range []string{"web/index.html", "docs/index.html"} {
		t.Run(page, func(t *testing.T) {
			body := readFile(t, page)
			matches := assetPattern.FindAllStringSubmatch(body, -1)
			if len(matches) < 4 {
				t.Fatalf("%s loads only %d files of its own", page, len(matches))
			}
			dir := strings.SplitN(page, "/", 2)[0]
			for _, match := range matches {
				if _, err := os.Stat(dir + "/" + match[1]); err != nil {
					t.Fatalf("%s loads %s, which is not there: %v", page, match[1], err)
				}
			}
		})
	}
}

func TestTheServedPagesKeepTheirProtections(t *testing.T) {
	// Byte-exact, because a content-security policy that has quietly lost a
	// directive protects nothing and looks as though it does.
	const base = "default-src 'self'; base-uri 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; form-action 'self'"

	cases := []struct {
		page string
		// The interface page fetches from the tool on the same machine, so it
		// needs that one addition and nothing else.
		connect bool
	}{
		{"web/index.html", true},
		{"docs/index.html", false},
	}

	for _, c := range cases {
		t.Run(c.page, func(t *testing.T) {
			page := readFile(t, c.page)

			want := base
			if c.connect {
				want += "; connect-src 'self'"
			}
			if !strings.Contains(page, `content="`+want+`"`) {
				t.Fatalf("%s does not carry the policy byte for byte", c.page)
			}

			for _, forbidden := range []string{"<style", `style="`, "onclick=", "oninput=", "onchange=", "onsubmit=", "onload="} {
				if strings.Contains(strings.ToLower(page), forbidden) {
					t.Fatalf("%s contains %q, which the policy forbids", c.page, forbidden)
				}
			}
			// Every script is an external file, and the theme is applied before
			// the first paint.
			for _, script := range regexp.MustCompile(`<script[^>]*>`).FindAllString(page, -1) {
				if !strings.Contains(script, "src=") {
					t.Fatalf("%s has an inline script: %s", c.page, script)
				}
			}
			if !strings.Contains(page, `<script src="assets/theme-init.js"></script>`) {
				t.Fatalf("%s does not load the theme before it paints", c.page)
			}
			if !strings.HasSuffix(titleOf(page), " · Open MSME") {
				t.Fatalf("%s is titled %q", c.page, titleOf(page))
			}
			if h1 := h1Of(page); h1 == "" {
				t.Fatalf("%s has no h1", c.page)
			} else if !strings.HasPrefix(strings.ToLower(h1), "product catalogue and order book") {
				t.Fatalf("%s leads with %q; the h1 has to lead with what the thing is", c.page, h1)
			}
		})
	}
}

func TestNothingInTheToolSaysItSendsAnything(t *testing.T) {
	// The honesty this tool rests on. It builds text and links; a person sends
	// them. Nothing anywhere may suggest otherwise.
	pages := []string{"web/index.html", "docs/index.html", "README.md"}

	// Phrases that would claim the tool does the sending itself.
	claims := []string{
		"sends the message",
		"sends your message",
		"sends it for you",
		"we send",
		"automatically sends",
		"sends automatically",
		"integrates with whatsapp",
		"whatsapp api",
		"whatsapp business api",
		"connects to whatsapp",
		"sends the whatsapp",
		"takes payment",
		"collects payment",
		"processes payment",
		"payment gateway",
	}

	for _, page := range pages {
		body := strings.ToLower(readFile(t, page))
		for _, claim := range claims {
			if strings.Contains(body, claim) {
				t.Errorf("%s says %q, which this tool does not do", page, claim)
			}
		}
	}

	// And each one says plainly that the shopkeeper does the sending.
	for _, page := range pages {
		body := strings.ToLower(readFile(t, page))
		if !strings.Contains(body, "sends nothing") && !strings.Contains(body, "send it yourself") &&
			!strings.Contains(body, "you send") {
			t.Errorf("%s does not say anywhere that the shopkeeper does the sending", page)
		}
	}
}

func TestTheDocumentationStatesEveryFigureTheBuildShipsWith(t *testing.T) {
	// A figure used in the code and not stated in the documentation is a figure
	// the shop cannot check.
	docs := readFile(t, "docs/index.html")
	readme := readFile(t, "README.md")

	for _, figure := range []string{"2,000", "700", "65,536", "15"} {
		if !strings.Contains(docs, figure) {
			t.Errorf("the documentation does not state the figure %s", figure)
		}
	}
	for _, name := range []string{
		"SafeLinkCharacters", "StatusCharacters", "MessageCharacters", "MaxPhoneDigits",
	} {
		if !strings.Contains(docs, name) {
			t.Errorf("the documentation does not name the constant %s", name)
		}
	}
	for _, phrase := range []string{"RFC 3986", "E.164", "MIT licensed."} {
		if !strings.Contains(readme, phrase) {
			t.Errorf("the README does not mention %q", phrase)
		}
	}
	if !strings.Contains(docs, "RFC 3986") || !strings.Contains(docs, "E.164") {
		t.Error("the documentation does not state the standards the tool implements")
	}
}

func TestEveryProblemCodeTheToolReportsIsSomethingTheInterfaceCanShow(t *testing.T) {
	// The page shows a problem's own words and its code, so a code that is
	// reported but never produced would be dead copy, and a severity the page
	// does not style would show as nothing.
	source := readFile(t, "internal/orderbook/problem.go")
	script := readFile(t, "web/assets/app.js")
	style := readFile(t, "web/assets/app.css")

	codes := regexp.MustCompile(`Problem[A-Za-z]+\s+= "([a-z-]+)"`).FindAllStringSubmatch(source, -1)
	if len(codes) < 20 {
		t.Fatalf("only %d problem codes found; the pattern has probably stopped matching", len(codes))
	}

	// The page prints the code and the message, so it needs no per-code copy —
	// but it does need to style both severities.
	for _, severity := range []string{"stop", "check"} {
		if !strings.Contains(style, `[data-severity="`+severity+`"]`) {
			t.Errorf("the stylesheet does not style a %q problem", severity)
		}
		if !strings.Contains(script, severity) {
			t.Errorf("the interface does not mention the %q severity", severity)
		}
	}
}

// titleOf pulls a page's title out.
func titleOf(page string) string {
	match := regexp.MustCompile(`<title>([^<]*)</title>`).FindStringSubmatch(page)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// h1Of pulls a page's heading out, with any tags inside it removed.
func h1Of(page string) string {
	match := regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`).FindStringSubmatch(page)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(match[1], ""))
}

// decodeStrictly reads a body the way the API reads one: refusing any field the
// tool does not know.
func decodeStrictly(body string, into any) error {
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(into)
}

// encode is what the tool sends to the page.
func encode(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	return string(raw)
}
