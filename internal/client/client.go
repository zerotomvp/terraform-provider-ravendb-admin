package client

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// Client is a RavenDB HTTP client
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// ClientConfig holds configuration for creating a new client
type ClientConfig struct {
	URL                 string
	CertificatePath     string
	CertificatePassword string
	InsecureSkipVerify  bool
}

// NewClient creates a new RavenDB client
func NewClient(config ClientConfig) (*Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkipVerify,
	}

	// Load client certificate if provided
	if config.CertificatePath != "" {
		certData, err := os.ReadFile(config.CertificatePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}

		cert, err := tls.X509KeyPair(certData, certData)
		if err != nil {
			// Try loading as PFX
			cert, err = loadPFXCertificate(certData, config.CertificatePassword)
			if err != nil {
				return nil, fmt.Errorf("failed to load certificate: %w", err)
			}
		}

		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &Client{
		BaseURL: strings.TrimSuffix(config.URL, "/"),
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}

	return client, nil
}

// loadPFXCertificate loads a PFX/PKCS#12 bundle into a tls.Certificate.
func loadPFXCertificate(data []byte, password string) (tls.Certificate, error) {
	privateKey, cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to decode PFX: %w", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  privateKey,
		Leaf:        cert,
	}, nil
}

// Database represents a RavenDB database
type Database struct {
	Name              string            `json:"Name,omitempty"`
	DatabaseName      string            `json:"DatabaseName,omitempty"`
	Disabled          bool              `json:"Disabled"`
	Encrypted         bool              `json:"Encrypted"`
	Settings          map[string]string `json:"Settings,omitempty"`
	ReplicationFactor int               `json:"ReplicationFactor,omitempty"`
	Topology          *DatabaseTopology `json:"Topology,omitempty"`
}

// DatabaseTopology represents the topology of a database
type DatabaseTopology struct {
	Members                  []string `json:"Members,omitempty"`
	DynamicNodesDistribution bool     `json:"DynamicNodesDistribution,omitempty"`
}

// DatabaseInfo represents database info from the list endpoint
type DatabaseInfo struct {
	Name           string `json:"Name"`
	Disabled       bool   `json:"Disabled"`
	IsEncrypted    bool   `json:"IsEncrypted"`
	DocumentsCount int64  `json:"DocumentsCount"`
	IndexesCount   int    `json:"IndexesCount"`
	LockMode       string `json:"LockMode"`
}

// DatabasesResponse is the response from GET /databases
type DatabasesResponse struct {
	Databases []DatabaseInfo `json:"Databases"`
}

// DatabaseRecord represents the full database record
type DatabaseRecord struct {
	DatabaseName string            `json:"DatabaseName"`
	Disabled     bool              `json:"Disabled"`
	Encrypted    bool              `json:"Encrypted"`
	Settings     map[string]string `json:"Settings"`
	Topology     *DatabaseTopology `json:"Topology,omitempty"`
	Etag         int64             `json:"Etag,omitempty"`
}

// DatabaseRecordResponse wraps the database record response
type DatabaseRecordResponse struct {
	DatabaseRecord DatabaseRecord `json:"DatabaseRecord"`
	Etag           int64          `json:"Etag"`
}

// CreateDatabaseRequest is the request body for creating a database
type CreateDatabaseRequest struct {
	DatabaseName string            `json:"DatabaseName"`
	Settings     map[string]string `json:"Settings,omitempty"`
	Disabled     bool              `json:"Disabled"`
	Encrypted    bool              `json:"Encrypted"`
	Topology     *DatabaseTopology `json:"Topology,omitempty"`
}

// DeleteDatabaseRequest is the request body for deleting a database
type DeleteDatabaseRequest struct {
	HardDelete    bool     `json:"HardDelete"`
	DatabaseNames []string `json:"DatabaseNames"`
	FromNodes     []string `json:"FromNodes,omitempty"`
}

// Certificate represents a RavenDB certificate
type Certificate struct {
	Name              string            `json:"Name"`
	Certificate       string            `json:"Certificate,omitempty"`
	Password          string            `json:"Password,omitempty"`
	Thumbprint        string            `json:"Thumbprint,omitempty"`
	NotAfter          string            `json:"NotAfter,omitempty"`
	NotBefore         string            `json:"NotBefore,omitempty"`
	SecurityClearance string            `json:"SecurityClearance"`
	Permissions       map[string]string `json:"Permissions,omitempty"`
}

// GenerateCertificateRequest is the request for generating a certificate
type GenerateCertificateRequest struct {
	Name                       string            `json:"Name"`
	Password                   string            `json:"Password,omitempty"`
	SecurityClearance          string            `json:"SecurityClearance"`
	NotAfter                   string            `json:"NotAfter,omitempty"`
	Permissions                map[string]string `json:"Permissions,omitempty"`
	TwoFactorAuthenticationKey string            `json:"TwoFactorAuthenticationKey,omitempty"`
}

// GeneratedCertificate holds the result of certificate generation
type GeneratedCertificate struct {
	PFX        []byte
	PEM        []byte
	Thumbprint string
	NotAfter   string
	Name       string
}

