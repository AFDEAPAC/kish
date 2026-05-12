package s3

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// TLSOptions controls certificate trust for HTTPS S3-compatible endpoints.
type TLSOptions struct {
	// CAFile points to an additional PEM bundle appended to the system trust store.
	CAFile string

	// InsecureSkipVerify disables S3 TLS verification. Use only for lab debugging.
	InsecureSkipVerify bool
}

// NewHTTPClient builds the HTTP client used by the AWS SDK S3 client.
// It starts from the system trust store and optionally appends a custom CA file.
func NewHTTPClient(opts TLSOptions) (*http.Client, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		certPool = x509.NewCertPool()
	}

	if opts.CAFile != "" {
		pemBytes, err := os.ReadFile(opts.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read s3 tls ca_file %q: %w", opts.CAFile, err)
		}
		if ok := certPool.AppendCertsFromPEM(pemBytes); !ok {
			return nil, fmt.Errorf("append s3 tls ca_file %q: no valid PEM certificates found", opts.CAFile)
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		RootCAs: certPool,
		// This is an explicit lab/debug escape hatch only. Production should use CAFile.
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}, nil
}
