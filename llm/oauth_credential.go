package llm

// oauth_credential.go — refreshable OAuth bearer support for subscription-backed
// providers (xAI Grok via SuperGrok / X Premium+, and future OAuth subscriptions).
//
// Unlike an API key, an OAuth access_token is short-lived and must be refreshed
// from a stored refresh_token. OpenAICompat therefore accepts a CredentialSource
// instead of a static APIKey: it asks for a fresh bearer on every request, and
// the source refreshes under a lock when the cached token is near expiry.
//
// This file ships the standalone (single-subscription) source: FileTokenSource,
// backed by a JSON file written by `cmd/xai-login`. In the snth fleet the same
// CredentialSource interface is satisfied by a hub-fetching source (private),
// so this OSS engine never learns about the hub.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CredentialSource yields a valid bearer token for the Authorization header,
// refreshing it transparently when needed. Implementations must be safe for
// concurrent use — OpenAICompat may be shared across goroutines.
type CredentialSource interface {
	// Bearer returns a currently-valid access token.
	Bearer() (string, error)
}

// xAI OAuth defaults. The client_id impersonates the official Grok CLI client
// (the same value Hermes uses); xAI does not mint per-app client_ids for
// loopback/device logins, so this is the only id that works with plan=generic.
//
// NOTE: automated inference against a personal SuperGrok / X Premium+
// subscription may violate xAI's Terms of Service and risk the account. This
// is a deliberate operator choice — see docs/providers.md. Both values are
// overridable via env for anyone holding their own registered client.
const (
	defaultXAIOAuthClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	defaultXAIOAuthTokenURL = "https://auth.x.ai/oauth2/token"

	// xaiRefreshSkew: refresh this early before expiry to cover clock skew and
	// the time the caller holds the token in-flight. Mirrors the hub's codex
	// manager (5 min).
	xaiRefreshSkew = 5 * time.Minute

	// xaiFallbackTTL: when a refresh response carries neither expires_in nor a
	// JWT exp claim, assume this conservative lifetime so we refresh
	// periodically instead of on every single request.
	xaiFallbackTTL = 30 * time.Minute
)

// XAIOAuthTokens is the on-disk shape written by cmd/xai-login and rewritten on
// every refresh. Field names match the hub's codex credential blob so the two
// ecosystems stay interchangeable.
type XAIOAuthTokens struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	ExpiresAtUnixMs int64  `json:"expires_at_unix_ms"`
	AccountID       string `json:"account_id,omitempty"`
}

// TierDeniedError signals that xAI accepted the OAuth grant but the account is
// not entitled to API/OAuth inference (HTTP 403). Re-logging in will not fix
// this — the subscription tier itself is gated. Surfaced as a distinct type so
// callers don't loop on re-authentication.
//
// Detail carries only the parsed OAuth error/error_description (never the raw
// response body), so surfacing this error can't leak a token the endpoint
// happened to echo.
type TierDeniedError struct {
	Detail string
}

func (e *TierDeniedError) Error() string {
	msg := "xAI OAuth account is not authorized for API access (HTTP 403). " +
		"xAI restricts OAuth/API inference to specific SuperGrok tiers even when " +
		"the in-app subscription is active; re-login will not help. Use an " +
		"XAI_API_KEY (provider: xai) instead, or upgrade at https://x.ai/grok."
	if e.Detail != "" {
		msg += " Detail: " + e.Detail
	}
	return msg
}

// FileTokenSource is a CredentialSource backed by a local JSON token file. It
// refreshes the access_token against the xAI token endpoint when the cached one
// is within skew of expiry, and persists the rotated pair back to disk.
//
// Single-process safe via an in-process mutex. Multi-process sharing of one
// subscription would race on the refresh_token (xAI invalidates the old chain
// on rotation) — that's what the hub-centralized fleet path exists for.
type FileTokenSource struct {
	path     string
	clientID string
	tokenURL string
	skew     time.Duration

	mu    sync.Mutex
	httpc *http.Client
	// cache is the authoritative token copy once loaded or refreshed. It lets a
	// successful upstream refresh survive a failed disk write (which would
	// otherwise strand the rotated refresh_token and brick the credential), and
	// avoids a file read on every request. Guarded by mu.
	cache *XAIOAuthTokens
}

