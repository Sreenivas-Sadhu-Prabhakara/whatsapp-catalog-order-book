package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sreenivas-sadhu-prabhakara/whatsapp-catalog-order-book/internal/orderbook"
)

// openTestStore is a store in a directory of its own.
func openTestStore(t *testing.T) *store {
	t.Helper()

	path := filepath.Join(t.TempDir(), "book.json")
	st, err := openStore(path)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	return st
}

func TestAMissingFileStartsAnEmptyShopAndIsNotCreatedUntilSomethingIsSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.json")

	st, err := openStore(path)
	if err != nil {
		t.Fatalf("opening a store with no file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the file was created before anything was saved")
	}
	if len(st.snapshot().Orders) != 0 || len(st.snapshot().Catalogue.Products) != 0 {
		t.Fatal("an empty store is not empty")
	}

	if _, err := st.putShop(orderbook.Shop{Name: "Aaji's Kitchen"}); err != nil {
		t.Fatalf("saving the shop: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
}

func TestTheFileIsWrittenPrivateToItsOwnerAndIsReadableJSON(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading the worked shop: %v", err)
	}

	info, err := os.Stat(st.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// The book holds customers' names, numbers and addresses.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the file is %04o, want 0600", perm)
	}

	raw, err := os.ReadFile(st.path)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	var into fileData
	if err := json.Unmarshal(raw, &into); err != nil {
		t.Fatalf("the file is not readable JSON: %v", err)
	}
	if into.Version != fileFormatVersion {
		t.Fatalf("the file says format %d", into.Version)
	}
	// Money is text in the file, not a float, so nothing that reads it can
	// round a price.
	if !strings.Contains(string(raw), `"price": "320.00"`) {
		t.Fatalf("a price is not stored as text:\n%s", firstLines(string(raw), 40))
	}
}

func TestTheFileSurvivesBeingClosedAndOpenedAgain(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.PH); err != nil {
		t.Fatalf("loading: %v", err)
	}
	before := st.snapshot()

	again, err := openStore(st.path)
	if err != nil {
		t.Fatalf("opening again: %v", err)
	}
	after := again.snapshot()

	if after.Shop.Name != before.Shop.Name {
		t.Fatalf("the shop came back as %q", after.Shop.Name)
	}
	if len(after.Catalogue.Products) != len(before.Catalogue.Products) {
		t.Fatalf("%d products came back of %d", len(after.Catalogue.Products), len(before.Catalogue.Products))
	}
	if len(after.Orders) != len(before.Orders) {
		t.Fatalf("%d orders came back of %d", len(after.Orders), len(before.Orders))
	}
	if after.Orders[0].Lines[0].UnitPrice != before.Orders[0].Lines[0].UnitPrice {
		t.Fatalf("a line's price came back as %s, was %s",
			after.Orders[0].Lines[0].UnitPrice, before.Orders[0].Lines[0].UnitPrice)
	}
}

func TestAFileFromANewerVersionIsRefusedRatherThanPartlyRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.json")
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, err := openStore(path); err == nil {
		t.Fatal("a file from a newer version was read")
	}
}

