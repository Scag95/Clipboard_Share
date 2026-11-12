package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clipboard-share/internal/api"
	"clipboard-share/internal/clipboard"
	"clipboard-share/internal/discovery"
	"clipboard-share/internal/input"
	"clipboard-share/internal/pair"
)

func main() {
	port := "8765"
	if p := os.Getenv("CLIPBOARD_SHARE_PORT"); p != "" {
		port = p
	}

	if err := clipboard.Init(); err != nil {
		log.Fatalf("clipboard init failed: %v", err)
	}

	stopAnnounce, err := discovery.Announce("clipboard-share", port)
	if err != nil {
		log.Printf("warning: mDNS announce failed: %v", err)
	} else {
		defer stopAnnounce()
	}

	pairer := pair.NewBroker()
	log.Printf("remote-control pairing PIN: %s", pairer.Pin())
	pairer.OnPINChange(func(p string) {
		log.Printf("pairing PIN rotated (too many failed attempts); new PIN: %s", p)
	})

	r := api.NewRouter(clipboardStore{})
	api.AttachPairing(r, pairer)
	api.AttachInputWS(r, pairer, input.New(), api.DefaultInputRate)
	addr := ":" + port
	srv := &http.Server{Addr: addr, Handler: r}

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	log.Printf("Clipboard Share server listening on %s", addr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serveErr:
		log.Fatalf("server failed: %v", err)
	case <-sig:
		log.Println("shutting down...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

// clipboardStore adapts the package-level clipboard API to api.Store.
type clipboardStore struct{}

func (clipboardStore) GetText() (string, error)  { return clipboard.GetText() }
func (clipboardStore) SetText(text string) error { return clipboard.SetText(text) }
func (clipboardStore) GetImage() ([]byte, error) { return clipboard.GetImage() }
func (clipboardStore) SetImage(data []byte) error {
	return clipboard.SetImage(data)
}