// CertificatesResponse is the response from GET /admin/certificates
type CertificatesResponse struct {
	Results          []Certificate `json:"Results"` // API returns "Results", not "Certificates"
	LoadedServerCert string        `json:"LoadedServerCert"`
}

// UpdateCertificateRequest is the request for updating a certificate
type UpdateCertificateRequest struct {
	Name              string            `json:"Name"`
	Thumbprint        string            `json:"Thumbprint"`
	SecurityClearance string            `json:"SecurityClearance"`
	Permissions       map[string]string `json:"Permissions,omitempty"`
}

// OperationIDResponse is the response for getting next operation ID
type OperationIDResponse struct {
	ID int64 `json:"Id"`
}

// doRequest performs an HTTP request and returns the response
func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// CreateDatabase creates a new database
func (c *Client) CreateDatabase(name string, replicationFactor int, disabled, encrypted bool, settings map[string]string) error {
	reqBody := CreateDatabaseRequest{
		DatabaseName: name,
		Settings:     settings,
		Disabled:     disabled,
		Encrypted:    encrypted,
	}

	params := url.Values{}
	params.Set("name", name)
	params.Set("replicationFactor", fmt.Sprintf("%d", replicationFactor))

	path := "/admin/databases?" + params.Encode()

	resp, err := c.doRequest("PUT", path, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create database: %s - %s", resp.Status, string(body))
	}

	return nil
}

// GetDatabase retrieves a database by name
func (c *Client) GetDatabase(name string) (*DatabaseRecord, error) {
	params := url.Values{}
	params.Set("name", name)

	path := "/admin/databases?" + params.Encode()

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get database: %s - %s", resp.Status, string(body))
	}

	// API returns fields directly at root level (not wrapped in DatabaseRecord)
	var result DatabaseRecord
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ListDatabases returns all databases
func (c *Client) ListDatabases() ([]DatabaseInfo, error) {
	resp, err := c.doRequest("GET", "/databases", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list databases: %s - %s", resp.Status, string(body))
	}

	var result DatabasesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Databases, nil
}

// DeleteDatabase deletes a database
func (c *Client) DeleteDatabase(name string, hardDelete bool) error {
	reqBody := DeleteDatabaseRequest{
		HardDelete:    hardDelete,
		DatabaseNames: []string{name},
	}

	resp, err := c.doRequest("DELETE", "/admin/databases", reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete database: %s - %s", resp.Status, string(body))
	}

	return nil
}

// ToggleDatabase enables or disables a database
func (c *Client) ToggleDatabase(name string, disable bool) error {
	reqBody := map[string][]string{
		"DatabaseNames": {name},
	}

	endpoint := "/admin/databases/enable"
	if disable {
		endpoint = "/admin/databases/disable"
	}

	resp, err := c.doRequest("POST", endpoint, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to toggle database: %s - %s", resp.Status, string(body))
	}

	return nil
}

// GetNextOperationID gets the next operation ID for async operations
func (c *Client) GetNextOperationID() (int64, error) {
	resp, err := c.doRequest("GET", "/admin/operations/next-operation-id", nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to get operation ID: %s - %s", resp.Status, string(body))
	}

	var result OperationIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.ID, nil
}

// GenerateCertificate generates a new client certificate
func (c *Client) GenerateCertificate(req GenerateCertificateRequest) (*GeneratedCertificate, error) {
	// Get operation ID
	operationID, err := c.GetNextOperationID()
	if err != nil {
		return nil, fmt.Errorf("failed to get operation ID: %w", err)
	}

	// Prepare the request body as form data
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	formData := url.Values{}
	formData.Set("Options", string(jsonBody))

	path := fmt.Sprintf("/admin/certificates?operationId=%d", operationID)

	httpReq, err := http.NewRequest("POST", c.BaseURL+path, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to generate certificate: %s - %s", resp.Status, string(body))
	}

	// Read the ZIP response
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Extract PFX and PEM from ZIP
	result := &GeneratedCertificate{
		Name: req.Name,
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to read ZIP: %w", err)
	}

	var crtData, keyData []byte

	for _, file := range zipReader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file in ZIP: %w", err)
		}

		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read file in ZIP: %w", err)
		}

		if strings.HasSuffix(file.Name, ".pfx") {
			result.PFX = data
			// Extract thumbprint and NotAfter from PFX
			certInfo, err := ExtractCertInfoFromPFX(data, req.Password)
			if err == nil {
				result.Thumbprint = certInfo.Thumbprint
				result.NotAfter = certInfo.NotAfter.UTC().Format(time.RFC3339)
			}
		} else if strings.HasSuffix(file.Name, ".pem") {
			result.PEM = data
		} else if strings.HasSuffix(file.Name, ".crt") {
			crtData = data
		} else if strings.HasSuffix(file.Name, ".key") {
			keyData = data
		}
	}

	// Build PEM from .crt + .key if no .pem file was in the ZIP
	if len(result.PEM) == 0 && len(crtData) > 0 && len(keyData) > 0 {
		// Ensure newline between cert and key
		combined := append([]byte{}, bytes.TrimRight(crtData, "\r\n")...)
		combined = append(combined, '\n')
		combined = append(combined, bytes.TrimRight(keyData, "\r\n")...)
		combined = append(combined, '\n')
		result.PEM = combined
	}

	// Last resort: convert PFX to PEM
	if len(result.PEM) == 0 && len(result.PFX) > 0 {
		pemData, err := PFXToPEM(result.PFX, req.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to convert PFX to PEM: %w", err)
		}
		result.PEM = pemData
	}

	return result, nil
}

