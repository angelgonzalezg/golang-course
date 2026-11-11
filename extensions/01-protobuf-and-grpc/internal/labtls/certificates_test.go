package labtls

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Verify the generated certificate against its CA for each loopback name.
// Move verification time past expiry; the same certificate must then be rejected.
func TestLocalCertificateTrustNamesAndExpiry(t *testing.T) {
	material, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	client, err := ClientConfig(material.CA)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(material.Certificate)
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"localhost", "127.0.0.1", "::1"} {
		if _, err := certificate.Verify(x509.VerifyOptions{Roots: client.RootCAs, DNSName: name}); err != nil {
			t.Fatalf("local name %s is not verified: %v", name, err)
		}
	}
	if _, err := certificate.Verify(x509.VerifyOptions{Roots: client.RootCAs, DNSName: "localhost", CurrentTime: time.Now().Add(25 * time.Hour)}); err == nil {
		t.Fatal("expired local certificate still verified")
	}
}

// Write TLS files once, reject replacement, and load both configurations.
// Check private-key permissions on POSIX and prove a client needs only the public CA.
func TestCertificateFilesArePrivateAndNeverOverwritten(t *testing.T) {
	material, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "tls")
	if err := material.WriteDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if err := material.WriteDirectory(directory); err == nil {
		t.Fatal("existing certificate directory was accepted")
	}
	key, err := os.ReadFile(filepath.Join(directory, "server-key.pem"))
	if err != nil || !bytes.Equal(key, material.Key) {
		t.Fatalf("existing key changed: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(directory, "server-key.pem"))
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("private key permissions: %v, %v", info, err)
		}
	}
	if _, err := LoadServer(directory); err != nil {
		t.Fatal(err)
	}
	// A client needs only the public CA, never the server's private key.
	clientDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(clientDirectory, "ca.pem"), material.CA, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadClient(clientDirectory); err != nil {
		t.Fatal(err)
	}
}
