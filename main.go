// Command whatsapp-catalog-order-book keeps a small shop's product catalogue
// and its order book, and builds the messages and links the shopkeeper sends.
//
// A home baker, a pickle maker or a tiffin seller takes orders in a chat
// thread. The orders work; the thread does not. Nothing adds up, nothing is in
// one place, and the message that has to go back — what was ordered, what it
// comes to, when it is coming, how to pay — is typed out by hand every time,
// which is where the mistakes are.
//
// So this tool holds the catalogue and the book on the shop's own machine, adds
// everything up, and hands over the message already written, with a link that
// opens the customer's chat with that message in it.
//
// It sends nothing. It has no messaging account, no payment account, no API key
// and no network connection of any kind. It produces text and links; a person
// reads them and sends them, from their own phone, under their own name.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// webUI is the interface, built into the binary so the tool is one file.
//
//go:embed web
var webUI embed.FS

const (
	defaultAddr = "127.0.0.1:8080"
	defaultData = "./catalog-order-book-data.json"

	// maxJSONBody caps what one request may send. A catalogue of a few hundred
	// products and an order with a few hundred lines are both well inside it.
	maxJSONBody = 2 << 20 // 2 MiB
)

func main() {
	log.SetFlags(0)

	addr := flag.String("addr", defaultAddr, "address to serve the interface on")
	data := flag.String("data", defaultData, "file to keep the catalogue and the order book in")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "A product catalogue and order book for a small shop, with the chat")
		fmt.Fprintln(out, "messages and links already written. Serves a local web interface and")
		fmt.Fprintln(out, "keeps everything in one file. It sends nothing itself.")
		fmt.Fprintln(out, "\nUsage:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(*addr, *data); err != nil {
		log.Fatalf("catalogue and order book: %v", err)
	}
}

// embeddedAssets is the interface as it sits inside the binary.
func embeddedAssets() (fs.FS, error) {
	assets, err := fs.Sub(webUI, "web")
	if err != nil {
		return nil, fmt.Errorf("reading the built-in interface: %w", err)
	}
	return assets, nil
}

// run opens the data file, serves the interface and waits for an interrupt.
func run(addr, dataPath string) error {
	st, err := openStore(dataPath)
	if err != nil {
		return err
	}

	assets, err := embeddedAssets()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	(&api{store: st}).routes(mux)
	mux.Handle("GET /", http.FileServerFS(assets))

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot serve on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           guard(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	absData, absErr := filepath.Abs(dataPath)
	if absErr != nil {
		absData = dataPath
	}
	log.Printf("Catalogue and order book")
	log.Printf("  open   http://%s", listener.Addr())
	log.Printf("  book   %s", absData)
	log.Printf("  stop   Ctrl-C")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()

	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Printf("stopping")
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

// guard wraps the whole interface in the few protections a local web tool
// needs: it refuses a request a page on another site has sent to it, and tells
// the browser not to guess at content types. A shop's order book has its
// customers' names, numbers and addresses in it, and a page open in another tab
// must not be able to read them.
func guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")

		if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r.Host) {
			http.Error(w, "this request came from another site and was refused", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether an Origin header belongs to the host this server
// was reached on.
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
