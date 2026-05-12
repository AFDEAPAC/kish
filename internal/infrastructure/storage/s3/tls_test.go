package s3

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHTTPClientWithoutCustomCA(t *testing.T) {
	client, err := NewHTTPClient(TLSOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.Transport == nil {
		t.Fatal("expected transport")
	}
}

func TestNewHTTPClientRejectsMissingCAFile(t *testing.T) {
	_, err := NewHTTPClient(TLSOptions{CAFile: "/missing/ca.crt"})
	if err == nil {
		t.Fatal("expected missing ca_file error")
	}
}

func TestNewHTTPClientRejectsInvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.crt")
	if err := os.WriteFile(path, []byte("not a pem certificate"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewHTTPClient(TLSOptions{CAFile: path})
	if err == nil {
		t.Fatal("expected invalid pem error")
	}
}

func TestNewHTTPClientTrustsCustomCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	defaultClient, err := NewHTTPClient(TLSOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := defaultClient.Get(srv.URL); err == nil {
		t.Fatal("expected self-signed server to fail without custom CA")
	}

	caFile := writeServerCertPEM(t, srv)
	client, err := NewHTTPClient(TLSOptions{CAFile: caFile})
	if err != nil {
		t.Fatalf("unexpected custom CA error: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected custom CA to trust server, got %v", err)
	}
	resp.Body.Close()
}

func TestNewHTTPClientAllowsInsecureSkipVerify(t *testing.T) {
	client, err := NewHTTPClient(TLSOptions{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=true")
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected insecure client to trust self-signed server, got %v", err)
	}
	resp.Body.Close()
}

func writeServerCertPEM(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	cert := srv.Certificate()
	if _, err := x509.ParseCertificate(cert.Raw); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server-ca.crt")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(path, pemBytes, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
