package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"nudge/internal/api"
	"nudge/internal/clipboard"
	"nudge/internal/discovery"
	"nudge/internal/input"
	"nudge/internal/pair"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "pair" {
		os.Exit(runPairClient(os.Args[2:], os.Stdout, os.Stderr))
	}
	runServer()
}

// migrateConfig moves the pre-rename ~/.config/clipboard-share directory to
// ~/.config/nudge once. Only the server migrates. Failure (or a state override
// inside the old directory) is reported and the old directory is left as it
// was; startup continues with the effective dir, so pairings stay available.
func migrateConfig(logf func(format string, args ...any)) {
	moved, err := pair.MigrateLegacyConfig()
	if err != nil {
		logf("nudge: config migration skipped, %s left untouched: %v\n", pair.LegacyConfigDir(), err)
		return
	}
	if moved {
		logf("nudge: moved %s to %s\n", pair.LegacyConfigDir(), pair.ConfigDir())
	}
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

// serverPort returns NUDGE_PORT (or CLIPBOARD_SHARE_PORT) or 8765.
func serverPort() string {
	if p := pair.EnvOverride("PORT"); p != "" {
		return p
	}
	return "8765"
}

// pairNotification returns the desktop notification shown for a new request.
func pairNotification(deviceName string) (title, body string) {
	return "Nudge", fmt.Sprintf("Pair request from %s — nudge pair --last", deviceName)
}

func runServer() {
	setupLogging()
	migrateConfig(log.Printf)
	log.Printf("config dir: %s", pair.EffectiveConfigDir())
	port := serverPort()

	pairer, err := pair.Load()
	if err != nil {
		log.Fatalf("pairing state load failed: %v", err)
	}

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

	// Linux: best-effort notify-send. Windows: an in-process Yes/No dialog
	// that calls the broker directly, so it works without the socket.
	pairer.OnRequest(pairRequestHandler(pairer, log.Printf))

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
	log.Printf("Nudge server listening on %s", addr)

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

type pairClientResponse struct {
	OK       bool               `json:"ok"`
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

	// No migration here: the CLI reaches the socket of the effective dir, so a
	// not-yet-migrated or still-running old server stays reachable.
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
