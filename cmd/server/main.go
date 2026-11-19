package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"nudge/internal/api"
	"nudge/internal/clipboard"
	"nudge/internal/discovery"
	"nudge/internal/input"
	"nudge/internal/pair"
	"nudge/internal/tlscert"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "pair" {
		os.Exit(runPairClient(os.Args[2:], os.Stdout, os.Stderr))
	}
	runServer()
}

// newRouter wires the clipboard, pairing and remote-input routes; /health
// reports api.ProtocolVersion and Version.
func newRouter(pairer *pair.Broker, injector input.Injector) http.Handler {
	r := api.NewRouter(clipboardStore{}, pairer, Version)
	api.AttachPairing(r, pairer)
	api.AttachInputWS(r, pairer, injector, api.DefaultInputRate)
	return r
}

// instanceName returns the hostname to announce over mDNS so clients show
// the real machine name ("omarchy", "livingroom-pc"), falling back to
// "nudge" when the hostname is unusable.
func instanceName() string {
	return instanceNameFrom(os.Hostname())
}

func instanceNameFrom(name string, err error) string {
	if err != nil {
		return "nudge"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "nudge"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

// serverPort returns NUDGE_PORT or 8765.
func serverPort() string {
	if p := pair.EnvOverride("PORT"); p != "" {
		return p
	}
	return "8765"
}

// pairNotification returns the desktop notification shown for a new request;
// code is the verification code the phone shows for the same certificate.
func pairNotification(deviceName, code string) (title, body string) {
	return "Nudge", fmt.Sprintf("Pair request from %s, code %s — nudge pair --last", deviceName, code)
}

func runServer() {
	setupLogging()
	log.Printf("config dir: %s", pair.ConfigDir())
	port := serverPort()

	pairer, err := pair.Load()
	if err != nil {
		log.Fatalf("pairing state load failed: %v", err)
	}

	// The self-signed certificate lives next to the state file; phones pin its
	// fingerprint and compare the code when pairing.
	certDir := filepath.Dir(pair.StatePath())
	cert, fingerprint, code, err := tlscert.LoadOrCreate(certDir, log.Printf)
	if err != nil {
		log.Fatalf("TLS certificate failed: %v", err)
	}
	pairer.SetVerification(fingerprint, code)
	log.Printf("TLS certificate: %s", filepath.Join(certDir, tlscert.CertFile))
	log.Printf("verification code: %s", code)

	if err := clipboard.Init(); err != nil {
		log.Fatalf("clipboard init failed: %v", err)
	}

	stopAnnounce, err := discovery.Announce(instanceName(), port)
	if err != nil {
		log.Printf("warning: mDNS announce failed: %v", err)
	} else {
		defer stopAnnounce()
	}

	control, err := pair.ListenControl(pairer)
	if err != nil {
		if controlFailureFatal(runtime.GOOS) {
			log.Fatalf("pairing control socket failed: %v", err)
		}
		log.Printf("warning: pairing control socket failed, `nudge pair` is unavailable; approve phones from the dialog: %v", err)
	} else {
		log.Printf("pairing control socket: %s", pair.ControlPath())
		defer func() {
			if err := control.Close(); err != nil {
				log.Printf("closing pairing control socket failed: %v", err)
			}
		}()
	}

	// Windows only: a notification-area icon (status and paired phones) that
	// also announces new requests; a no-op elsewhere. Failure keeps serving.
	tray := startTray(port, pairer, log.Printf)
	defer tray.Close()

	// Linux: best-effort notify-send. Windows: an in-process Yes/No dialog
	// that calls the broker directly, so it works without the socket.
	pairer.OnRequest(withTrayNotice(tray, pairRequestHandler(pairer, log.Printf)))

	r := newRouter(pairer, input.New())
	addr := ":" + port
	srv := &http.Server{Addr: addr, Handler: r, TLSConfig: serverTLSConfig(cert)}

	serveErr := make(chan error, 1)
	go func() {
		// HTTPS only: the certificate comes from TLSConfig, so no file paths.
		if err := srv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	log.Printf("Nudge server listening on %s (HTTPS)", addr)

	// On Windows SIGINT is Ctrl+C/Ctrl+Break and SIGTERM is console close,
	// logoff or shutdown; a windowsgui process gets neither and is simply
	// terminated, which is fine (state is saved on every change).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serveErr:
		log.Printf("server failed: %v", err)
	case <-sig:
		log.Println("shutting down...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

// serverTLSConfig serves the one self-signed certificate with TLS 1.2 or later.
func serverTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
}

type pairClientResponse struct {
	OK       bool               `json:"ok"`
	Code     string             `json:"code"`
	Error    string             `json:"error"`
	Message  string             `json:"message"`
	Requests []pair.RequestInfo `json:"requests"`
	Devices  []pair.DeviceInfo  `json:"devices"`
}

func runPairClient(args []string, stdout, stderr io.Writer) int {
	command, err := pairClientCommand(args)
	if err != nil {
		fmt.Fprintf(stderr, "nudge pair: %v\n", err)
		fmt.Fprintln(stderr, "usage: nudge pair list | --last | deny --request <id> | revoke --device <id>")
		return 1
	}

	conn, err := net.DialTimeout("unix", pair.ControlPath(), 2*time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "nudge pair: cannot connect to server at %s: %v (is the server running?)\n", pair.ControlPath(), err)
		return 1
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(conn).Encode(command); err != nil {
		fmt.Fprintf(stderr, "nudge pair: send command: %v\n", err)
		return 1
	}
	var response pairClientResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		fmt.Fprintf(stderr, "nudge pair: read server response: %v\n", err)
		return 1
	}
	if !response.OK {
		fmt.Fprintf(stderr, "nudge pair: %s\n", response.Error)
		return 1
	}
	if command["cmd"] == "list" {
		printPairList(stdout, response)
	} else if response.Message != "" {
		fmt.Fprintln(stdout, response.Message)
	}
	return 0
}

func pairClientCommand(args []string) (map[string]string, error) {
	if len(args) == 1 {
		switch args[0] {
		case "list":
			return map[string]string{"cmd": "list"}, nil
		case "--last":
			return map[string]string{"cmd": "approve-last"}, nil
		}
	}
	if len(args) == 3 && args[0] == "deny" && args[1] == "--request" && args[2] != "" {
		return map[string]string{"cmd": "deny", "request": args[2]}, nil
	}
	if len(args) == 3 && args[0] == "revoke" && args[1] == "--device" && args[2] != "" {
		return map[string]string{"cmd": "revoke", "device": args[2]}, nil
	}
	return nil, fmt.Errorf("invalid arguments")
}

func printPairList(w io.Writer, response pairClientResponse) {
	if response.Code != "" {
		fmt.Fprintf(w, "Verification code: %s\n", response.Code)
	}
	fmt.Fprintln(w, "Pending requests:")
	if len(response.Requests) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, req := range response.Requests {
		fmt.Fprintf(w, "  %s  %s (%s) from %s\n", req.RequestID, req.Name, req.ID, req.IP)
	}
	fmt.Fprintln(w, "Approved devices:")
	if len(response.Devices) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, device := range response.Devices {
		fmt.Fprintf(w, "  %s  %s from %s (first approved %s)\n",
			device.ID, device.Name, device.IP, device.FirstApproved.Format(time.RFC3339))
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
