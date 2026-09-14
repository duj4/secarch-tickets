package oidcauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Config contains settings from the oidc section of app.json.
type Config struct {
	Enabled            bool          `json:"enabled"`
	IssuerURL          string        `json:"issuer_url"`
	ClientID           string        `json:"client_id"`
	ClientSecret       string        `json:"client_secret"`
	RedirectURL        string        `json:"redirect_url"`
	UsernameClaim      string        `json:"username_claim"`
	GroupsClaim        string        `json:"groups_claim"`
	AdminGroup         string        `json:"admin_group"`
	Scopes             []string      `json:"scopes"`
	SessionCookie      string        `json:"session_cookie"`
	SessionSecret      string        `json:"session_secret"`
	HTTPTimeoutSeconds int           `json:"http_timeout_seconds"`
	SessionTTL         time.Duration `json:"-"`
}

// ParseConfig defaults to requiring OIDC. An explicit enabled:false skips the
// remaining OIDC settings, including their field types and duration parsing.
func ParseConfig(data []byte) (Config, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		data = []byte("{}")
	}
	var switchConfig struct {
		Enabled *bool `json:"enabled"`
	}
	if data[0] != '{' || json.Unmarshal(data, &switchConfig) != nil {
		return Config{}, fmt.Errorf("oidc must be an object with a boolean enabled setting")
	}
	if switchConfig.Enabled != nil && !*switchConfig.Enabled {
		return Config{Enabled: false}, nil
	}

	fileConfig := struct {
		Config
		SessionTTL string `json:"session_ttl"`
	}{Config: Config{
		Enabled:            true,
		UsernameClaim:      defaultUsernameClaim,
		GroupsClaim:        defaultGroupsClaim,
		AdminGroup:         defaultAdminGroup,
		SessionCookie:      defaultSessionCookie,
		HTTPTimeoutSeconds: 30,
		SessionTTL:         8 * time.Hour,
	}}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fileConfig); err != nil {
		return Config{}, fmt.Errorf("invalid oidc configuration: %w", err)
	}
	cfg := fileConfig.Config
	cfg.IssuerURL = strings.TrimSpace(cfg.IssuerURL)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.RedirectURL = strings.TrimSpace(cfg.RedirectURL)
	cfg.UsernameClaim = strings.TrimSpace(cfg.UsernameClaim)
	cfg.GroupsClaim = strings.TrimSpace(cfg.GroupsClaim)
	cfg.AdminGroup = strings.TrimSpace(cfg.AdminGroup)
	cfg.SessionCookie = strings.TrimSpace(cfg.SessionCookie)
	cfg.Scopes = normalizedList(cfg.Scopes)
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}
	if !slices.Contains(cfg.Scopes, "openid") {
		cfg.Scopes = append([]string{"openid"}, cfg.Scopes...)
	}
	if fileConfig.SessionTTL != "" {
		ttl, err := time.ParseDuration(strings.TrimSpace(fileConfig.SessionTTL))
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("oidc.session_ttl must be a positive duration")
		}
		cfg.SessionTTL = ttl
	}
	missing := make([]string, 0, 5)
	for name, value := range map[string]string{
		"issuer_url": cfg.IssuerURL, "client_id": cfg.ClientID,
		"client_secret": cfg.ClientSecret, "redirect_url": cfg.RedirectURL,
		"session_secret": cfg.SessionSecret,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required oidc settings: %s", strings.Join(missing, ", "))
	}
	issuerURL, err := url.Parse(cfg.IssuerURL)
	if err != nil || issuerURL.Scheme != "https" || issuerURL.Host == "" || issuerURL.User != nil || issuerURL.RawQuery != "" || issuerURL.Fragment != "" {
		return Config{}, fmt.Errorf("oidc.issuer_url must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	redirectURL, err := url.Parse(cfg.RedirectURL)
	if err != nil || redirectURL.Scheme != "https" || redirectURL.Host == "" || redirectURL.User != nil || redirectURL.Path != "/auth/callback" || redirectURL.RawQuery != "" || redirectURL.Fragment != "" {
		return Config{}, fmt.Errorf("oidc.redirect_url must be an absolute HTTPS URL ending at /auth/callback")
	}
	if cfg.UsernameClaim == "" || cfg.GroupsClaim == "" || cfg.AdminGroup == "" || cfg.SessionCookie == "" {
		return Config{}, fmt.Errorf("oidc claim, admin group, and session cookie settings must not be empty")
	}
	if len(cfg.SessionSecret) < 32 {
		return Config{}, fmt.Errorf("oidc.session_secret must contain at least 32 bytes")
	}
	if cfg.HTTPTimeoutSeconds <= 0 {
		return Config{}, fmt.Errorf("oidc.http_timeout_seconds must be positive")
	}
	return cfg, nil
}
