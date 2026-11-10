// Package labtls creates short-lived certificates for the local TLS lesson.
// It is not a certificate authority service or a production renewal system.
package labtls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Material holds PEM-encoded public CA, server certificate, and server private
// key bytes. Clients need only CA; never give them the server's Key.
type Material struct{ CA, Certificate, Key []byte }

func serialNumber() (*big.Int, error) {
	// Certificate serial numbers are positive; choose uniformly from 1..2^128-1.
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	limit.Sub(limit, big.NewInt(1))
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, err
	}
	return serial.Add(serial, big.NewInt(1)), nil
}

// Generate creates a 24-hour local CA/server pair for localhost and loopback IPs.
// The CA private key is discarded after signing, so renewal requires a new pair.
func Generate() (Material, error) {
	caPublic, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Material{}, err
	}
	caSerial, err := serialNumber()
	if err != nil {
		return Material{}, err
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber: caSerial, Subject: pkix.Name{CommonName: "Go course local CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, caPublic, caKey)
	if err != nil {
		return Material{}, err
	}
	serverPublic, serverKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Material{}, err
	}
	serverSerial, err := serialNumber()
	if err != nil {
		return Material{}, err
	}
	server := &x509.Certificate{
		SerialNumber: serverSerial, Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, server, ca, serverPublic, caKey)
	if err != nil {
		return Material{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return Material{}, err
	}
	// The CA private key is discarded. Only the public CA goes to clients.
	return Material{
		CA:          pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		Key:         pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}, nil
}

// ServerConfig loads this server's certificate/key and requires TLS 1.3 or newer.
// Client identity is supplied by bearer credentials, not client certificates.
func (m Material) ServerConfig() (*tls.Config, error) {
	certificate, err := tls.X509KeyPair(m.Certificate, m.Key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}, nil
}

// ClientConfig requires TLS 1.3 or newer and trusts the supplied CA certificates.
// The dial target determines the hostname/IP to verify during the handshake.
func ClientConfig(caPEM []byte) (*tls.Config, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("local CA file contains no usable certificate")
	}
	// Verification checks both the issuing CA and the target hostname/IP.
	// Do not set InsecureSkipVerify to work around a failed handshake.
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}, nil
}

// WriteDirectory creates a new owner-only directory beneath an existing parent.
// Existing directories fail; an I/O failure can leave partial files for inspection.
func (m Material) WriteDirectory(directory string) error {
	// Require a new directory. Existing certificates/keys are never overwritten.
	if err := os.Mkdir(directory, 0o700); err != nil {
		return err
	}
	for _, file := range []struct {
		name string
		data []byte
	}{{"ca.pem", m.CA}, {"server.pem", m.Certificate}, {"server-key.pem", m.Key}} {
		if err := os.WriteFile(filepath.Join(directory, file.name), file.data, 0o600); err != nil {
			return err // A failed setup may leave a partial directory to inspect.
		}
	}
	return nil
}

// LoadServer reads server.pem/server-key.pem for the standalone TLS server.
func LoadServer(directory string) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(filepath.Join(directory, "server.pem"), filepath.Join(directory, "server-key.pem"))
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}, nil
}

// LoadClient reads only ca.pem, so the standalone client needs no private key.
func LoadClient(directory string) (*tls.Config, error) {
	ca, err := os.ReadFile(filepath.Join(directory, "ca.pem"))
	if err != nil {
		return nil, err
	}
	return ClientConfig(ca)
}
