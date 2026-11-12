package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"clipboard-share/internal/api"
	"clipboard-share/internal/discovery"
)

func main() {
	port := "8765"
	if p := os.Getenv("CLIPBOARD_SHARE_PORT"); p != "" {
		port = p
	}

	if err := discovery.Announce("clipboard-share", port); err != nil {
		log.Printf("warning: mDNS announce failed: %v", err)
	}

	r := api.NewRouter()
	addr := ":" + port
	log.Printf("Clipboard Share server listening on %s", addr)

	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("shutting down...")
	_ = srv.Close()
}
