package cmdb

import (
	"net/http"
)

// Config holds settings for the CMDB client.
type Config struct {
	TicketAPIURL                    string `json:"ticket_api_url"`
	TicketBrowseURL                 string `json:"ticket_browse_url"`
	ObjectsAPIURL                   string `json:"objects_api_url"`
	PageSize                        int    `json:"page_size"`
	ObjectBatchSize                 int    `json:"object_batch_size"`
	ObjectMaxConcurrency            int    `json:"object_max_concurrency"`
	HTTPTimeoutSeconds              int    `json:"http_timeout_seconds"`
	RefreshSuccessCooldownSeconds   int    `json:"refresh_success_cooldown_seconds"`
	RefreshFailureBackoffSeconds    int    `json:"refresh_failure_backoff_seconds"`
	RefreshFailureBackoffMultiplier int    `json:"refresh_failure_backoff_multiplier"`
	RefreshCircuitBreakerThreshold  int    `json:"refresh_circuit_breaker_threshold"`
	RefreshCircuitOpenSeconds       int    `json:"refresh_circuit_open_seconds"`
	CACertPath                      string `json:"-"`
	ClientCertPath                  string `json:"-"`
	ClientKeyPath                   string `json:"-"`
}

// Client calls the CMDB API over the configured HTTP transport.
type Client struct {
	httpClient *http.Client
	cfg        Config
}