// NewFileTokenSource builds a source reading/writing the token file at path.
// clientID and tokenURL fall back to the xAI Grok CLI defaults when empty.
func NewFileTokenSource(path, clientID, tokenURL string) *FileTokenSource {
	if clientID == "" {
		clientID = defaultXAIOAuthClientID
	}
	if tokenURL == "" {
		tokenURL = defaultXAIOAuthTokenURL
	}
	return &FileTokenSource{
		path:     path,
		clientID: clientID,
		tokenURL: tokenURL,
		skew:     xaiRefreshSkew,
		httpc: &http.Client{
			Timeout: 30 * time.Second,
			// Refuse redirects: the origin pin only validates the initial
			// URL, but a 307/308 to an off-origin Location would resend the
			// refresh_token body there. A token endpoint never legitimately
			// redirects, so treat any 3xx as a failed refresh.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Bearer returns a valid access token, refreshing under lock if it is missing
// or within skew of expiry.
func (s *FileTokenSource) Bearer() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The in-memory cache is authoritative once populated — it survives a disk
	// that has gone read-only and avoids a file read per request.
	if s.cache == nil {
		t, err := s.read()
		if err != nil {
			return "", err
		}
		s.cache = &t
	}
	tokens := *s.cache

	if tokens.AccessToken != "" && !s.expiring(tokens) {
		return tokens.AccessToken, nil
	}
	if tokens.RefreshToken == "" {
		return "", fmt.Errorf("xai-oauth: token file %s has no refresh_token; re-run `make xai-login`", s.path)
	}

	refreshed, err := s.refresh(tokens)
	if err != nil {
		return "", err
	}
	// Update the in-memory copy BEFORE writing: xAI has already invalidated the
	// old refresh_token, so the rotated pair must not be lost if the disk write
	// fails. Persistence is best-effort — a failed write only costs the rotation
	// across a process restart, never the live request.
	s.cache = &refreshed
	if err := s.write(refreshed); err != nil {
		log.Printf("xai-oauth: refreshed token but could not persist to %s (serving from memory): %v", s.path, err)
	}
	return refreshed.AccessToken, nil
}

// expiring reports whether the cached access token should be refreshed. Prefers
// the stored expires_at_unix_ms; falls back to the JWT `exp` claim.
func (s *FileTokenSource) expiring(t XAIOAuthTokens) bool {
	deadline := time.Now().Add(s.skew).UnixMilli()
	if t.ExpiresAtUnixMs > 0 {
		return t.ExpiresAtUnixMs <= deadline
	}
	if exp, ok := jwtExpUnixMs(t.AccessToken); ok {
		return exp <= deadline
	}
	// Unknown expiry: refresh to be safe.
	return true
}

func (s *FileTokenSource) read() (XAIOAuthTokens, error) {
	var t XAIOAuthTokens
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return t, fmt.Errorf("xai-oauth: read token file %s: %w", s.path, err)
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, fmt.Errorf("xai-oauth: parse token file %s: %w", s.path, err)
	}
	return t, nil
}

// write persists tokens atomically (temp file + rename) so a crash mid-write
// can't truncate the credential file.
func (s *FileTokenSource) write(t XAIOAuthTokens) error {
	raw, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".xai_oauth-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

// refresh exchanges the refresh_token for a new access/refresh pair.
func (s *FileTokenSource) refresh(cur XAIOAuthTokens) (XAIOAuthTokens, error) {
	if err := checkTokenEndpoint(s.tokenURL); err != nil {
		return XAIOAuthTokens{}, err
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", s.clientID)
	form.Set("refresh_token", cur.RefreshToken)

	req, err := http.NewRequest(http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return XAIOAuthTokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpc.Do(req)
	if err != nil {
		return XAIOAuthTokens{}, fmt.Errorf("xai-oauth: token refresh request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Never echo the raw response body into errors: a token endpoint may return
	// a body containing a refresh_token/access_token, and these errors flow to
	// application logs. Surface only the parsed OAuth error/description.
	if resp.StatusCode == http.StatusForbidden {
		return XAIOAuthTokens{}, &TierDeniedError{Detail: oauthErrorDetail(body)}
	}
	if resp.StatusCode != http.StatusOK {
		return XAIOAuthTokens{}, fmt.Errorf("xai-oauth: token refresh HTTP %d%s", resp.StatusCode, detailSuffix(oauthErrorDetail(body)))
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return XAIOAuthTokens{}, fmt.Errorf("xai-oauth: refresh response was not valid JSON")
	}
	if tr.AccessToken == "" {
		return XAIOAuthTokens{}, fmt.Errorf("xai-oauth: refresh response missing access_token%s", detailSuffix(oauthErrorDetail(body)))
	}

	next := XAIOAuthTokens{
		AccessToken:  tr.AccessToken,
		RefreshToken: firstNonEmpty(tr.RefreshToken, cur.RefreshToken),
		AccountID:    cur.AccountID,
	}
	switch {
	case tr.ExpiresIn > 0:
		next.ExpiresAtUnixMs = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second).UnixMilli()
	default:
		if exp, ok := jwtExpUnixMs(tr.AccessToken); ok {
			next.ExpiresAtUnixMs = exp
		} else {
			// No expires_in and not a JWT: assume a conservative lifetime so we
			// don't refresh (and rotate the chain) on every request.
			next.ExpiresAtUnixMs = time.Now().Add(xaiFallbackTTL).UnixMilli()
		}
	}
	return next, nil
}

// oauthErrorDetail extracts the RFC 6749 error / error_description fields (which
// are not secrets) from a token-endpoint response, so refresh failures stay
// descriptive without ever surfacing a body that may contain tokens.
func oauthErrorDetail(body []byte) string {
	var e struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error == "" {
		return ""
	}
	if e.Description != "" {
		return e.Error + ": " + e.Description
	}
	return e.Error
}

func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	return " (" + detail + ")"
}

// checkTokenEndpoint guards the refresh endpoint. Indirected through a var so
// tests can exercise the refresh path against a local mock server.
var checkTokenEndpoint = pinXAIEndpoint

// pinXAIEndpoint refuses any token endpoint that isn't HTTPS on the xAI origin.
// A poisoned token_endpoint (hand-edited config, MITM'd discovery) would
// otherwise receive the refresh_token in plaintext on every refresh — a
// permanent credential leak. Accept x.ai and any *.x.ai subdomain.
func pinXAIEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("xai-oauth: malformed token endpoint %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("xai-oauth: refusing non-HTTPS token endpoint %q (bearer/refresh_token would leak)", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return fmt.Errorf("xai-oauth: token endpoint host %q is not on the xAI origin (expected x.ai or *.x.ai)", host)
	}
	return nil
}

// jwtExpUnixMs extracts the `exp` claim (seconds) from a JWT access token and
// returns it in unix ms. ok=false when the token isn't a parseable JWT.
func jwtExpUnixMs(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some encoders pad; retry with standard URL encoding.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return 0, false
		}
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return 0, false
	}
	return int64(claims.Exp * 1000), true
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