func TestAnUnreadableFileIsRefusedWithAWayOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.json")
	if err := os.WriteFile(path, []byte("this is not JSON"), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	_, err := openStore(path)
	if err == nil {
		t.Fatal("a file that is not JSON was read")
	}
	if !strings.Contains(err.Error(), "move the file aside") {
		t.Fatalf("the error gives no way out: %v", err)
	}

	// An empty file is a new book, not an error: a shop that creates the file
	// by hand should not be stuck.
	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(empty, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, err := openStore(empty); err != nil {
		t.Fatalf("an empty file was refused: %v", err)
	}
}

func TestANewOrderTakesTodaysPricesAndKeepsThem(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	saved, err := st.addOrder(newOrder{
		Customer:   orderbook.Customer{Name: "Test Customer", Phone: "9876543299"},
		Fulfilment: orderbook.Pickup,
		Wanted:     orderbook.Today().AddDays(1),
		Lines: []newLine{
			{Code: "mango 500", Qty: orderbook.Whole(2)},
		},
	})
	if err != nil {
		t.Fatalf("taking the order: %v", err)
	}
	if saved.Ref == "" {
		t.Fatal("the order has no reference")
	}
	if len(saved.Lines) != 1 || saved.Lines[0].Code != "MANGO-500" {
		t.Fatalf("the line is %+v", saved.Lines)
	}
	if saved.Lines[0].UnitPrice != 32000 {
		t.Fatalf("the line was taken at %s, want 320.00", saved.Lines[0].UnitPrice)
	}
	if saved.Lines[0].Name != "Mango pickle" || saved.Lines[0].Pack != "500 g" {
		t.Fatalf("the line did not copy the product: %+v", saved.Lines[0])
	}

	// Put the price up. The order must not move.
	mango, found := st.snapshot().Catalogue.Find("MANGO-500")
	if !found {
		t.Fatal("the product vanished")
	}
	mango.Price = 40000
	if _, err := st.putProduct(mango); err != nil {
		t.Fatalf("changing the price: %v", err)
	}

	at, ok := orderbook.FindOrder(st.snapshot().Orders, saved.Ref)
	if !ok {
		t.Fatal("the order vanished")
	}
	again := st.snapshot().Orders[at]
	if again.Lines[0].UnitPrice != 32000 {
		t.Fatalf("the line's price became %s after the catalogue changed", again.Lines[0].UnitPrice)
	}
	if again.Add().Total != 64000 {
		t.Fatalf("the order came to %s, want 640.00", again.Add().Total)
	}
}

func TestAOneOffPriceIsRecordedAsItWasAgreed(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	agreed := orderbook.Money(30000)
	saved, err := st.addOrder(newOrder{
		Customer:   orderbook.Customer{Name: "Test Customer", Phone: "9876543299"},
		Fulfilment: orderbook.Pickup,
		Wanted:     orderbook.Today(),
		Lines: []newLine{
			{Code: "MANGO-500", Qty: orderbook.Whole(1), Price: &agreed, Note: "price agreed on the phone"},
		},
	})
	if err != nil {
		t.Fatalf("taking the order: %v", err)
	}
	if saved.Lines[0].UnitPrice != 30000 {
		t.Fatalf("the agreed price became %s", saved.Lines[0].UnitPrice)
	}
	// And the catalogue is untouched.
	mango, _ := st.snapshot().Catalogue.Find("MANGO-500")
	if mango.Price != 32000 {
		t.Fatalf("a one-off price changed the catalogue to %s", mango.Price)
	}
}

func TestAnOrderForSomethingNotInTheCatalogueIsRefused(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	_, err := st.addOrder(newOrder{
		Customer: orderbook.Customer{Name: "Test", Phone: "9876543299"},
		Lines: []newLine{
			{Code: "NOT-A-THING", Qty: orderbook.Whole(1)},
		},
	})
	if err == nil {
		t.Fatal("an order was taken for something the shop does not sell")
	}
	if len(st.snapshot().Orders) != 4 {
		t.Fatalf("the refused order left %d orders in the book, want the worked shop's 4",
			len(st.snapshot().Orders))
	}
}

func TestPuttingAProductInStampsThePriceDayOnlyWhenThePriceMoved(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	before := st.snapshot().Catalogue.PricesChangedOn

	// Changing a note is not a price change.
	mango, _ := st.snapshot().Catalogue.Find("MANGO-500")
	mango.Note = "a different note"
	if _, err := st.putProduct(mango); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if got := st.snapshot().Catalogue.PricesChangedOn; got != before {
		t.Fatalf("changing a note moved the price day from %s to %s", before, got)
	}

	// Changing the price is.
	mango.Price = 40000
	if _, err := st.putProduct(mango); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if got := st.snapshot().Catalogue.PricesChangedOn; got != orderbook.Today() {
		t.Fatalf("a price change left the price day at %s", got)
	}

	// A new product is a price change too: the catalogue the customer holds is
	// now out of date.
	st2 := openTestStore(t)
	if _, err := st2.putProduct(orderbook.Product{Code: "NEW", Name: "New thing", Price: 10000}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if got := st2.snapshot().Catalogue.PricesChangedOn; got != orderbook.Today() {
		t.Fatalf("a new product left the price day at %s", got)
	}
}

func TestAProductWithNoCodeIsRefused(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.putProduct(orderbook.Product{Name: "No code", Price: 10000}); err == nil {
		t.Fatal("a product with no code was saved; nothing could ever order it")
	}
	if len(st.snapshot().Catalogue.Products) != 0 {
		t.Fatal("the refused product went in anyway")
	}
}

func TestRemovingAProductLeavesEveryOrderExactlyAsItWas(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	before := map[string]orderbook.Totals{}
	for _, o := range st.snapshot().Orders {
		before[o.Ref] = o.Add()
	}

	if err := st.deleteProduct("MANGO-500"); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if err := st.deleteProduct("MANGO-500"); err == nil {
		t.Fatal("removing it twice succeeded twice")
	}

	for _, o := range st.snapshot().Orders {
		if o.Add() != before[o.Ref] {
			t.Fatalf("%s changed when the product was removed:\n  before: %+v\n   after: %+v",
				o.Ref, before[o.Ref], o.Add())
		}
	}
}

func TestAFailedChangeIsRolledBack(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}
	before := st.snapshot()

	// Moving a delivered order back to taken is refused by the rule, and must
	// leave nothing changed.
	if _, err := st.setStatus("ORD-0001", orderbook.Taken); err == nil {
		t.Fatal("a delivered order was moved back to taken")
	}

	after := st.snapshot()
	if len(after.Orders) != len(before.Orders) {
		t.Fatalf("%d orders after a refused change, was %d", len(after.Orders), len(before.Orders))
	}
	for i := range after.Orders {
		if after.Orders[i].Status != before.Orders[i].Status {
			t.Fatalf("%s is now %s, was %s", after.Orders[i].Ref, after.Orders[i].Status, before.Orders[i].Status)
		}
	}
}

func TestAddingALineTakesTodaysPrice(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	// Put the lime jar up, then add a line for it. The new line carries the new
	// price; the lines already on the order do not.
	lime, _ := st.snapshot().Catalogue.Find("LIME-400")
	lime.Price = 30000
	if _, err := st.putProduct(lime); err != nil {
		t.Fatalf("saving: %v", err)
	}

	order, err := st.addLine("ORD-0003", "LIME-400", orderbook.Whole(1), "extra jar")
	if err != nil {
		t.Fatalf("adding a line: %v", err)
	}
	last := order.Lines[len(order.Lines)-1]
	if last.UnitPrice != 30000 {
		t.Fatalf("the new line was taken at %s, want today's 300.00", last.UnitPrice)
	}
	if order.Lines[0].UnitPrice != 58000 {
		t.Fatalf("an existing line moved to %s", order.Lines[0].UnitPrice)
	}

	if _, err := st.addLine("ORD-0003", "NOT-A-THING", orderbook.Whole(1), ""); err == nil {
		t.Fatal("a line was added for something the shop does not sell")
	}
	if _, err := st.addLine("NO-SUCH-ORDER", "LIME-400", orderbook.Whole(1), ""); err == nil {
		t.Fatal("a line was added to an order that does not exist")
	}
}

func TestPuttingAnOrderBackKeepsWhatIsNotThePagesToChange(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	at, _ := orderbook.FindOrder(st.snapshot().Orders, "ORD-0002")
	order := st.snapshot().Orders[at]
	placed, status := order.PlacedOn, order.Status

	order.PlacedOn = orderbook.Date{Year: 2000, Month: 1, Day: 1}
	order.Status = orderbook.Delivered
	order.Customer.Name = "A New Name"
	// The page sends back the prices it was given, so an order edited after a
	// price change keeps the price it was taken at.
	order.Lines[0].UnitPrice = 11111

	saved, err := st.putOrder(order)
	if err != nil {
		t.Fatalf("saving the order: %v", err)
	}
	if saved.PlacedOn != placed {
		t.Fatalf("the day it was taken became %s", saved.PlacedOn)
	}
	if saved.Status != status {
		t.Fatalf("the status became %s; there is an endpoint of its own for that", saved.Status)
	}
	if saved.Customer.Name != "A New Name" {
		t.Fatalf("the customer's name is %q", saved.Customer.Name)
	}
	if saved.Lines[0].UnitPrice != 11111 {
		t.Fatalf("the price sent back was changed to %s", saved.Lines[0].UnitPrice)
	}

	if _, err := st.putOrder(orderbook.Order{Ref: "NO-SUCH-ORDER"}); err == nil {
		t.Fatal("an order that does not exist was saved")
	}
}

func TestEmptyingTheBookKeepsTheShopAndTheCatalogue(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	if err := st.clear(); err != nil {
		t.Fatalf("emptying: %v", err)
	}
	after := st.snapshot()
	if len(after.Orders) != 0 {
		t.Fatalf("%d orders left", len(after.Orders))
	}
	if after.Shop.Name == "" {
		t.Fatal("the shop was thrown away with the orders")
	}
	if len(after.Catalogue.Products) == 0 {
		t.Fatal("the catalogue was thrown away with the orders")
	}

	// And the next reference starts again, because nothing above it was ever
	// issued in this book.
	if got := orderbook.NextRef(after.Orders); got != "ORD-0001" {
		t.Fatalf("the next reference is %q", got)
	}
}

func TestDeletingAnOrderTakesOnlyThatOrder(t *testing.T) {
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}
	before := len(st.snapshot().Orders)

	if err := st.deleteOrder("ord-0002"); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if got := len(st.snapshot().Orders); got != before-1 {
		t.Fatalf("%d orders left, want %d", got, before-1)
	}
	if _, found := orderbook.FindOrder(st.snapshot().Orders, "ORD-0002"); found {
		t.Fatal("ORD-0002 is still there")
	}
	if err := st.deleteOrder("ORD-0002"); err == nil {
		t.Fatal("deleting it twice succeeded twice")
	}
}

