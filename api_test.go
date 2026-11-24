package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sreenivas-sadhu-prabhakara/whatsapp-catalog-order-book/internal/orderbook"
)

// testServer is the whole interface, wired up the way main wires it, over a
// store in a directory of its own.
func testServer(t *testing.T) (*httptest.Server, *store) {
	t.Helper()

	st := openTestStore(t)
	mux := http.NewServeMux()
	(&api{store: st}).routes(mux)
	srv := httptest.NewServer(guard(mux))
	t.Cleanup(srv.Close)
	return srv, st
}

// call makes a request and returns the status and the decoded body.
func call(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encoding the request: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("%s %s answered with something that is not JSON: %v", method, path, err)
	}
	return res.StatusCode, out
}

// raw makes a request and returns the status and the body as bytes.
func rawCall(t *testing.T, srv *httptest.Server, method, path string) (int, []byte, http.Header) {
	t.Helper()

	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(res.Body); err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return res.StatusCode, buffer.Bytes(), res.Header
}

func TestStateIsEverythingThePageNeeds(t *testing.T) {
	srv, _ := testServer(t)

	status, body := call(t, srv, "GET", "/api/state", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/state answered %d", status)
	}
	for _, key := range []string{
		"today", "shop", "catalogue", "products", "orders", "book", "share",
		"daySheet", "dayText", "reference", "limits", "catalogueProblems", "dataFile",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("the state has no %q in it", key)
		}
	}

	// The lists the page offers in a dropdown all come from the tool, so the
	// page keeps no second copy of any of them.
	reference, ok := body["reference"].(map[string]any)
	if !ok {
		t.Fatal("the reference lists are not an object")
	}
	for _, key := range []string{
		"currencies", "availabilities", "measures", "statuses", "slots", "fulfilments", "markets", "messageKinds",
	} {
		list, ok := reference[key].([]any)
		if !ok || len(list) == 0 {
			t.Errorf("the reference list %q is %v", key, reference[key])
		}
	}

	// The figures this build ships with are stated, so the page can show them
	// beside the answers they produced.
	limits, ok := body["limits"].(map[string]any)
	if !ok {
		t.Fatal("the limits are not an object")
	}
	for _, key := range []string{
		"safeLinkCharacters", "statusCharacters", "messageCharacters", "maxPhoneDigits", "minPhoneDigits", "maxOrders",
	} {
		if value, ok := limits[key].(float64); !ok || value <= 0 {
			t.Errorf("the limit %q is %v", key, limits[key])
		}
	}
}

