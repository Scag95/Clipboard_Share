package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"nudge/internal/tlscert"
)

// S4: the server speaks TLS 1.2+ with its own certificate and nothing else.
func TestServerTLSConfigServesTheCertificateWithTLS12OrLater(t *testing.T) {
	cert, fingerprint, _, err := tlscert.LoadOrCreate(t.TempDir(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = serverTLSConfig(cert)
	srv.StartTLS()
	defer srv.Close()
	addr := srv.Listener.Addr().String()

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("TLS 1.2+ handshake: %v", err)
	}
	peer := conn.ConnectionState().PeerCertificates
	conn.Close()
	if len(peer) != 1 || tlscert.Fingerprint(peer[0].RawSubjectPublicKeyInfo) != fingerprint {
		t.Fatalf("served certificate does not have fingerprint %s", fingerprint)
	}

	if conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS11}); err == nil {
		conn.Close()
		t.Fatal("TLS 1.1 handshake succeeded, want rejected")
	}

	resp, err := http.Get("http://" + addr + "/")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatal("plain HTTP got 200, want no plain HTTP service")
		}
	}
}
