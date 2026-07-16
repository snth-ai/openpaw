package llm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeJWT builds a minimal unsigned JWT carrying an exp claim (unix seconds).
func makeJWT(expUnix int64) string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body, _ := json.Marshal(map[string]any{"exp": expUnix, "sub": "user-123"})
	return hdr + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}

func writeTokens(t *testing.T, dir string, tok XAIOAuthTokens) string {
	t.Helper()
	p := filepath.Join(dir, "xai_oauth.json")
	raw, _ := json.MarshalIndent(tok, "", "  ")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write tokens: %v", err)
	}
	return p
}

func TestBearer_FreshToken_NoRefresh(t *testing.T) {
	dir := t.TempDir()
	p := writeTokens(t, dir, XAIOAuthTokens{
		AccessToken:     "fresh-access",
		RefreshToken:    "rt",
		ExpiresAtUnixMs: time.Now().Add(1 * time.Hour).UnixMilli(),
	})
	// tokenURL points nowhere valid: if the code tried to refresh, it'd error.
	s := NewFileTokenSource(p, "cid", "https://api.x.ai/oauth2/token")
	got, err := s.Bearer()
	if err != nil {
		t.Fatalf("Bearer: %v", err)
	}
	if got != "fresh-access" {
		t.Fatalf("expected cached token, got %q", got)
	}
}

