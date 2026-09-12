package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// NewClient uses an explicit proxy policy; nil means a direct connection.
func NewClient(timeout time.Duration, proxy ProxyFunc) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxy
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// NewTLSClient creates an HTTP client configured for mutual TLS.
func NewTLSClient(caCertPath, clientCertPath, clientKeyPath string, timeout time.Duration, proxy ProxyFunc) (*http.Client, error) {
	if caCertPath == "" || clientCertPath == "" || clientKeyPath == "" {
		return nil, fmt.Errorf("TLS requires CA, client certificate, and client key paths")
	}

	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("append CA certificate")
	}

	cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client certificate and key: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy:               proxy,
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 10,
		},
		Timeout: timeout,
	}, nil
}
