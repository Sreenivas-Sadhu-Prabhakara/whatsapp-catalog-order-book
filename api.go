package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sreenivas-sadhu-prabhakara/whatsapp-catalog-order-book/internal/orderbook"
)

// api serves the JSON interface the page talks to. Every call is same-origin,
// to the machine the tool is running on, and nothing leaves it.
type api struct {
	store *store
}

// routes registers every endpoint.
func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/state", a.getState)

	mux.HandleFunc("PUT /api/shop", a.putShop)

	mux.HandleFunc("PUT /api/product", a.putProduct)
	mux.HandleFunc("DELETE /api/product", a.deleteProduct)

	mux.HandleFunc("POST /api/order", a.postOrder)
	mux.HandleFunc("PUT /api/order", a.putOrder)
	mux.HandleFunc("DELETE /api/order", a.deleteOrder)
	mux.HandleFunc("POST /api/order/line", a.postLine)
	mux.HandleFunc("POST /api/order/status", a.postStatus)
	mux.HandleFunc("POST /api/order/payment", a.postPayment)
	mux.HandleFunc("GET /api/order/messages", a.getMessages)

	mux.HandleFunc("GET /api/share", a.getShare)
	mux.HandleFunc("GET /api/day", a.getDay)

	mux.HandleFunc("POST /api/sample", a.postSample)
	mux.HandleFunc("POST /api/clear", a.postClear)

	mux.HandleFunc("GET /api/orders.csv", a.getOrdersCSV)
	mux.HandleFunc("GET /api/catalogue.csv", a.getCatalogueCSV)
	mux.HandleFunc("GET /api/day.csv", a.getDayCSV)
}

// orderView is one order with everything the page needs to draw it, worked out
// by the tool so that the page keeps no rules of its own.
type orderView struct {
	Order    orderbook.Order         `json:"order"`
	Totals   orderbook.Totals        `json:"totals"`
	Problems []orderbook.Problem     `json:"problems"`
	Blocked  bool                    `json:"blocked"`
	Changes  []orderbook.PriceChange `json:"priceChanges"`
	Earliest orderbook.Date          `json:"earliest"`
	// Next is the stages this order may move to from where it is.
	Next []orderbook.Status `json:"next"`
	// LineAmounts is what each line comes to, in the same order as the lines.
	//
	// It is sent beside the order rather than on the lines themselves, because
	// the page sends the order back to be saved and a figure the tool worked
	// out must not come back as a figure the tool stores. The page therefore
	// never multiplies a price by a quantity for a saved order: it reads the
	// figure from here.
	LineAmounts []orderbook.Money `json:"lineAmounts"`
}

// reference is everything the page offers in a dropdown, sent by the tool so
// that the page does not keep a second copy of any list.
type reference struct {
	Currencies     []orderbook.Currency     `json:"currencies"`
	Availabilities []orderbook.Availability `json:"availabilities"`
	Measures       []orderbook.Measure      `json:"measures"`
	Statuses       []statusOption           `json:"statuses"`
	Slots          []slotOption             `json:"slots"`
	Fulfilments    []orderbook.Fulfilment   `json:"fulfilments"`
	Markets        []marketOption           `json:"markets"`
	Kinds          []orderbook.MessageKind  `json:"messageKinds"`
}

type statusOption struct {
	Value orderbook.Status `json:"value"`
	Name  string           `json:"name"`
	Open  bool             `json:"open"`
}

type slotOption struct {
	Value orderbook.Slot `json:"value"`
	Name  string         `json:"name"`
}

type marketOption struct {
	Value orderbook.Market `json:"value"`
	Name  string           `json:"name"`
}

// limitsNote is the figures this build ships with, sent to the page so that it
// states them beside the answers they produced rather than carrying its own
// copy.
type limitsNote struct {
	SafeLink  int `json:"safeLinkCharacters"`
	Status    int `json:"statusCharacters"`
	Message   int `json:"messageCharacters"`
	MaxPhone  int `json:"maxPhoneDigits"`
	MinPhone  int `json:"minPhoneDigits"`
	MaxOrders int `json:"maxOrders"`
}

// statePayload is everything the page needs to draw itself.
type statePayload struct {
	Today     orderbook.Date      `json:"today"`
	Shop      orderbook.Shop      `json:"shop"`
	Catalogue orderbook.Catalogue `json:"catalogue"`
	Products  []orderbook.Product `json:"products"`
	Orders    []orderView         `json:"orders"`
	Book      orderbook.Book      `json:"book"`
	Share     orderbook.Share     `json:"share"`
	Today24   orderbook.DaySheet  `json:"daySheet"`
	DayText   string              `json:"dayText"`
	Reference reference           `json:"reference"`
	Limits    limitsNote          `json:"limits"`
	Problems  []orderbook.Problem `json:"catalogueProblems"`
	DataFile  string              `json:"dataFile"`
}