func TestTheWholeOrderJourneyOverTheAPI(t *testing.T) {
	srv, _ := testServer(t)

	// A shop.
	status, _ := call(t, srv, "PUT", "/api/shop", map[string]any{
		"name": "Aaji's Kitchen", "area": "Pune", "currency": "INR", "dialCode": "91",
		"phone": "+91 98765 43210", "payTo": "UPI test@examplebank",
	})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/shop answered %d", status)
	}

	// A product.
	status, body := call(t, srv, "PUT", "/api/product", map[string]any{
		"code": "mango 500", "name": "Mango pickle", "unit": "jar", "price": "320",
		"packAmount": "500", "packMeasure": "g", "availability": "available",
	})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/product answered %d: %v", status, body["error"])
	}
	product := body["product"].(map[string]any)
	if product["code"] != "MANGO-500" {
		t.Fatalf("the code came back as %v", product["code"])
	}
	if body["packLabel"] != "500 g" {
		t.Fatalf("the pack label is %v", body["packLabel"])
	}
	per := body["perStandard"].(map[string]any)
	if per["amount"] != "640.00" || per["unit"] != "kg" {
		t.Fatalf("the per-kilogramme figure is %v", per)
	}

	// An order.
	status, body = call(t, srv, "POST", "/api/order", map[string]any{
		"customer":   map[string]any{"name": "Meera", "phone": "9876543211"},
		"fulfilment": "pickup",
		"wanted":     orderbook.Today().AddDays(1).String(),
		"slot":       "morning",
		"lines":      []map[string]any{{"code": "MANGO-500", "qty": "2"}},
	})
	if status != http.StatusOK {
		t.Fatalf("POST /api/order answered %d: %v", status, body["error"])
	}
	view := body["order"].(map[string]any)
	order := view["order"].(map[string]any)
	ref := order["ref"].(string)
	if ref != "ORD-0001" {
		t.Fatalf("the reference is %q", ref)
	}
	if view["totals"].(map[string]any)["total"] != "640.00" {
		t.Fatalf("the order came to %v", view["totals"])
	}
	if view["blocked"] != false {
		t.Fatalf("a sound order is blocked: %v", view["problems"])
	}

	// A message, with a link that round-trips.
	messages := body["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("no messages came back with the order")
	}
	confirmation := messages[0].(map[string]any)
	if confirmation["kind"] != "confirmation" || confirmation["available"] != true {
		t.Fatalf("the first message is %v", confirmation)
	}
	text := confirmation["text"].(string)
	link := confirmation["whatsapp"].(map[string]any)["url"].(string)
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("the link the API returned does not parse: %v", err)
	}
	if got := parsed.Query().Get("text"); got != text {
		t.Fatalf("the message did not survive the link:\n  want %q\n   got %q", text, got)
	}

	// Add a line.
	status, body = call(t, srv, "POST", "/api/order/line", map[string]any{
		"ref": ref, "code": "MANGO-500", "qty": "1", "note": "extra",
	})
	if status != http.StatusOK {
		t.Fatalf("POST /api/order/line answered %d: %v", status, body["error"])
	}
	if got := body["order"].(map[string]any)["totals"].(map[string]any)["total"]; got != "960.00" {
		t.Fatalf("after a third jar the order comes to %v", got)
	}

	// Move it on, one stage at a time.
	for _, stage := range []string{"confirmed", "packed"} {
		status, body = call(t, srv, "POST", "/api/order/status", map[string]any{"ref": ref, "status": stage})
		if status != http.StatusOK {
			t.Fatalf("moving to %s answered %d: %v", stage, status, body["error"])
		}
	}
	status, body = call(t, srv, "POST", "/api/order/status", map[string]any{"ref": ref, "status": "delivered"})
	if status == http.StatusOK {
		t.Fatal("packed to delivered skipped a stage and was allowed")
	}
	if !strings.Contains(body["error"].(string), "one stage at a time") {
		t.Fatalf("the refusal reads %q", body["error"])
	}

	// Payment.
	status, body = call(t, srv, "POST", "/api/order/payment", map[string]any{
		"ref": ref, "paid": "960", "note": "UPI",
	})
	if status != http.StatusOK {
		t.Fatalf("POST /api/order/payment answered %d: %v", status, body["error"])
	}
	if got := body["order"].(map[string]any)["totals"].(map[string]any)["balance"]; got != "0.00" {
		t.Fatalf("the balance is %v", got)
	}

	// Overpaying is refused by the rule, and reported rather than hidden.
	status, body = call(t, srv, "POST", "/api/order/payment", map[string]any{"ref": ref, "paid": "5000"})
	if status != http.StatusOK {
		t.Fatalf("recording an overpayment answered %d", status)
	}
	if body["order"].(map[string]any)["blocked"] != true {
		t.Fatal("an overpaid order is not blocked")
	}
}

func TestAPriceChangeDoesNotMoveAnOrderOverTheAPI(t *testing.T) {
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	_, before := call(t, srv, "GET", "/api/order/messages?ref=ORD-0002", nil)
	beforeTotal := before["order"].(map[string]any)["totals"].(map[string]any)["total"]

	status, body := call(t, srv, "PUT", "/api/product", map[string]any{
		"code": "CHAKLI-250", "name": "Chakli", "unit": "packet", "price": "200",
		"packAmount": "250", "packMeasure": "g", "availability": "made-to-order",
		"noticeDays": 2, "minQty": "2",
	})
	if status != http.StatusOK {
		t.Fatalf("changing the price answered %d: %v", status, body["error"])
	}

	_, after := call(t, srv, "GET", "/api/order/messages?ref=ORD-0002", nil)
	view := after["order"].(map[string]any)
	if got := view["totals"].(map[string]any)["total"]; got != beforeTotal {
		t.Fatalf("the order came to %v before the price change and %v after", beforeTotal, got)
	}

	changes := view["priceChanges"].([]any)
	if len(changes) == 0 {
		t.Fatal("the price change was not reported")
	}
	change := changes[0].(map[string]any)
	if change["takenAt"] != "140.00" || change["nowCosts"] != "200.00" {
		t.Fatalf("the change reads %v", change)
	}
	if view["blocked"] == true {
		t.Fatalf("a price change blocked the order: %v", view["problems"])
	}
}

