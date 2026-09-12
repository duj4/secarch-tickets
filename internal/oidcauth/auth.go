package oidcauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

const (
	defaultUsernameClaim     = "samaccountname"
	defaultGroupsClaim       = "roles"
	defaultAdminGroup        = "SecArchTicketsAdmin"
	defaultSessionCookie     = "secarch_oidc_session"
	loginFlowTTL             = 10 * time.Minute
	providerDiscoveryTimeout = 30 * time.Second
)

// Principal contains the authenticated identity and ticket access decision.
// With OIDC disabled, only IsAdmin is set to grant unrestricted ticket access.
type Principal struct {
	Username string
	Groups   []string
	IsAdmin  bool
}

type loginFlow struct {
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"code_verifier"`
	ReturnTo     string `json:"return_to"`
	ExpiresAt    int64  `json:"expires_at"`
}

type session struct {
	Username  string `json:"username"`
	IsAdmin   bool   `json:"is_admin"`
	ExpiresAt int64  `json:"expires_at"`
}

// Manager owns the OIDC authorization-code flow and authenticated browser
// sessions. Cookies contain only encrypted, verified identity data; tokens are
// not sent to the browser or retained after the callback.
type Manager struct {
	config       Config
	oauthConfig  oauth2.Config
	verifier     *oidc.IDTokenVerifier
	httpClient   *http.Client
	cookieCipher cipher.AEAD
	now          func() time.Time
}

// New constructs the manager, discovering the provider only when OIDC is enabled.
func New(ctx context.Context, cfg Config, client *http.Client) (*Manager, error) {
	if !cfg.Enabled {
		return &Manager{config: cfg}, nil
	}
	if client == nil {
		return nil, fmt.Errorf("OIDC HTTP client is required")
	}

	cookieCipher, err := newCookieCipher(cfg.SessionSecret)
	if err != nil {
		return nil, err
	}
	discoveryCtx, cancel := context.WithTimeout(oidc.ClientContext(ctx, client), providerDiscoveryTimeout)
	defer cancel()
	provider, err := oidc.NewProvider(discoveryCtx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}

	return &Manager{
		config:     cfg,
		httpClient: client,
		oauthConfig: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       cfg.Scopes,
		},
		verifier:     provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		cookieCipher: cookieCipher,
		now:          time.Now,
	}, nil
}

// Enabled reports whether requests require OIDC authentication.
func (m *Manager) Enabled() bool {
	return m.config.Enabled
}

// RegisterRoutes adds the public endpoints needed for OIDC login and logout.
func (m *Manager) RegisterRoutes(r gin.IRoutes) {
	if !m.Enabled() {
		return
	}
	r.GET("/auth/login", m.login)
	r.GET("/auth/callback", m.callback)
	r.GET("/auth/logout", m.logout)
}

// RequireAuth verifies the opaque session and stores its principal in Gin's
// request context. Browser navigation is redirected to login; API calls get a
// JSON 401 response. Explicitly disabling OIDC grants unrestricted ticket access.
func (m *Manager) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !m.Enabled() {
			c.Set(principalContextKey, Principal{IsAdmin: true})
			c.Header("Cache-Control", "no-store")
			c.Next()
			return
		}
		principal, ok := m.requestPrincipal(c)
		if !ok {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
				return
			}
			returnTo := c.Request.URL.RequestURI()
			c.Redirect(http.StatusFound, "/auth/login?return_to="+url.QueryEscape(returnTo))
			c.Abort()
			return
		}

		c.Set(principalContextKey, principal)
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

const principalContextKey = "oidc.principal"

// PrincipalFromContext returns the identity established by RequireAuth.
func PrincipalFromContext(c *gin.Context) (Principal, bool) {
	value, exists := c.Get(principalContextKey)
	if !exists {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok
}

func (m *Manager) login(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	state, err := randomString(32)
	if err != nil {
		c.String(http.StatusInternalServerError, "could not start login")
		return
	}
	nonce, err := randomString(32)
	if err != nil {
		c.String(http.StatusInternalServerError, "could not start login")
		return
	}
	codeVerifier := oauth2.GenerateVerifier()
	now := m.now()
	flow := loginFlow{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		ReturnTo:     safeReturnTo(c.Query("return_to")),
		ExpiresAt:    now.Add(loginFlowTTL).Unix(),
	}
	encodedFlow, err := m.encodeCookie(m.stateCookieName(), flow)
	if err != nil {
		c.String(http.StatusInternalServerError, "could not start login")
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     m.stateCookieName(),
		Value:    encodedFlow,
		Path:     "/",
		MaxAge:   int(loginFlowTTL.Seconds()),
		Expires:  now.Add(loginFlowTTL),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	c.Redirect(http.StatusFound, m.oauthConfig.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(codeVerifier),
	))
}

func (m *Manager) callback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		c.String(http.StatusUnauthorized, "OIDC login failed: %s", providerError)
		return
	}

	state := strings.TrimSpace(c.Query("state"))
	stateCookie, err := c.Request.Cookie(m.stateCookieName())
	var flow loginFlow
	if err != nil || m.decodeCookie(m.stateCookieName(), stateCookie.Value, &flow) != nil || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(flow.State)) != 1 {
		c.String(http.StatusBadRequest, "invalid OIDC state")
		return
	}
	m.clearCookie(c, m.stateCookieName(), "/")
	if !time.Unix(flow.ExpiresAt, 0).After(m.now()) {
		c.String(http.StatusBadRequest, "expired OIDC login")
		return
	}

	oauthToken, err := m.oauthConfig.Exchange(oidc.ClientContext(c.Request.Context(), m.httpClient), c.Query("code"), oauth2.VerifierOption(flow.CodeVerifier))
	if err != nil {
		c.String(http.StatusUnauthorized, "OIDC code exchange failed")
		return
	}
	rawIDToken, ok := oauthToken.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		c.String(http.StatusUnauthorized, "OIDC response did not include an ID token")
		return
	}
	idToken, err := m.verifier.Verify(oidc.ClientContext(c.Request.Context(), m.httpClient), rawIDToken)
	if err != nil {
		c.String(http.StatusUnauthorized, "OIDC ID token verification failed")
		return
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(flow.Nonce)) != 1 {
		c.String(http.StatusUnauthorized, "OIDC nonce verification failed")
		return
	}

	principal, err := principalFromIDToken(idToken, m.config)
	if err != nil {
		c.String(http.StatusUnauthorized, "OIDC identity is incomplete")
		return
	}
	now := m.now()
	expiresAt := now.Add(m.config.SessionTTL)
	if !idToken.Expiry.IsZero() && idToken.Expiry.Before(expiresAt) {
		expiresAt = idToken.Expiry
	}
	if !expiresAt.After(now) {
		c.String(http.StatusUnauthorized, "OIDC ID token is expired")
		return
	}

	encodedSession, err := m.encodeCookie(m.config.SessionCookie, session{
		Username:  principal.Username,
		IsAdmin:   principal.IsAdmin,
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     m.config.SessionCookie,
		Value:    encodedSession,
		Path:     "/",
		MaxAge:   maxAge(now, expiresAt),
		Expires:  expiresAt,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	c.Redirect(http.StatusFound, flow.ReturnTo)
}

func (m *Manager) logout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	m.clearCookie(c, m.config.SessionCookie, "/")
	c.Redirect(http.StatusFound, "/auth/login")
}

func (m *Manager) requestPrincipal(c *gin.Context) (Principal, bool) {
	cookie, err := c.Request.Cookie(m.config.SessionCookie)
	if err != nil || cookie.Value == "" {
		return Principal{}, false
	}
	now := m.now()
	var sess session
	if m.decodeCookie(m.config.SessionCookie, cookie.Value, &sess) != nil || !time.Unix(sess.ExpiresAt, 0).After(now) || strings.TrimSpace(sess.Username) == "" {
		return Principal{}, false
	}
	return Principal{Username: sess.Username, IsAdmin: sess.IsAdmin}, true
}

func newCookieCipher(secret string) (cipher.AEAD, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("OIDC_SESSION_SECRET must contain at least 32 bytes")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("initialize OIDC session cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize OIDC session encryption: %w", err)
	}
	return aead, nil
}

func (m *Manager) encodeCookie(purpose string, value any) (string, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode OIDC cookie: %w", err)
	}
	nonce := make([]byte, m.cookieCipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("create OIDC cookie nonce: %w", err)
	}
	sealed := m.cookieCipher.Seal(nonce, nonce, plaintext, []byte(purpose))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (m *Manager) decodeCookie(purpose, encoded string, value any) error {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(sealed) < m.cookieCipher.NonceSize() {
		return fmt.Errorf("invalid OIDC cookie encoding")
	}
	nonce := sealed[:m.cookieCipher.NonceSize()]
	plaintext, err := m.cookieCipher.Open(nil, nonce, sealed[m.cookieCipher.NonceSize():], []byte(purpose))
	if err != nil {
		return fmt.Errorf("invalid OIDC cookie authentication")
	}
	if err := json.Unmarshal(plaintext, value); err != nil {
		return fmt.Errorf("decode OIDC cookie: %w", err)
	}
	return nil
}

func (m *Manager) stateCookieName() string {
	return m.config.SessionCookie + "_state"
}

func (m *Manager) clearCookie(c *gin.Context, name, path string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func principalFromIDToken(idToken *oidc.IDToken, cfg Config) (Principal, error) {
	var claims map[string]json.RawMessage
	if err := idToken.Claims(&claims); err != nil {
		return Principal{}, fmt.Errorf("decode OIDC claims: %w", err)
	}
	return principalFromClaims(claims, cfg)
}

func principalFromClaims(claims map[string]json.RawMessage, cfg Config) (Principal, error) {
	username, err := stringClaim(claims, cfg.UsernameClaim)
	if err != nil || username == "" {
		return Principal{}, fmt.Errorf("required username claim %q is missing or invalid", cfg.UsernameClaim)
	}
	groups, err := stringListClaim(claims, cfg.GroupsClaim)
	if err != nil && !errors.Is(err, errClaimMissing) {
		return Principal{}, fmt.Errorf("authorization claim %q is invalid: %w", cfg.GroupsClaim, err)
	}

	return Principal{
		Username: username,
		Groups:   groups,
		IsAdmin:  slices.Contains(groups, cfg.AdminGroup),
	}, nil
}

var errClaimMissing = errors.New("claim missing")

func stringClaim(claims map[string]json.RawMessage, name string) (string, error) {
	raw, ok := claims[name]
	if !ok {
		return "", errClaimMissing
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func stringListClaim(claims map[string]json.RawMessage, name string) ([]string, error) {
	raw, ok := claims[name]
	if !ok || string(raw) == "null" {
		return nil, errClaimMissing
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return normalizedList(values), nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return normalizedList(splitList(value)), nil
}

func normalizedList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func splitList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func safeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "/"
	}
	return value
}

func randomString(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func maxAge(now, expiresAt time.Time) int {
	seconds := int(expiresAt.Sub(now).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