func TestBearer_Expired_RefreshesAndPersists(t *testing.T) {
	// Mock xAI token endpoint.
	var gotGrant, gotRefresh, gotClient string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		gotClient = r.Form.Get("client_id")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"rt-new","expires_in":3600}`, makeJWT(time.Now().Add(time.Hour).Unix()))
	}))
	defer srv.Close()

	// Allow the mock host past the origin pin for this test only.
	orig := checkTokenEndpoint
	checkTokenEndpoint = func(string) error { return nil }
	defer func() { checkTokenEndpoint = orig }()

	dir := t.TempDir()
	p := writeTokens(t, dir, XAIOAuthTokens{
		AccessToken:     "stale-access",
		RefreshToken:    "rt-old",
		ExpiresAtUnixMs: time.Now().Add(-1 * time.Minute).UnixMilli(), // expired
		AccountID:       "acct-1",
	})
	s := NewFileTokenSource(p, "my-client", srv.URL)

	got, err := s.Bearer()
	if err != nil {
		t.Fatalf("Bearer: %v", err)
	}
	if got == "stale-access" {
		t.Fatalf("expected refreshed token, still stale")
	}
	if gotGrant != "refresh_token" || gotRefresh != "rt-old" || gotClient != "my-client" {
		t.Fatalf("bad refresh request: grant=%q refresh=%q client=%q", gotGrant, gotRefresh, gotClient)
	}

	// The rotated pair must be persisted back to disk, preserving account_id.
	raw, _ := os.ReadFile(p)
	var saved XAIOAuthTokens
	_ = json.Unmarshal(raw, &saved)
	if saved.RefreshToken != "rt-new" {
		t.Fatalf("refresh_token not rotated on disk: %q", saved.RefreshToken)
	}
	if saved.AccessToken != got {
		t.Fatalf("access_token not persisted: disk=%q returned=%q", saved.AccessToken, got)
	}
	if saved.AccountID != "acct-1" {
		t.Fatalf("account_id lost across refresh: %q", saved.AccountID)
	}
	if saved.ExpiresAtUnixMs <= time.Now().UnixMilli() {
		t.Fatalf("expiry not advanced: %d", saved.ExpiresAtUnixMs)
	}
}

func TestBearer_TierDenied403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	orig := checkTokenEndpoint
	checkTokenEndpoint = func(string) error { return nil }
	defer func() { checkTokenEndpoint = orig }()

	dir := t.TempDir()
	p := writeTokens(t, dir, XAIOAuthTokens{
		AccessToken:     "x",
		RefreshToken:    "rt",
		ExpiresAtUnixMs: time.Now().Add(-time.Minute).UnixMilli(),
	})
	s := NewFileTokenSource(p, "cid", srv.URL)
	_, err := s.Bearer()
	if err == nil {
		t.Fatal("expected error on 403")
	}
	var tde *TierDeniedError
	if !errorAs(err, &tde) {
		t.Fatalf("expected TierDeniedError, got %T: %v", err, err)
	}
}

func TestExpiring(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		tok  XAIOAuthTokens
		want bool
	}{
		{"far future expires_at", XAIOAuthTokens{ExpiresAtUnixMs: now.Add(time.Hour).UnixMilli()}, false},
		{"within skew expires_at", XAIOAuthTokens{ExpiresAtUnixMs: now.Add(2 * time.Minute).UnixMilli()}, true},
		{"past expires_at", XAIOAuthTokens{ExpiresAtUnixMs: now.Add(-time.Second).UnixMilli()}, true},
		{"jwt fallback far", XAIOAuthTokens{AccessToken: makeJWT(now.Add(time.Hour).Unix())}, false},
		{"jwt fallback near", XAIOAuthTokens{AccessToken: makeJWT(now.Add(time.Minute).Unix())}, true},
		{"no expiry info", XAIOAuthTokens{AccessToken: "not-a-jwt"}, true},
	}
	s := &FileTokenSource{skew: xaiRefreshSkew}
	for _, c := range cases {
		if got := s.expiring(c.tok); got != c.want {
			t.Errorf("%s: expiring=%v want %v", c.name, got, c.want)
		}
	}
}

func TestPinXAIEndpoint(t *testing.T) {
	ok := []string{
		"https://auth.x.ai/oauth2/token",
		"https://api.x.ai/oauth2/token",
		"https://x.ai/oauth2/token",
	}
	for _, u := range ok {
		if err := pinXAIEndpoint(u); err != nil {
			t.Errorf("expected %q allowed, got %v", u, err)
		}
	}
	bad := []string{
		"http://auth.x.ai/oauth2/token",         // not https
		"https://attacker.example/oauth2/token", // wrong origin
		"https://auth.x.ai.attacker.com/token",  // suffix trick
		"https://notx.ai/token",                 // lookalike host
	}
	for _, u := range bad {
		if err := pinXAIEndpoint(u); err == nil {
			t.Errorf("expected %q rejected, got nil", u)
		}
	}
}

func TestJWTExp(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	ms, ok := jwtExpUnixMs(makeJWT(exp))
	if !ok || ms != exp*1000 {
		t.Fatalf("jwtExpUnixMs=%d ok=%v want %d", ms, ok, exp*1000)
	}
	if _, ok := jwtExpUnixMs("garbage"); ok {
		t.Fatal("expected non-JWT to fail")
	}
}

// countingRefreshServer returns a token endpoint that records how many times it
// is hit and replies with the given handler.
func countingRefreshServer(t *testing.T, handler func(w http.ResponseWriter, hits int)) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		handler(w, hits)
	}))
	t.Cleanup(srv.Close)
	// Let the mock host past the origin pin for the test.
	orig := checkTokenEndpoint
	checkTokenEndpoint = func(string) error { return nil }
	t.Cleanup(func() { checkTokenEndpoint = orig })
	return srv, &hits
}

// #2 + #4: a disk-write failure after a successful upstream refresh must NOT
// fail the request or brick the credential — the rotated token is served from
// memory and reused on the next call.
func TestBearer_PersistFailure_ServesFromMemory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based write-failure injection is a no-op as root")
	}
	freshJWT := makeJWT(time.Now().Add(time.Hour).Unix())
	srv, hits := countingRefreshServer(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"rt-new","expires_in":3600}`, freshJWT)
	})

	dir := t.TempDir()
	p := writeTokens(t, dir, XAIOAuthTokens{
		AccessToken:     "stale",
		RefreshToken:    "rt-old",
		ExpiresAtUnixMs: time.Now().Add(-time.Minute).UnixMilli(),
	})
	// Make the directory read-only so the atomic temp-file write fails, while
	// the existing token file is still readable.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	s := NewFileTokenSource(p, "cid", srv.URL)
	got, err := s.Bearer()
	if err != nil {
		t.Fatalf("Bearer should not error on persist failure, got: %v", err)
	}
	if got != freshJWT {
		t.Fatalf("expected refreshed token served from memory, got %q", got)
	}
	// Second call must reuse the in-memory rotated token — no re-refresh, no
	// re-reading the un-updated (rt-old) file.
	got2, err := s.Bearer()
	if err != nil {
		t.Fatalf("second Bearer errored: %v", err)
	}
	if got2 != freshJWT {
		t.Fatalf("second call returned %q, expected cached %q", got2, freshJWT)
	}
	if *hits != 1 {
		t.Fatalf("expected exactly 1 refresh, got %d (refresh storm / brick)", *hits)
	}
}