func TestTheWorkedShopsLoadAndTheBookCanBeEmptied(t *testing.T) {
	srv, _ := testServer(t)

	for _, market := range []string{"IN", "PH"} {
		status, body := call(t, srv, "POST", "/api/sample", map[string]any{"market": market})
		if status != http.StatusOK {
			t.Fatalf("loading the %s shop answered %d: %v", market, status, body["error"])
		}
		if len(body["products"].([]any)) < 5 {
			t.Fatalf("the %s shop loaded %d products", market, len(body["products"].([]any)))
		}
		if len(body["orders"].([]any)) < 3 {
			t.Fatalf("the %s shop loaded %d orders", market, len(body["orders"].([]any)))
		}
		share := body["share"].(map[string]any)
		if share["text"] == "" {
			t.Fatalf("the %s shop has no shareable catalogue", market)
		}
		if share["fitsStatus"] != true {
			t.Fatalf("the %s shop's catalogue is %v characters and does not fit a status",
				market, share["characters"])
		}
	}

	status, body := call(t, srv, "POST", "/api/clear", nil)
	if status != http.StatusOK {
		t.Fatalf("emptying answered %d", status)
	}
	if len(body["orders"].([]any)) != 0 {
		t.Fatalf("%d orders left after emptying", len(body["orders"].([]any)))
	}
	if len(body["products"].([]any)) == 0 {
		t.Fatal("emptying the book threw the catalogue away")
	}
}

func TestTheDayEndpointAnswersForAnyDay(t *testing.T) {
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	tomorrow := orderbook.Today().AddDays(1).String()
	status, body := call(t, srv, "GET", "/api/day?on="+tomorrow, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/day answered %d", status)
	}
	sheet := body["sheet"].(map[string]any)
	if sheet["on"] != tomorrow {
		t.Fatalf("the sheet is for %v", sheet["on"])
	}
	if len(sheet["make"].([]any)) == 0 {
		t.Fatal("the worked shop has nothing to make tomorrow")
	}
	if body["text"] == "" {
		t.Fatal("the day has no text")
	}

	message := body["message"].(map[string]any)
	if message["text"] != body["text"] {
		t.Fatal("the day message is not the day text")
	}
	link := message["whatsapp"].(map[string]any)
	if link["url"] == "" {
		t.Fatal("the day sheet has no link")
	}
	parsed, err := url.Parse(link["url"].(string))
	if err != nil {
		t.Fatalf("the day link does not parse: %v", err)
	}
	if got := parsed.Query().Get("text"); got != body["text"].(string) {
		t.Fatal("the day sheet did not survive its own link")
	}

	// A day that is not a day is refused with a reason rather than answered
	// with something wrong.
	status, body = call(t, srv, "GET", "/api/day?on=2027-02-29", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("the 29th of February 2027 answered %d", status)
	}
	if !strings.Contains(body["error"].(string), "not a day that exists") {
		t.Fatalf("the refusal reads %q", body["error"])
	}
}

func TestEveryCSVComesOutReadableWithItsHeadings(t *testing.T) {
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	cases := []struct {
		path    string
		heading string
		least   int
	}{
		{"/api/orders.csv", "reference", 5},
		{"/api/catalogue.csv", "code", 6},
		{"/api/day.csv?on=" + orderbook.Today().AddDays(1).String(), "day", 2},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			status, body, header := rawCall(t, srv, "GET", c.path)
			if status != http.StatusOK {
				t.Fatalf("answered %d", status)
			}
			if !strings.HasPrefix(header.Get("Content-Type"), "text/csv") {
				t.Fatalf("the content type is %q", header.Get("Content-Type"))
			}
			if !strings.Contains(header.Get("Content-Disposition"), ".csv") {
				t.Fatalf("the file name is %q", header.Get("Content-Disposition"))
			}

			// The byte-order mark, so a spreadsheet reads the rupee sign and
			// the Devanagari names as what they are.
			if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
				t.Fatal("the file has no byte-order mark, so a spreadsheet will mangle the currency sign")
			}

			rows, err := csv.NewReader(bytes.NewReader(body[3:])).ReadAll()
			if err != nil {
				t.Fatalf("the file is not readable CSV: %v", err)
			}
			if len(rows) < c.least {
				t.Fatalf("%d rows, want at least %d", len(rows), c.least)
			}
			if rows[0][0] != c.heading {
				t.Fatalf("the first heading is %q, want %q", rows[0][0], c.heading)
			}
			for i, row := range rows {
				if len(row) != len(rows[0]) {
					t.Fatalf("row %d has %d columns and the heading has %d", i, len(row), len(rows[0]))
				}
			}
		})
	}
}

