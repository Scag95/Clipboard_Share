package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The shared S3 vector: the Android test suite pins the same pair.
const vectorFingerprint = "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b"

func TestCodeOfTheSharedVector(t *testing.T) {
	if got := Code(vectorFingerprint); got != "6B86-B273-FF34" {
		t.Fatalf("Code(vector) = %q, want 6B86-B273-FF34", got)
	}
	if got := Code(strings.ToUpper(vectorFingerprint)); got != "6B86-B273-FF34" {
		t.Fatalf("Code(upper vector) = %q, want 6B86-B273-FF34", got)
	}
	for _, bad := range []string{"", "6b86b273ff3", "zz86b273ff34fce1"} {
		if got := Code(bad); got != "" {
			t.Errorf("Code(%q) = %q, want empty", bad, got)
		}
	}
}

func TestFingerprintIsLowercaseSHA256Hex(t *testing.T) {
	// "1" hashes to the vector fingerprint.
	if got := Fingerprint([]byte("1")); got != vectorFingerprint {
		t.Fatalf("Fingerprint(\"1\") = %q, want %q", got, vectorFingerprint)
	}
}

type logRecorder struct{ lines []string }

func (l *logRecorder) logf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func loadOK(t *testing.T, dir string, log *logRecorder) (*x509.Certificate, string, string) {
	t.Helper()
	cert, fp, code, err := LoadOrCreate(dir, log.logf)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if len(cert.Certificate) != 1 {
		t.Fatalf("certificate chain length = %d, want 1", len(cert.Certificate))
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if want := hex.EncodeToString(sum[:]); fp != want {
		t.Fatalf("fingerprint = %q, want SPKI SHA-256 %q", fp, want)
	}
	if want := Code(fp); code != want {
		t.Fatalf("code = %q, want %q", code, want)
	}
	if !regexp.MustCompile(`^[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$`).MatchString(code) {
		t.Fatalf("code %q does not have the XXXX-XXXX-XXXX shape", code)
	}
	return leaf, fp, code
}

func TestLoadOrCreateGeneratesTheContractedCertificate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nudge")
	var log logRecorder
	leaf, _, _ := loadOK(t, dir, &log)
	if len(log.lines) != 0 {
		t.Fatalf("first start logged %q, want no warning", log.lines)
	}
	if leaf.Subject.CommonName != "Nudge" {
		t.Errorf("CN = %q, want Nudge", leaf.Subject.CommonName)
	}
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		t.Errorf("public key = %T, want ECDSA P-256", leaf.PublicKey)
	}
	if leaf.Issuer.String() != leaf.Subject.String() {
		t.Errorf("issuer %q != subject %q", leaf.Issuer, leaf.Subject)
	}
	if err := leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature); err != nil {
		t.Errorf("certificate is not self-signed: %v", err)
	}
	validity := leaf.NotAfter.Sub(leaf.NotBefore)
	if min := 20*365*24*time.Hour - 48*time.Hour; validity < min || validity > 20*366*24*time.Hour {
		t.Errorf("validity = %v, want 20 years", validity)
	}
	if leaf.SerialNumber.Sign() <= 0 {
		t.Errorf("serial = %v, want a positive random serial", leaf.SerialNumber)
	}

	keyPEM, err := os.ReadFile(filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("tls.key is not a PKCS#8 PEM block: %q", keyPEM)
	}
	if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		t.Fatalf("tls.key is not PKCS#8: %v", err)
	}
	crtPEM, err := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if block, _ := pem.Decode(crtPEM); block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("tls.crt is not a CERTIFICATE PEM block: %q", crtPEM)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "tls.key"))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("tls.key mode = %o, want 600", perm)
		}
		info, err = os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("config dir mode = %o, want 700", perm)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir holds %v, want only tls.crt and tls.key", names)
	}
}

func TestLoadOrCreateReloadsTheSameKey(t *testing.T) {
	dir := t.TempDir()
	var log logRecorder
	_, fp1, code1 := loadOK(t, dir, &log)
	crt1, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))
	_, fp2, code2 := loadOK(t, dir, &log)
	crt2, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if fp1 != fp2 || code1 != code2 || string(crt1) != string(crt2) {
		t.Fatalf("reload changed identity: %s/%s -> %s/%s", fp1, code1, fp2, code2)
	}
	if len(log.lines) != 0 {
		t.Fatalf("reload logged %q, want no warning", log.lines)
	}
}

func TestTwoCertificatesHaveDifferentSerialsAndKeys(t *testing.T) {
	var log logRecorder
	a, fpA, _ := loadOK(t, t.TempDir(), &log)
	b, fpB, _ := loadOK(t, t.TempDir(), &log)
	if a.SerialNumber.Cmp(b.SerialNumber) == 0 {
		t.Errorf("two certificates share serial %v", a.SerialNumber)
	}
	if fpA == fpB {
		t.Errorf("two certificates share fingerprint %s", fpA)
	}
}

func TestBrokenFilesAreRegeneratedWithAWarning(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"corrupt key": func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "tls.key"), "garbage")
		},
		"corrupt certificate": func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "tls.crt"), "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n")
		},
		"missing key": func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "tls.key")); err != nil {
				t.Fatal(err)
			}
		},
		"missing certificate": func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "tls.crt")); err != nil {
				t.Fatal(err)
			}
		},
		"key from another pair": func(t *testing.T, dir string) {
			other := t.TempDir()
			var log logRecorder
			loadOK(t, other, &log)
			data, err := os.ReadFile(filepath.Join(other, "tls.key"))
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(dir, "tls.key"), string(data))
		},
	}
	for name, breakFiles := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			var log logRecorder
			_, before, _ := loadOK(t, dir, &log)
			breakFiles(t, dir)

			_, after, _ := loadOK(t, dir, &log)
			if after == before {
				t.Fatalf("fingerprint unchanged after %s; the broken pair was reused", name)
			}
			if len(log.lines) != 1 || !strings.Contains(log.lines[0], "warning") ||
				!strings.Contains(log.lines[0], "pair again") {
				t.Fatalf("log = %q, want one warning that phones must pair again", log.lines)
			}
			// The regenerated pair is stable from now on.
			var quiet logRecorder
			if _, again, _ := loadOK(t, dir, &quiet); again != after || len(quiet.lines) != 0 {
				t.Fatalf("regenerated pair not stable: %s -> %s, log %q", after, again, quiet.lines)
			}
		})
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
