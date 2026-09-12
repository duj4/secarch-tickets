package httpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

type ProxyFunc func(*http.Request) (*url.URL, error)

// ProxyConfig supplies proxy settings directly, without changing process variables.
type ProxyConfig struct {
	HTTPProxy  string `json:"http_proxy"`
	HTTPSProxy string `json:"https_proxy"`
	NoProxy    string `json:"no_proxy"`
}

// Resolver uses Go's standard proxy matching rules with file-supplied values.
func (cfg ProxyConfig) Resolver() (ProxyFunc, error) {
	for name, value := range map[string]string{"http_proxy": cfg.HTTPProxy, "https_proxy": cfg.HTTPSProxy} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !strings.Contains(value, "://") {
			value = "http://" + value
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Hostname() == "" {
			return nil, fmt.Errorf("proxy.%s must be a valid proxy URL", name)
		}
		switch parsed.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("proxy.%s must use http, https, socks5, or socks5h", name)
		}
	}
	resolver := (&httpproxy.Config{
		HTTPProxy:  strings.TrimSpace(cfg.HTTPProxy),
		HTTPSProxy: strings.TrimSpace(cfg.HTTPSProxy),
		NoProxy:    strings.TrimSpace(cfg.NoProxy),
	}).ProxyFunc()
	return func(request *http.Request) (*url.URL, error) {
		return resolver(request.URL)
	}, nil
}