// PFXToPEM converts a PFX/PKCS#12 bundle to a combined PEM (cert + key).
func PFXToPEM(pfxData []byte, password string) ([]byte, error) {
	privateKey, cert, err := pkcs12.Decode(pfxData, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PFX: %w", err)
	}

	var buf bytes.Buffer

	// Encode certificate
	if err := pem.Encode(&buf, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}); err != nil {
		return nil, fmt.Errorf("failed to encode certificate PEM: %w", err)
	}

	// Encode private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	if err := pem.Encode(&buf, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	}); err != nil {
		return nil, fmt.Errorf("failed to encode private key PEM: %w", err)
	}

	return buf.Bytes(), nil
}

// CertificateInfo holds extracted certificate information
type CertificateInfo struct {
	Thumbprint string
	NotAfter   time.Time
}

// ExtractCertInfoFromPFX extracts the thumbprint and expiry from a PFX
// certificate.
func ExtractCertInfoFromPFX(data []byte, password string) (*CertificateInfo, error) {
	_, cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PFX: %w", err)
	}

	return &CertificateInfo{
		Thumbprint: CalculateThumbprint(cert),
		NotAfter:   cert.NotAfter,
	}, nil
}

// ParseCertFromPEM extracts the first CERTIFICATE block from a PEM bundle
// (which may also contain a PRIVATE KEY block) and parses it as an x509
// certificate.
func ParseCertFromPEM(pemData []byte) (*x509.Certificate, error) {
	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, fmt.Errorf("no CERTIFICATE block found in PEM data")
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			return cert, nil
		}
	}
}

// CalculateThumbprint calculates the SHA1 thumbprint of a certificate,
// formatted as an uppercase hex string (matching the RavenDB convention).
func CalculateThumbprint(cert *x509.Certificate) string {
	hash := sha1.Sum(cert.Raw)
	return fmt.Sprintf("%X", hash)
}

// NormalizeNotAfter parses a NotAfter timestamp in any of the formats RavenDB
// or the provider may emit and returns it in canonical RFC3339. RavenDB serves
// "2006-01-02T15:04:05.0000000" (no timezone, treated as UTC by the server).
// Returns the input unchanged if it cannot be parsed.
func NormalizeNotAfter(s string) string {
	if s == "" {
		return s
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

// UploadCertificate uploads an existing certificate
func (c *Client) UploadCertificate(cert Certificate) error {
	resp, err := c.doRequest("PUT", "/admin/certificates", cert)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload certificate: %s - %s", resp.Status, string(body))
	}

	return nil
}

// GetCertificate retrieves a certificate by thumbprint
func (c *Client) GetCertificate(thumbprint string) (*Certificate, error) {
	params := url.Values{}
	params.Set("thumbprint", thumbprint)

	path := "/admin/certificates?" + params.Encode()

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get certificate: %s - %s", resp.Status, string(body))
	}

	var result CertificatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, nil
	}

	cert := result.Results[0]
	cert.NotAfter = NormalizeNotAfter(cert.NotAfter)
	cert.NotBefore = NormalizeNotAfter(cert.NotBefore)
	return &cert, nil
}

// ListCertificates returns all certificates
func (c *Client) ListCertificates() ([]Certificate, error) {
	resp, err := c.doRequest("GET", "/admin/certificates", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list certificates: %s - %s", resp.Status, string(body))
	}

	var result CertificatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	for i := range result.Results {
		result.Results[i].NotAfter = NormalizeNotAfter(result.Results[i].NotAfter)
		result.Results[i].NotBefore = NormalizeNotAfter(result.Results[i].NotBefore)
	}
	return result.Results, nil
}

// UpdateCertificate updates a certificate's metadata
func (c *Client) UpdateCertificate(req UpdateCertificateRequest) error {
	resp, err := c.doRequest("POST", "/admin/certificates/edit", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update certificate: %s - %s", resp.Status, string(body))
	}

	return nil
}

// DeleteCertificate deletes a certificate by thumbprint
func (c *Client) DeleteCertificate(thumbprint string) error {
	params := url.Values{}
	params.Set("thumbprint", thumbprint)

	path := "/admin/certificates?" + params.Encode()

	resp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete certificate: %s - %s", resp.Status, string(body))
	}

	return nil
}

// Ping checks connectivity to the RavenDB server
func (c *Client) Ping() error {
	resp, err := c.doRequest("GET", "/build/version", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %s", resp.Status)
	}

	return nil
}
