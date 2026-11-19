package discovery

import (
	"errors"
	"reflect"
	"strconv"
	"testing"
)

func TestServiceTypes(t *testing.T) {
	if ServiceType != "_nudge._tcp" {
		t.Fatalf("ServiceType = %q, want _nudge._tcp", ServiceType)
	}
}

type fakeRegistry struct {
	calls    []string
	txt      [][]string
	fail     map[string]error
	shutdown []string
}

func (f *fakeRegistry) install(t *testing.T) {
	t.Helper()
	prev := register
	register = func(name, service string, port int, txt []string) (func(), error) {
		f.txt = append(f.txt, txt)
		f.calls = append(f.calls, name+"|"+service+"|"+strconv.Itoa(port))
		if err := f.fail[service]; err != nil {
			return nil, err
		}
		return func() { f.shutdown = append(f.shutdown, service) }, nil
	}
	t.Cleanup(func() { register = prev })
}

func TestAnnounceRegistersOnlyNudgeAndShutsItDown(t *testing.T) {
	f := &fakeRegistry{}
	f.install(t)
	stop, err := Announce("omarchy", "8765")
	if err != nil {
		t.Fatalf("Announce error: %v", err)
	}
	want := []string{"omarchy|_nudge._tcp|8765"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("registrations = %q, want %q", f.calls, want)
	}
	// S2: the TXT record carries the protocol version (replacing version=1).
	if wantTXT := [][]string{{"protocol=1"}}; !reflect.DeepEqual(f.txt, wantTXT) {
		t.Fatalf("TXT records = %q, want %q", f.txt, wantTXT)
	}
	if len(f.shutdown) != 0 {
		t.Fatalf("shut down early: %q", f.shutdown)
	}
	stop()
	if !reflect.DeepEqual(f.shutdown, []string{ServiceType}) {
		t.Fatalf("shutdown = %q, want %q", f.shutdown, ServiceType)
	}
}

func TestAnnounceFailsWhenPrimaryFails(t *testing.T) {
	boom := errors.New("primary boom")
	f := &fakeRegistry{fail: map[string]error{ServiceType: boom}}
	f.install(t)
	stop, err := Announce("omarchy", "8765")
	if !errors.Is(err, boom) || stop != nil {
		t.Fatalf("Announce = (stop set: %v), %v; want nil, primary error", stop != nil, err)
	}
	if len(f.shutdown) != 0 || len(f.calls) != 1 {
		t.Fatalf("calls = %q, shutdown = %q; want only the failed primary attempt", f.calls, f.shutdown)
	}
}

func TestAnnounceRejectsBadPort(t *testing.T) {
	f := &fakeRegistry{}
	f.install(t)
	for _, port := range []string{"", "0", "70000", "x"} {
		if _, err := Announce("n", port); err == nil {
			t.Errorf("port %q accepted", port)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("registered despite bad port: %q", f.calls)
	}
}
