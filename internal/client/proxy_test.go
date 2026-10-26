package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// recordingProxy answers every request itself and remembers what it was asked,
// so a test can tell whether the client went through it.
type recordingProxy struct {
	mu       sync.Mutex
	requests []string
}

func (p *recordingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	if r.Method == http.MethodConnect {
		p.requests = append(p.requests, "CONNECT "+r.Host)
	} else {
		p.requests = append(p.requests, r.Method+" "+r.URL.String())
	}
	p.mu.Unlock()

	if r.Method == http.MethodConnect {
		// Refuse the tunnel: the test only needs to see that it was asked for.
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (p *recordingProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.requests...)
}

func startProxy(t *testing.T) (*recordingProxy, string) {
	t.Helper()
	proxy := &recordingProxy{}
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	return proxy, server.URL
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(name, "")
	}
}

// A sandbox that has no network of its own (Claude Code's, for one) lets a
// process out only through the proxy it names in HTTPS_PROXY. The client must
// use it, and must leave name resolution to the proxy.
func TestClientUsesHTTPSProxyFromEnvironment(t *testing.T) {
	clearProxyEnv(t)
	proxy, proxyURL := startProxy(t)
	t.Setenv("HTTPS_PROXY", proxyURL)

	c, err := NewClient(ClientConfig{URL: "https://ravendb.example.invalid:7443"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.Ping(); err == nil {
		t.Fatal("Ping succeeded through a proxy that refuses tunnels")
	}

	got := proxy.seen()
	if len(got) != 1 || got[0] != "CONNECT ravendb.example.invalid:7443" {
		t.Fatalf("proxy saw %q, want one CONNECT ravendb.example.invalid:7443", got)
	}
}

func TestClientUsesHTTPProxyFromEnvironment(t *testing.T) {
	clearProxyEnv(t)
	proxy, proxyURL := startProxy(t)
	t.Setenv("HTTP_PROXY", proxyURL)

	c, err := NewClient(ClientConfig{URL: "http://ravendb.example.invalid:8080"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.Ping(); err != nil {
		t.Fatalf("Ping through proxy: %v", err)
	}

	got := proxy.seen()
	if len(got) != 1 || got[0] != "GET http://ravendb.example.invalid:8080/build/version" {
		t.Fatalf("proxy saw %q, want one GET for /build/version", got)
	}
}

func TestClientHonoursNoProxy(t *testing.T) {
	clearProxyEnv(t)
	proxy, proxyURL := startProxy(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	t.Setenv("HTTP_PROXY", proxyURL)
	t.Setenv("NO_PROXY", "127.0.0.1")

	c, err := NewClient(ClientConfig{URL: target.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Ping(); err != nil {
		t.Fatalf("Ping direct: %v", err)
	}
	if got := proxy.seen(); len(got) != 0 {
		t.Fatalf("proxy saw %q, want nothing for a NO_PROXY host", got)
	}
}