func TestTheOrdersCSVCarriesThePriceEachLineWasTakenAt(t *testing.T) {
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	_, body, _ := rawCall(t, srv, "GET", "/api/orders.csv")
	rows, err := csv.NewReader(bytes.NewReader(body[3:])).ReadAll()
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	at := -1
	for i, heading := range rows[0] {
		if heading == "unit price taken at" {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("there is no column for the price a line was taken at: %v", rows[0])
	}

	// ORD-0001's first line was taken at 300.00 and the catalogue says 320.00.
	found := false
	for _, row := range rows[1:] {
		if row[0] == "ORD-0001" && row[9] == "MANGO-500" {
			found = true
			if row[at] != "300.00" {
				t.Fatalf("the price taken at is %q, want 300.00 — the catalogue's 320.00 must not appear here", row[at])
			}
		}
	}
	if !found {
		t.Fatal("ORD-0001's mango line is not in the file")
	}
}

func TestWhatTheAPIRefuses(t *testing.T) {
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		status int
		says   string
	}{
		{"a misspelled field", "PUT", "/api/shop", map[string]any{"nmae": "typo"},
			http.StatusBadRequest, "unknown field"},
		{"a product with no code", "PUT", "/api/product", map[string]any{"name": "No code", "price": "100"},
			http.StatusBadRequest, "needs a code"},
		{"a product that is not JSON", "PUT", "/api/product", "not an object",
			http.StatusBadRequest, "could not read"},
		{"removing a product that is not there", "DELETE", "/api/product?code=NOPE", nil,
			http.StatusNotFound, "nothing in the catalogue"},
		{"removing a product with no code", "DELETE", "/api/product", nil,
			http.StatusBadRequest, "which product"},
		{"an order for something not sold", "POST", "/api/order",
			map[string]any{"customer": map[string]any{"name": "X"}, "lines": []map[string]any{{"code": "NOPE", "qty": "1"}}},
			http.StatusBadRequest, "nothing in the catalogue"},
		{"a line on an order that is not there", "POST", "/api/order/line",
			map[string]any{"ref": "NOPE", "code": "MANGO-500", "qty": "1"},
			http.StatusBadRequest, "no order"},
		{"a status on an order that is not there", "POST", "/api/order/status",
			map[string]any{"ref": "NOPE", "status": "confirmed"},
			http.StatusBadRequest, "no order"},
		{"a payment less than nothing", "POST", "/api/order/payment",
			map[string]any{"ref": "ORD-0002", "paid": "-100"},
			http.StatusBadRequest, "less than nothing"},
		{"messages for an order that is not there", "GET", "/api/order/messages?ref=NOPE", nil,
			http.StatusNotFound, "no order"},
		{"deleting an order with no reference", "DELETE", "/api/order", nil,
			http.StatusBadRequest, "which order"},
		{"deleting an order that is not there", "DELETE", "/api/order?ref=NOPE", nil,
			http.StatusNotFound, "no order"},
		{"a price that is not a price", "PUT", "/api/product",
			map[string]any{"code": "X", "name": "X", "price": "three hundred"},
			http.StatusBadRequest, "not an amount"},
		{"a quantity finer than a thousandth", "POST", "/api/order/line",
			map[string]any{"ref": "ORD-0002", "code": "MANGO-500", "qty": "1.00001"},
			http.StatusBadRequest, "thousandth"},
		{"a date that is not a date", "GET", "/api/day?on=the+day+after+tomorrow", nil,
			http.StatusBadRequest, "not a date"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := call(t, srv, c.method, c.path, c.body)
			if status != c.status {
				t.Fatalf("answered %d, want %d (body %v)", status, c.status, body)
			}
			message, ok := body["error"].(string)
			if !ok {
				t.Fatalf("the refusal carries no error: %v", body)
			}
			if !strings.Contains(message, c.says) {
				t.Fatalf("the refusal reads %q, which does not mention %q", message, c.says)
			}
		})
	}
}

func TestARequestFromAnotherSiteIsRefused(t *testing.T) {
	srv, _ := testServer(t)

	cases := []struct {
		origin string
		status int
	}{
		{"", http.StatusOK},
		{srv.URL, http.StatusOK},
		{"https://example.com", http.StatusForbidden},
		{"http://localhost:9999", http.StatusForbidden},
		{"null", http.StatusForbidden},
	}

	for _, c := range cases {
		req, err := http.NewRequest("GET", srv.URL+"/api/state", nil)
		if err != nil {
			t.Fatalf("building: %v", err)
		}
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("%q: %v", c.origin, err)
		}
		res.Body.Close()
		if res.StatusCode != c.status {
			t.Errorf("Origin %q answered %d, want %d", c.origin, res.StatusCode, c.status)
		}
	}
}