// #3: a refresh response with no expires_in and a non-JWT access token must get
// a fallback TTL, not refresh on every subsequent call.
func TestBearer_UnknownExpiry_NoRefreshStorm(t *testing.T) {
	srv, hits := countingRefreshServer(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Content-Type", "application/json")
		// opaque (non-JWT) access token, no expires_in
		fmt.Fprint(w, `{"access_token":"opaque-token","refresh_token":"rt2"}`)
	})

	dir := t.TempDir()
	p := writeTokens(t, dir, XAIOAuthTokens{
		AccessToken:     "stale",
		RefreshToken:    "rt1",
		ExpiresAtUnixMs: time.Now().Add(-time.Minute).UnixMilli(),
	})
	s := NewFileTokenSource(p, "cid", srv.URL)

	if _, err := s.Bearer(); err != nil {
		t.Fatalf("Bearer: %v", err)
	}
	if _, err := s.Bearer(); err != nil {
		t.Fatalf("Bearer 2: %v", err)
	}
	if *hits != 1 {
		t.Fatalf("expected 1 refresh, got %d (unknown expiry caused a refresh storm)", *hits)
	}
	// The persisted expiry must be advanced into the future.
	raw, _ := os.ReadFile(p)
	var saved XAIOAuthTokens
	_ = json.Unmarshal(raw, &saved)
	if saved.ExpiresAtUnixMs <= time.Now().UnixMilli() {
		t.Fatalf("fallback TTL not applied: expires_at=%d", saved.ExpiresAtUnixMs)
	}
}

// #1: response bodies that may contain tokens must never appear in the returned
// error (they flow to application logs).
func TestRefresh_NoSecretLeakInError(t *testing.T) {
	const secret = "SUPER-SECRET-RT"

	t.Run("200 missing access_token", func(t *testing.T) {
		srv, _ := countingRefreshServer(t, func(w http.ResponseWriter, _ int) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"refresh_token":%q}`, secret)
		})
		dir := t.TempDir()
		p := writeTokens(t, dir, XAIOAuthTokens{AccessToken: "x", RefreshToken: "rt", ExpiresAtUnixMs: time.Now().Add(-time.Minute).UnixMilli()})
		_, err := NewFileTokenSource(p, "cid", srv.URL).Bearer()
		if err == nil {
			t.Fatal("expected error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked secret body: %v", err)
		}
	})

	t.Run("4xx echoes oauth error only", func(t *testing.T) {
		srv, _ := countingRefreshServer(t, func(w http.ResponseWriter, _ int) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_grant","error_description":"expired","refresh_token":%q}`, secret)
		})
		dir := t.TempDir()
		p := writeTokens(t, dir, XAIOAuthTokens{AccessToken: "x", RefreshToken: "rt", ExpiresAtUnixMs: time.Now().Add(-time.Minute).UnixMilli()})
		_, err := NewFileTokenSource(p, "cid", srv.URL).Bearer()
		if err == nil {
			t.Fatal("expected error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked secret body: %v", err)
		}
		if !strings.Contains(err.Error(), "invalid_grant") {
			t.Fatalf("expected oauth error surfaced, got: %v", err)
		}
	})
}

// errorAs is a tiny local errors.As to avoid an import churn in this file.
func errorAs(err error, target **TierDeniedError) bool {
	for err != nil {
		if e, ok := err.(*TierDeniedError); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
