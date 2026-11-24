package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sreenivas-sadhu-prabhakara/whatsapp-catalog-order-book/internal/orderbook"
)

// fileFormatVersion is written into the data file so that a later version of
// the tool can tell what it is reading, and refuse a file it does not
// understand rather than silently losing part of it.
const fileFormatVersion = 1

// maxOrders is how many orders are kept. A home kitchen takes a few a day; two
// thousand is a couple of years of them and leaves the file something a person
// can still open in a text editor.
const maxOrders = 2000

// fileData is the whole contents of the data file: the shop, what it sells, and
// every order. One file, readable, and nowhere else.
type fileData struct {
	Version   int                 `json:"version"`
	Shop      orderbook.Shop      `json:"shop"`
	Catalogue orderbook.Catalogue `json:"catalogue"`
	Orders    []orderbook.Order   `json:"orders"`
}

// normalise puts the file's contents into standard form.
func (d fileData) normalise() fileData {
	d.Version = fileFormatVersion
	d.Shop = d.Shop.Normalise()
	d.Catalogue = d.Catalogue.Normalise()

	orders := make([]orderbook.Order, 0, len(d.Orders))
	for _, o := range d.Orders {
		orders = append(orders, o.Normalise())
	}
	d.Orders = orders
	return d
}

// store keeps everything in memory and writes the whole file out after every
// change. A shop's catalogue and a couple of years of orders come to a few
// hundred kilobytes, so rewriting the file is both simpler and safer than
// editing it in place.
type store struct {
	mu   sync.RWMutex
	path string
	data fileData
}

// openStore reads the data file, or starts an empty shop if there is no file
// yet. It does not create the file until something is saved.
func openStore(path string) (*store, error) {
	s := &store{path: path, data: fileData{}.normalise()}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return s, nil
	}

	var loaded fileData
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, fmt.Errorf("reading %s: %w (move the file aside to start again)", path, err)
	}
	if loaded.Version > fileFormatVersion {
		return nil, fmt.Errorf("%s was written by a newer version of this tool (format %d, this build reads %d)",
			path, loaded.Version, fileFormatVersion)
	}
	s.data = loaded.normalise()
	return s, nil
}

// snapshot is a copy of the file's contents that a reader can use without
// holding the lock.
func (s *store) snapshot() fileData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := s.data
	out.Catalogue.Products = copyOf(s.data.Catalogue.Products)
	out.Orders = make([]orderbook.Order, len(s.data.Orders))
	for i, o := range s.data.Orders {
		o.Lines = copyOf(o.Lines)
		out.Orders[i] = o
	}
	return out
}

// copyOf duplicates a slice, keeping an empty one empty rather than letting it
// come out as nothing at all. A shop with nothing in its catalogue yet has to
// answer with an empty list, not with a null the page has to guess about.
func copyOf[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// change applies a change and saves the file. If saving fails the change is
// rolled back, so what is in memory always matches what is on disk.
func (s *store) change(apply func(d *fileData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := s.data
	before.Catalogue.Products = copyOf(s.data.Catalogue.Products)
	before.Orders = make([]orderbook.Order, len(s.data.Orders))
	for i, o := range s.data.Orders {
		o.Lines = copyOf(o.Lines)
		before.Orders[i] = o
	}

	if err := apply(&s.data); err != nil {
		s.data = before
		return err
	}
	s.data = s.data.normalise()
	if len(s.data.Orders) > maxOrders {
		s.data.Orders = s.data.Orders[len(s.data.Orders)-maxOrders:]
	}
	if err := s.save(); err != nil {
		s.data = before
		return err
	}
	return nil
}

// save writes the whole file atomically: a temporary file in the same
// directory, flushed to disk, then renamed over the old one. A crash therefore
// leaves either the old file or the new one, never half of either.
func (s *store) save() error {
	s.data.Version = fileFormatVersion

	body, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("preparing the order book: %w", err)
	}
	body = append(body, '\n')

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // a no-op once the rename has succeeded

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	// A shop's order book has its customers' names, numbers and addresses in
	// it, so the file is readable only by the user who runs the tool.
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replacing %s: %w", s.path, err)
	}
	return nil
}