func (a *api) getState(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()
	today := orderbook.Today()

	on := today
	if typed := r.URL.Query().Get("on"); typed != "" {
		parsed, err := orderbook.ParseDate(typed)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		on = parsed
	}

	sheet := orderbook.Day(data.Catalogue, data.Orders, on)

	ok(w, statePayload{
		Today:     today,
		Shop:      data.Shop,
		Catalogue: data.Catalogue,
		Products:  data.Catalogue.Products,
		Orders:    views(data, today),
		Book:      orderbook.Summarise(data.Catalogue, data.Shop, data.Orders, today),
		Share:     orderbook.ShareCatalogue(data.Shop, data.Catalogue),
		Today24:   sheet,
		DayText:   orderbook.DayText(data.Shop, sheet),
		Reference: referenceLists(),
		Limits: limitsNote{
			SafeLink:  data.Shop.Limits.SafeLinkCharacters,
			Status:    data.Shop.Limits.StatusCharacters,
			Message:   data.Shop.Limits.MessageCharacters,
			MaxPhone:  orderbook.MaxPhoneDigits,
			MinPhone:  orderbook.MinPhoneDigits,
			MaxOrders: maxOrders,
		},
		Problems: data.Catalogue.Problems(),
		DataFile: a.store.path,
	})
}

// views turns every order into what the page draws, newest first — because the
// order a shop wants to see is the one it just took.
func views(data fileData, today orderbook.Date) []orderView {
	out := make([]orderView, 0, len(data.Orders))
	for i := len(data.Orders) - 1; i >= 0; i-- {
		out = append(out, view(data, data.Orders[i], today))
	}
	return out
}

func view(data fileData, o orderbook.Order, _ orderbook.Date) orderView {
	problems := o.Problems(data.Catalogue, data.Shop)

	var next []orderbook.Status
	for _, status := range orderbook.Statuses() {
		if status == o.Status.Normalise() {
			continue
		}
		if o.Status.CanMoveTo(status) == nil {
			next = append(next, status)
		}
	}
	if next == nil {
		next = []orderbook.Status{}
	}

	changes := o.PriceChanges(data.Catalogue)
	if changes == nil {
		changes = []orderbook.PriceChange{}
	}
	if problems == nil {
		problems = []orderbook.Problem{}
	}

	amounts := make([]orderbook.Money, 0, len(o.Lines))
	for _, l := range o.Lines {
		amounts = append(amounts, l.Amount())
	}

	return orderView{
		Order:       o,
		Totals:      o.Add(),
		Problems:    problems,
		Blocked:     o.Blocked(data.Catalogue, data.Shop),
		Changes:     changes,
		Earliest:    o.EarliestPossible(data.Catalogue),
		Next:        next,
		LineAmounts: amounts,
	}
}

func referenceLists() reference {
	statuses := make([]statusOption, 0, len(orderbook.Statuses()))
	for _, s := range orderbook.Statuses() {
		statuses = append(statuses, statusOption{Value: s, Name: s.Name(), Open: s.Open()})
	}
	slots := make([]slotOption, 0, len(orderbook.Slots()))
	for _, s := range orderbook.Slots() {
		slots = append(slots, slotOption{Value: s, Name: s.Name()})
	}
	markets := make([]marketOption, 0, len(orderbook.Markets()))
	for _, m := range orderbook.Markets() {
		markets = append(markets, marketOption{Value: m, Name: orderbook.MarketName(m)})
	}

	return reference{
		Currencies:     orderbook.Currencies(),
		Availabilities: orderbook.Availabilities(),
		Measures:       orderbook.Measures(),
		Statuses:       statuses,
		Slots:          slots,
		Fulfilments:    orderbook.Fulfilments(),
		Markets:        markets,
		Kinds: []orderbook.MessageKind{
			orderbook.Confirmation, orderbook.Ready, orderbook.Reminder, orderbook.Receipt,
		},
	}
}

func (a *api) putShop(w http.ResponseWriter, r *http.Request) {
	var next orderbook.Shop
	if err := read(r, &next); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.putShop(next)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	ok(w, map[string]any{"shop": saved})
}

