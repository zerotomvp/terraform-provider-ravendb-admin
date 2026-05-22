package client

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

func TestNormalizeNotAfter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ravendb-format-7-digit-fractional", "2026-05-05T07:41:19.0000000", "2026-05-05T07:41:19Z"},
		{"ravendb-format-with-microseconds", "2026-05-05T07:41:19.1234567", "2026-05-05T07:41:19Z"},
		{"rfc3339-zulu", "2026-05-05T07:41:19Z", "2026-05-05T07:41:19Z"},
		{"rfc3339-with-offset", "2026-05-05T09:41:19+02:00", "2026-05-05T07:41:19Z"},
		{"rfc3339-nano", "2026-05-05T07:41:19.000000000Z", "2026-05-05T07:41:19Z"},
		{"no-fractional-no-tz", "2026-05-05T07:41:19", "2026-05-05T07:41:19Z"},
		{"unparseable-passthrough", "not-a-timestamp", "not-a-timestamp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeNotAfter(c.in)
			if got != c.want {
				t.Errorf("NormalizeNotAfter(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func generateTestPFX(t *testing.T, password string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	pfxData, err := pkcs12.Modern.Encode(key, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode: %v", err)
	}
	return pfxData
}

func TestLoadPFXCertificate(t *testing.T) {
	pfxData := generateTestPFX(t, "secret")

	cert, err := loadPFXCertificate(pfxData, "secret")
	if err != nil {
		t.Fatalf("loadPFXCertificate: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected at least one certificate in chain")
	}
	if cert.PrivateKey == nil {
		t.Fatal("expected private key to be populated")
	}
	if cert.Leaf == nil {
		t.Fatal("expected leaf to be populated")
	}
	if cert.Leaf.Subject.CommonName != "test" {
		t.Errorf("unexpected CN: %q", cert.Leaf.Subject.CommonName)
	}
}

func TestLoadPFXCertificateWrongPassword(t *testing.T) {
	pfxData := generateTestPFX(t, "right")

	_, err := loadPFXCertificate(pfxData, "wrong")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}