func TestEveryAnswerCarriesTheProtectionsALocalToolNeeds(t *testing.T) {
	srv, _ := testServer(t)

	_, _, header := rawCall(t, srv, "GET", "/api/state")
	for key, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := header.Get(key); got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
}

func TestTheShareEndpointRebuildsTheCatalogueEveryTime(t *testing.T) {
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	_, before := call(t, srv, "GET", "/api/share", nil)
	if !strings.Contains(before["text"].(string), "₹320.00") {
		t.Fatalf("the old price is not in the old text:\n%s", before["text"])
	}

	call(t, srv, "PUT", "/api/product", map[string]any{
		"code": "MANGO-500", "name": "Mango pickle", "unit": "jar", "price": "400",
		"packAmount": "500", "packMeasure": "g", "availability": "available",
	})

	_, after := call(t, srv, "GET", "/api/share", nil)
	text := after["text"].(string)
	if strings.Contains(text, "₹320.00 a jar") {
		t.Fatalf("the rebuilt catalogue still carries the old price:\n%s", text)
	}
	if !strings.Contains(text, "₹400.00 a jar") || !strings.Contains(text, "₹800.00 a kg") {
		t.Fatalf("the rebuilt catalogue does not carry the new price and its per-kilogramme figure:\n%s", text)
	}
	if !strings.Contains(text, "Prices from") {
		t.Fatalf("the rebuilt catalogue does not say when the prices are from:\n%s", text)
	}
}

func TestThePageItselfIsServedFromTheBinary(t *testing.T) {
	st := openTestStore(t)
	mux := http.NewServeMux()
	(&api{store: st}).routes(mux)

	assets, err := embeddedAssets()
	if err != nil {
		t.Fatalf("reading the built-in interface: %v", err)
	}
	mux.Handle("GET /", http.FileServerFS(assets))

	srv := httptest.NewServer(guard(mux))
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/", "/index.html", "/assets/app.js", "/assets/app.css",
		"/assets/style.css", "/assets/accent.css", "/assets/theme-init.js", "/assets/favicon.svg",
	} {
		status, body, _ := rawCall(t, srv, "GET", path)
		if status != http.StatusOK {
			t.Errorf("GET %s answered %d", path, status)
			continue
		}
		if len(body) == 0 {
			t.Errorf("GET %s answered with nothing", path)
		}
	}
}

func TestEveryLineAmountIsWorkedOutByTheToolAndNotByThePage(t *testing.T) {
	// The page draws a saved order's line amounts from the tool, because the
	// tool is the only thing here that works in whole paise. The figures have
	// to be there, one for each line, in the same order, and they have to add
	// up to the subtotal the tool also sends.
	srv, st := testServer(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	for _, ref := range []string{"ORD-0001", "ORD-0002", "ORD-0003", "ORD-0004"} {
		t.Run(ref, func(t *testing.T) {
			status, body := call(t, srv, "GET", "/api/order/messages?ref="+ref, nil)
			if status != http.StatusOK {
				t.Fatalf("answered %d", status)
			}
			view := body["order"].(map[string]any)
			lines := view["order"].(map[string]any)["lines"].([]any)
			amounts, ok := view["lineAmounts"].([]any)
			if !ok {
				t.Fatalf("there are no line amounts: %v", view["lineAmounts"])
			}
			if len(amounts) != len(lines) {
				t.Fatalf("%d line amounts for %d lines", len(amounts), len(lines))
			}

			// They are decimal strings, and they add up to the subtotal.
			var total orderbook.Money
			for i, amount := range amounts {
				as, ok := amount.(string)
				if !ok {
					t.Fatalf("line %d's amount is %v, which is not text", i+1, amount)
				}
				parsed, err := orderbook.ParseMoney(as)
				if err != nil {
					t.Fatalf("line %d's amount %q is not an amount: %v", i+1, as, err)
				}
				total += parsed
			}
			if got := view["totals"].(map[string]any)["subtotal"].(string); got != total.String() {
				t.Fatalf("the lines add up to %s and the subtotal says %s", total, got)
			}
		})
	}
}