func (a *api) putProduct(w http.ResponseWriter, r *http.Request) {
	var next orderbook.Product
	if err := read(r, &next); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.putProduct(next)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	data := a.store.snapshot()
	per, perUnit, hasPer := saved.PerStandard()
	ok(w, map[string]any{
		"product":     saved,
		"packLabel":   saved.PackLabel(),
		"perStandard": map[string]any{"amount": per, "unit": perUnit, "shown": hasPer},
		"problems":    saved.Problems(),
		"catalogue":   data.Catalogue,
	})
}

func (a *api) deleteProduct(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if strings.TrimSpace(code) == "" {
		fail(w, http.StatusBadRequest, errors.New("which product? give its code"))
		return
	}
	if err := a.store.deleteProduct(code); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	ok(w, map[string]any{"removed": orderbook.NormaliseCode(code), "catalogue": a.store.snapshot().Catalogue})
}

func (a *api) postOrder(w http.ResponseWriter, r *http.Request) {
	var in newOrder
	if err := read(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.addOrder(in)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.answerWithOrder(w, saved.Ref)
}

func (a *api) putOrder(w http.ResponseWriter, r *http.Request) {
	var next orderbook.Order
	if err := read(r, &next); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.putOrder(next)
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	a.answerWithOrder(w, saved.Ref)
}

func (a *api) deleteOrder(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if strings.TrimSpace(ref) == "" {
		fail(w, http.StatusBadRequest, errors.New("which order? give its reference"))
		return
	}
	if err := a.store.deleteOrder(ref); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	ok(w, map[string]any{"removed": strings.ToUpper(strings.TrimSpace(ref))})
}

func (a *api) postLine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ref  string        `json:"ref"`
		Code string        `json:"code"`
		Qty  orderbook.Qty `json:"qty"`
		Note string        `json:"note"`
	}
	if err := read(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.addLine(in.Ref, in.Code, in.Qty, in.Note)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.answerWithOrder(w, saved.Ref)
}

func (a *api) postStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ref    string           `json:"ref"`
		Status orderbook.Status `json:"status"`
	}
	if err := read(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.setStatus(in.Ref, in.Status)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.answerWithOrder(w, saved.Ref)
}

func (a *api) postPayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ref  string          `json:"ref"`
		Paid orderbook.Money `json:"paid"`
		Note string          `json:"note"`
	}
	if err := read(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	saved, err := a.store.setPayment(in.Ref, in.Paid, in.Note)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a.answerWithOrder(w, saved.Ref)
}

// answerWithOrder sends one order back, worked out, plus the book as it now
// stands, so the page need not ask twice.
func (a *api) answerWithOrder(w http.ResponseWriter, ref string) {
	data := a.store.snapshot()
	today := orderbook.Today()

	at, found := orderbook.FindOrder(data.Orders, ref)
	if !found {
		fail(w, http.StatusNotFound, fmt.Errorf("there is no order called %q", ref))
		return
	}

	ok(w, map[string]any{
		"order":    view(data, data.Orders[at], today),
		"messages": orderbook.MessagesFor(data.Catalogue, data.Shop, data.Orders[at]),
		"book":     orderbook.Summarise(data.Catalogue, data.Shop, data.Orders, today),
	})
}

func (a *api) getMessages(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	data := a.store.snapshot()

	at, found := orderbook.FindOrder(data.Orders, ref)
	if !found {
		fail(w, http.StatusNotFound, fmt.Errorf("there is no order called %q", ref))
		return
	}

	ok(w, map[string]any{
		"ref":      data.Orders[at].Ref,
		"messages": orderbook.MessagesFor(data.Catalogue, data.Shop, data.Orders[at]),
		"order":    view(data, data.Orders[at], orderbook.Today()),
	})
}

func (a *api) getShare(w http.ResponseWriter, _ *http.Request) {
	data := a.store.snapshot()
	ok(w, orderbook.ShareCatalogue(data.Shop, data.Catalogue))
}

func (a *api) getDay(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()

	on := orderbook.Today()
	if typed := r.URL.Query().Get("on"); typed != "" {
		parsed, err := orderbook.ParseDate(typed)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		on = parsed
	}

	sheet := orderbook.Day(data.Catalogue, data.Orders, on)
	ok(w, map[string]any{
		"sheet":   sheet,
		"text":    orderbook.DayText(data.Shop, sheet),
		"message": orderbook.DayMessage(data.Shop, sheet),
	})
}

func (a *api) postSample(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Market orderbook.Market `json:"market"`
	}
	if err := read(r, &in); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	market := orderbook.IN
	if strings.EqualFold(string(in.Market), string(orderbook.PH)) {
		market = orderbook.PH
	}
	if err := a.store.loadSample(market); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	a.getState(w, r)
}

func (a *api) postClear(w http.ResponseWriter, r *http.Request) {
	if err := a.store.clear(); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	a.getState(w, r)
}