func TestASnapshotCannotBeUsedToChangeTheStore(t *testing.T) {
	// The page gets a copy. A caller that edits what it was given must not be
	// editing the book.
	st := openTestStore(t)
	if err := st.loadSample(orderbook.IN); err != nil {
		t.Fatalf("loading: %v", err)
	}

	snapshot := st.snapshot()
	snapshot.Orders[0].Lines[0].UnitPrice = 1
	snapshot.Catalogue.Products[0].Price = 1

	again := st.snapshot()
	if again.Orders[0].Lines[0].UnitPrice == 1 {
		t.Fatal("editing a snapshot changed an order in the book")
	}
	if again.Catalogue.Products[0].Price == 1 {
		t.Fatal("editing a snapshot changed the catalogue in the book")
	}
}

func TestEmptyListsComeOutAsListsAndNotAsNothing(t *testing.T) {
	// The page has to be able to count them without checking for null first.
	st := openTestStore(t)
	snapshot := st.snapshot()

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if strings.Contains(string(raw), `"orders":null`) {
		t.Fatalf("an empty book encodes orders as null:\n%s", raw)
	}
	if strings.Contains(string(raw), `"products":null`) {
		t.Fatalf("an empty catalogue encodes products as null:\n%s", raw)
	}
}

// firstLines is for a failure message.
func firstLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