// putShop replaces the shop's own details.
func (s *store) putShop(next orderbook.Shop) (orderbook.Shop, error) {
	next = next.Normalise()
	err := s.change(func(d *fileData) error {
		d.Shop = next
		return nil
	})
	return next, err
}

// putProduct puts a product into the catalogue, replacing the one with the same
// code if there is one. It stamps the day the prices last changed where the
// price has actually moved, so the shared catalogue text can say when its
// figures are from.
func (s *store) putProduct(next orderbook.Product) (orderbook.Product, error) {
	next = next.Normalise()
	if next.Code == "" {
		return next, errors.New("a product needs a code, which is what an order line refers to")
	}

	err := s.change(func(d *fileData) error {
		existing, found := d.Catalogue.Find(next.Code)
		if !found || existing.Price != next.Price {
			d.Catalogue.PricesChangedOn = orderbook.Today()
		}
		d.Catalogue = d.Catalogue.Upsert(next)
		return nil
	})
	return next, err
}

// deleteProduct takes a product off the catalogue. It does not touch any order:
// an order already taken keeps its own copy of what was ordered and what it
// cost.
func (s *store) deleteProduct(code string) error {
	return s.change(func(d *fileData) error {
		next, removed := d.Catalogue.Remove(code)
		if !removed {
			return fmt.Errorf("there is nothing in the catalogue called %q", code)
		}
		d.Catalogue = next
		return nil
	})
}

// newOrder is an order as the page asks for one: the lines name products by
// code and a quantity, and the prices are taken from the catalogue here, now,
// and kept.
type newOrder struct {
	Customer    orderbook.Customer   `json:"customer"`
	Fulfilment  orderbook.Fulfilment `json:"fulfilment"`
	Wanted      orderbook.Date       `json:"wanted"`
	Slot        orderbook.Slot       `json:"slot"`
	DeliveryFee orderbook.Money      `json:"deliveryFee"`
	Discount    orderbook.Money      `json:"discount"`
	// DiscountReason says why there is a discount.
	DiscountReason string    `json:"discountReason"`
	Note           string    `json:"note"`
	Lines          []newLine `json:"lines"`
}

// newLine is one line of a new order as the page asks for it.
type newLine struct {
	Code string        `json:"code"`
	Qty  orderbook.Qty `json:"qty"`
	Note string        `json:"note"`
	// Price, where the shop gives one, overrides the catalogue price for this
	// line: a one-off price agreed with a customer, written down at the moment
	// it was agreed. The catalogue is not touched.
	Price *orderbook.Money `json:"price"`
}

// addOrder takes a new order, snapshotting each line's price from the catalogue
// as it stands at this moment.
func (s *store) addOrder(in newOrder) (orderbook.Order, error) {
	var out orderbook.Order

	err := s.change(func(d *fileData) error {
		order := orderbook.Order{
			Ref:            orderbook.NextRef(d.Orders),
			PlacedOn:       orderbook.Today(),
			Customer:       in.Customer,
			Fulfilment:     in.Fulfilment,
			Wanted:         in.Wanted,
			Slot:           in.Slot,
			DeliveryFee:    in.DeliveryFee,
			Discount:       in.Discount,
			DiscountReason: in.DiscountReason,
			Note:           in.Note,
			Status:         orderbook.Taken,
		}

		for _, line := range in.Lines {
			product, found := d.Catalogue.Find(line.Code)
			if !found {
				return fmt.Errorf("there is nothing in the catalogue called %q", line.Code)
			}
			if line.Price != nil {
				product.Price = *line.Price
			}
			order.Lines = append(order.Lines, orderbook.LineFor(product, line.Qty, line.Note))
		}

		order = order.Normalise()
		d.Orders = append(d.Orders, order)
		out = order
		return nil
	})
	return out, err
}