func (a *api) getOrdersCSV(w http.ResponseWriter, _ *http.Request) {
	data := a.store.snapshot()

	rows := [][]string{{
		"reference", "taken on", "wanted on", "part of day", "customer", "phone", "delivery or pickup",
		"address", "status", "product code", "product", "pack", "quantity", "unit price taken at",
		"line amount", "order subtotal", "delivery charge", "discount", "order total", "received", "balance",
	}}

	for _, o := range data.Orders {
		totals := o.Add()
		if len(o.Lines) == 0 {
			rows = append(rows, append(orderColumns(o), "", "", "", "", "",
				totals.Subtotal.String(), totals.DeliveryFee.String(), totals.Discount.String(),
				totals.Total.String(), totals.Paid.String(), totals.Balance.String()))
			continue
		}
		for _, l := range o.Lines {
			rows = append(rows, append(orderColumns(o),
				l.Code, l.Name, l.Pack, l.Qty.String(), l.UnitPrice.String(), l.Amount().String(),
				totals.Subtotal.String(), totals.DeliveryFee.String(), totals.Discount.String(),
				totals.Total.String(), totals.Paid.String(), totals.Balance.String()))
		}
	}
	sendCSV(w, "orders.csv", rows)
}

func orderColumns(o orderbook.Order) []string {
	return []string{
		o.Ref, o.PlacedOn.String(), o.Wanted.String(), string(o.Slot), o.Customer.Name, o.Customer.Phone,
		o.Fulfilment.Name(), strings.ReplaceAll(o.Customer.Address, "\n", ", "), o.Status.Name(),
	}
}

func (a *api) getCatalogueCSV(w http.ResponseWriter, _ *http.Request) {
	data := a.store.snapshot()

	rows := [][]string{{
		"code", "name", "unit", "price", "pack", "pack amount", "pack measure",
		"price per standard quantity", "standard quantity", "availability", "notice days",
		"least quantity", "hidden", "note",
	}}

	for _, p := range data.Catalogue.Products {
		per, perUnit, hasPer := p.PerStandard()
		perText, perName := "", ""
		if hasPer {
			perText, perName = per.String(), perUnit
		}
		rows = append(rows, []string{
			p.Code, p.Name, p.Unit, p.Price.String(), p.PackLabel(), p.PackAmount.String(), string(p.PackMeasure),
			perText, perName, p.Availability.Name(), fmt.Sprint(p.NoticeDays), p.MinQty.String(),
			yesNo(p.Hidden), p.Note,
		})
	}
	sendCSV(w, "catalogue.csv", rows)
}

func (a *api) getDayCSV(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()

	on := orderbook.Today()
	if typed := r.URL.Query().Get("on"); typed != "" {
		parsed, err := orderbook.ParseDate(typed)
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		on = parsed
	}
	sheet := orderbook.Day(data.Catalogue, data.Orders, on)

	rows := [][]string{{"day", "what", "code", "name", "pack", "quantity", "orders", "sold out"}}
	for _, m := range sheet.Make {
		rows = append(rows, []string{
			sheet.On.String(), "to make", m.Code, m.Name, m.Pack, m.Qty.String(), fmt.Sprint(m.Orders), yesNo(m.SoldOut),
		})
	}

	rows = append(rows, []string{"", "", "", "", "", "", "", ""})
	rows = append(rows, []string{"day", "what", "reference", "customer", "part of day", "total", "balance", "status"})
	for _, row := range append(sheet.Orders, sheet.Cancelled...) {
		rows = append(rows, []string{
			sheet.On.String(), "going out", row.Ref, row.Customer, row.SlotName,
			row.Total.String(), row.Balance.String(), row.Status.Name(),
		})
	}
	sendCSV(w, "day-"+sheet.On.String()+".csv", rows)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// read decodes a request body, refusing a field the tool does not know. A
// misspelled field that was silently ignored is a figure the shopkeeper thinks
// they entered and did not.
func read(r *http.Request, into any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxJSONBody))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("the tool could not read what was sent: %w", err)
	}
	return nil
}

// ok sends a successful answer.
func ok(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The answer has already begun; there is nothing useful left to say.
		return
	}
}

// fail sends an error the page can show as it is.
func fail(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// sendCSV writes a sheet out for the shop's own records.
func sendCSV(w http.ResponseWriter, name string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)

	// A byte-order mark, so that a spreadsheet opening this file reads the
	// rupee and peso signs and the Devanagari and Tamil names as what they are
	// rather than as mojibake.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	out := csv.NewWriter(w)
	if err := out.WriteAll(rows); err != nil {
		return
	}
	out.Flush()
}
