// Package tlscert manages the server's self-signed TLS certificate and the
// fingerprint and verification code phones pin and compare when pairing.
//
// Fingerprint: lowercase hex SHA-256 of the certificate's SubjectPublicKeyInfo
// DER, so it survives a re-issued certificate with the same key. Code: the
// first 6 bytes of that hash as 12 uppercase hex digits, "XXXX-XXXX-XXXX". The
// Android app computes both the same way.
package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// CertFile and KeyFile live next to the pairing state file.
	CertFile = "tls.crt"
	KeyFile  = "tls.key"

	commonName = "Nudge"
	validYears = 20
)

// Fingerprint returns the lowercase hex SHA-256 of a SubjectPublicKeyInfo DER.
func Fingerprint(spkiDER []byte) string {
	sum := sha256.Sum256(spkiDER)
	return hex.EncodeToString(sum[:])
}

// Code returns the verification code of a fingerprint hex string: its first
// 12 hex digits, uppercase, in three groups of 4 joined by "-". It returns ""
// when the input does not start with 12 hex digits.
func Code(fingerprintHex string) string {
	if len(fingerprintHex) < 12 {
		return ""
	}
	head := strings.ToUpper(fingerprintHex[:12])
	if _, err := hex.DecodeString(head); err != nil {
		return ""
	}
	return head[0:4] + "-" + head[4:8] + "-" + head[8:12]
}

// LoadOrCreate returns the certificate stored in dir with its fingerprint and
// code. Missing files are generated (dir 0700, key 0600, PKCS#8). A pair that
// cannot be read, does not parse or does not match is replaced by a new one
// and logf reports it, because every paired phone then has to pair again.
func LoadOrCreate(dir string, logf func(format string, args ...any)) (tls.Certificate, string, string, error) {
	certPath, keyPath := filepath.Join(dir, CertFile), filepath.Join(dir, KeyFile)
	cert, leaf, err := load(certPath, keyPath)
	if err == nil {
		fp := Fingerprint(leaf.RawSubjectPublicKeyInfo)
		return cert, fp, Code(fp), nil
	}
	if !(errors.Is(err, os.ErrNotExist) && !exists(certPath) && !exists(keyPath)) && logf != nil {
		logf("warning: TLS certificate in %s unusable (%v); generating a new one, every phone has to pair again", dir, err)
	}
	certPEM, keyPEM, err := generate()
	if err != nil {
		return tls.Certificate{}, "", "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("create TLS certificate directory: %w", err)
	}
	if err := writeFile(keyPath, keyPEM); err != nil {
		return tls.Certificate{}, "", "", err
	}
	if err := writeFile(certPath, certPEM); err != nil {
		return tls.Certificate{}, "", "", err
	}
	cert, leaf, err = load(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, "", "", fmt.Errorf("reload generated TLS certificate: %w", err)
	}
	fp := Fingerprint(leaf.RawSubjectPublicKeyInfo)
	return cert, fp, Code(fp), nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func load(certPath, keyPath string) (tls.Certificate, *x509.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	// X509KeyPair parses both and rejects a key that does not match.
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	if len(cert.Certificate) != 1 {
		return tls.Certificate{}, nil, fmt.Errorf("%s holds %d certificates, want 1", certPath, len(cert.Certificate))
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	cert.Leaf = leaf
	return cert, leaf, nil
}

func generate() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate TLS key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	serial.Add(serial, big.NewInt(1)) // never zero
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(validYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create TLS certificate: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode TLS key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), nil
}

// writeFile replaces path atomically with a 0600 file (on Windows Chmod only
// clears read-only; the per-user %APPDATA% ACL protects it).
func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tls-*.tmp")
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