// putOrder replaces an order with the one sent, keeping each line's price
// exactly as it arrives. The page sends back the prices it was given, so an
// order edited after a catalogue price change still carries the price it was
// taken at.
func (s *store) putOrder(next orderbook.Order) (orderbook.Order, error) {
	next = next.Normalise()

	err := s.change(func(d *fileData) error {
		at, found := orderbook.FindOrder(d.Orders, next.Ref)
		if !found {
			return fmt.Errorf("there is no order called %q", next.Ref)
		}
		// The reference, the day it was taken and the status are not the page's
		// to change here: there are endpoints of their own for the status, and
		// the other two are what the order is.
		next.Ref = d.Orders[at].Ref
		next.PlacedOn = d.Orders[at].PlacedOn
		next.Status = d.Orders[at].Status

		d.Orders[at] = next
		return nil
	})
	return next, err
}

// addLine puts one more line on an existing order, at today's catalogue price.
func (s *store) addLine(ref, code string, qty orderbook.Qty, note string) (orderbook.Order, error) {
	var out orderbook.Order

	err := s.change(func(d *fileData) error {
		at, found := orderbook.FindOrder(d.Orders, ref)
		if !found {
			return fmt.Errorf("there is no order called %q", ref)
		}
		product, inCatalogue := d.Catalogue.Find(code)
		if !inCatalogue {
			return fmt.Errorf("there is nothing in the catalogue called %q", code)
		}

		order := d.Orders[at]
		order.Lines = append(copyOf(order.Lines), orderbook.LineFor(product, qty, note))
		d.Orders[at] = order.Normalise()
		out = d.Orders[at]
		return nil
	})
	return out, err
}

// setStatus moves an order to another stage, applying the rule about which
// stage may follow which.
func (s *store) setStatus(ref string, next orderbook.Status) (orderbook.Order, error) {
	var out orderbook.Order

	err := s.change(func(d *fileData) error {
		at, found := orderbook.FindOrder(d.Orders, ref)
		if !found {
			return fmt.Errorf("there is no order called %q", ref)
		}
		if err := d.Orders[at].Status.CanMoveTo(next); err != nil {
			return err
		}
		d.Orders[at].Status = next.Normalise()
		out = d.Orders[at]
		return nil
	})
	return out, err
}

// setPayment records money received against an order.
func (s *store) setPayment(ref string, paid orderbook.Money, note string) (orderbook.Order, error) {
	var out orderbook.Order

	err := s.change(func(d *fileData) error {
		at, found := orderbook.FindOrder(d.Orders, ref)
		if !found {
			return fmt.Errorf("there is no order called %q", ref)
		}
		if paid < 0 {
			return errors.New("an amount received cannot be less than nothing")
		}
		d.Orders[at].Paid = paid
		d.Orders[at].PaymentNote = note
		out = d.Orders[at]
		return nil
	})
	return out, err
}

// deleteOrder takes an order out of the book altogether. Cancelling is almost
// always the right thing instead, which is why the page asks.
func (s *store) deleteOrder(ref string) error {
	return s.change(func(d *fileData) error {
		at, found := orderbook.FindOrder(d.Orders, ref)
		if !found {
			return fmt.Errorf("there is no order called %q", ref)
		}
		d.Orders = append(d.Orders[:at], d.Orders[at+1:]...)
		return nil
	})
}

// loadSample replaces everything with a worked shop.
func (s *store) loadSample(market orderbook.Market) error {
	return s.change(func(d *fileData) error {
		shop, catalogue, orders := orderbook.Sample(market, orderbook.Today())
		d.Shop = shop
		d.Catalogue = catalogue
		d.Orders = orders
		return nil
	})
}

// clear empties the book, keeping the shop's own details and its catalogue —
// which are the long part to type and are not what anybody wants to throw away.
func (s *store) clear() error {
	return s.change(func(d *fileData) error {
		d.Orders = nil
		return nil
	})
}
