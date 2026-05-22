package resources

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/zerotomvp/terraform-provider-ravendb-admin/internal/client"
	"software.sslmate.com/src/go-pkcs12"
)

func certificateSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := NewCertificateResource().(resource.ResourceWithImportState)
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestCertificateSchema_PasswordIsOptionalComputed(t *testing.T) {
	s := certificateSchema(t)
	attr, ok := s.Attributes["password"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("password attribute missing or wrong type")
	}
	if !attr.Optional {
		t.Error("password must be Optional so users can provide it")
	}
	if !attr.Computed {
		t.Error("password must be Computed so it survives reads (server does not return it)")
	}
	if !attr.Sensitive {
		t.Error("password must be Sensitive")
	}
}

func TestCertificateSchema_ExpireAfterDaysIsOptionalComputed(t *testing.T) {
	s := certificateSchema(t)
	attr, ok := s.Attributes["expire_after_days"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("expire_after_days attribute missing or wrong type")
	}
	if !attr.Optional {
		t.Error("expire_after_days must be Optional")
	}
	if !attr.Computed {
		t.Error("expire_after_days must be Computed so it survives reads")
	}
}

func TestParseImportID(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
		check   func(t *testing.T, p *importParams)
	}{
		{
			name: "bare-thumbprint",
			in:   "ABC123",
			check: func(t *testing.T, p *importParams) {
				if p.Thumbprint != "ABC123" {
					t.Errorf("thumbprint = %q", p.Thumbprint)
				}
				if p.HasPassword || p.HasExpire || p.PFXPath != "" || p.PEMPath != "" {
					t.Error("expected no optional fields set")
				}
			},
		},
		{
			name: "with-password",
			in:   "ABC123:password=secret",
			check: func(t *testing.T, p *importParams) {
				if !p.HasPassword || p.Password != "secret" {
					t.Errorf("password=%q hasPwd=%v", p.Password, p.HasPassword)
				}
			},
		},
		{
			name: "with-expire",
			in:   "ABC123:expire_after_days=30",
			check: func(t *testing.T, p *importParams) {
				if !p.HasExpire || p.ExpireAfterDays != 30 {
					t.Errorf("expire=%d hasExpire=%v", p.ExpireAfterDays, p.HasExpire)
				}
			},
		},
		{
			name: "with-pfx-and-pem-paths",
			in:   "ABC123:pfx=/tmp/cert.pfx:pem=/tmp/cert.pem",
			check: func(t *testing.T, p *importParams) {
				if p.PFXPath != "/tmp/cert.pfx" {
					t.Errorf("pfx path = %q", p.PFXPath)
				}
				if p.PEMPath != "/tmp/cert.pem" {
					t.Errorf("pem path = %q", p.PEMPath)
				}
			},
		},
		{
			name: "url-encoded-password",
			in:   "ABC123:password=p%40ss%3Aword",
			check: func(t *testing.T, p *importParams) {
				if p.Password != "p@ss:word" {
					t.Errorf("password = %q, want p@ss:word", p.Password)
				}
			},
		},
		{
			name: "all-fields",
			in:   "ABC123:password=s:expire_after_days=90:pfx=/x.pfx:pem=/x.pem",
			check: func(t *testing.T, p *importParams) {
				if p.Thumbprint != "ABC123" || p.Password != "s" || p.ExpireAfterDays != 90 || p.PFXPath != "/x.pfx" || p.PEMPath != "/x.pem" {
					t.Errorf("got %+v", p)
				}
			},
		},
		{name: "empty", in: "", wantErr: true},
		{name: "missing-thumbprint", in: ":password=foo", wantErr: true},
		{name: "missing-equals", in: "ABC123:password", wantErr: true},
		{name: "unknown-key", in: "ABC123:unknown=foo", wantErr: true},
		{name: "bad-int", in: "ABC123:expire_after_days=not-a-number", wantErr: true},
		{name: "bad-url-encoding", in: "ABC123:password=%ZZ", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := parseImportID(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (params=%+v)", p)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.check != nil {
				c.check(t, p)
			}
		})
	}
}

// generateTestCertPFX produces a self-signed cert + RSA key encoded as PFX,
// returning the PFX bytes and the cert's SHA1 thumbprint.
func generateTestCertPFX(t *testing.T, password string) (pfxData []byte, thumbprint string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "import-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(48 * time.Hour),
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
	pfxData, err = pkcs12.Modern.Encode(key, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode: %v", err)
	}
	return pfxData, client.CalculateThumbprint(cert)
}

func TestExtractFromPFX_Success(t *testing.T) {
	pfxData, thumbprint := generateTestCertPFX(t, "pw")

	m, err := extractFromPFX(pfxData, "pw", thumbprint)
	if err != nil {
		t.Fatalf("extractFromPFX: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(m.PFXBase64); err != nil {
		t.Errorf("pfx_base64 invalid: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(m.PEMBase64); err != nil {
		t.Errorf("pem_base64 invalid: %v", err)
	}
	if !strings.HasSuffix(m.NotAfter, "Z") {
		t.Errorf("not_after should be RFC3339 UTC, got %q", m.NotAfter)
	}
}

func TestExtractFromPFX_ThumbprintMismatch(t *testing.T) {
	pfxData, _ := generateTestCertPFX(t, "pw")

	_, err := extractFromPFX(pfxData, "pw", "0000000000000000000000000000000000000000")
	if err == nil || !strings.Contains(err.Error(), "thumbprint mismatch") {
		t.Fatalf("expected thumbprint mismatch error, got %v", err)
	}
}

func TestExtractFromPFX_LowercaseThumbprintAccepted(t *testing.T) {
	pfxData, thumbprint := generateTestCertPFX(t, "pw")

	if _, err := extractFromPFX(pfxData, "pw", strings.ToLower(thumbprint)); err != nil {
		t.Fatalf("lowercase thumbprint should match: %v", err)
	}
}

func TestExtractFromPEM_Success(t *testing.T) {
	pfxData, thumbprint := generateTestCertPFX(t, "pw")
	pemData, err := client.PFXToPEM(pfxData, "pw")
	if err != nil {
		t.Fatalf("PFXToPEM: %v", err)
	}

	m, err := extractFromPEM(pemData, thumbprint)
	if err != nil {
		t.Fatalf("extractFromPEM: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(m.PEMBase64); err != nil {
		t.Errorf("pem_base64 invalid: %v", err)
	}
	if m.PFXBase64 != "" {
		t.Error("PFX should not be set when only PEM was provided")
	}
}

func TestExtractFromPEM_ThumbprintMismatch(t *testing.T) {
	pfxData, _ := generateTestCertPFX(t, "pw")
	pemData, err := client.PFXToPEM(pfxData, "pw")
	if err != nil {
		t.Fatalf("PFXToPEM: %v", err)
	}

	_, err = extractFromPEM(pemData, "0000000000000000000000000000000000000000")
	if err == nil || !strings.Contains(err.Error(), "thumbprint mismatch") {
		t.Fatalf("expected thumbprint mismatch error, got %v", err)
	}
}
