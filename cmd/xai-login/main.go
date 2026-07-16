// Standalone helper to obtain xAI Grok OAuth tokens from a SuperGrok / X
// Premium+ subscription, so OpenPaw can run inference on the subscription's
// limits instead of a paid XAI_API_KEY.
//
// Run on a machine with a browser:
//
//	make xai-login              # or: go run ./cmd/xai-login
//
// A browser opens, you sign in to x.ai, and the token JSON is written to
// ./xai_oauth.json (override with -out). OpenPaw picks it up on next start and
// registers a "xai-oauth" provider.
//
// Headless / remote box with no local browser:
//
//	go run ./cmd/xai-login -device
//
// prints a short code to enter at https://x.ai on any device.
//
// NOTE: this impersonates the official Grok CLI OAuth client (the only client
// id that works for loopback/device logins). Automated inference against a
// personal subscription may violate xAI's Terms of Service — use deliberately.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Endpoints from https://auth.x.ai/.well-known/openid-configuration.
const (
	clientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	authorizeURL = "https://auth.x.ai/oauth2/authorize"
	tokenURL     = "https://auth.x.ai/oauth2/token"
	deviceURL    = "https://auth.x.ai/oauth2/device/code"
	scope        = "openid profile email offline_access grok-cli:access api:access"
	redirectURI  = "http://127.0.0.1:56121/callback"
	listenAddr   = "127.0.0.1:56121"
	referrer     = "openpaw"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
}

// output matches llm.XAIOAuthTokens — the shape OpenPaw reads and rewrites.
type output struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	ExpiresAtUnixMs int64  `json:"expires_at_unix_ms"`
	AccountID       string `json:"account_id,omitempty"`
}

func main() {
	device := flag.Bool("device", false, "use device-code flow (no local browser / headless box)")
	outPath := flag.String("out", "xai_oauth.json", "path to write the token JSON")
	flag.Parse()

	var tok *tokenResponse
	if *device {
		tok = runDeviceFlow()
	} else {
		tok = runLocalhostFlow()
	}

	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UnixMilli()
	out := output{
		AccessToken:     tok.AccessToken,
		RefreshToken:    tok.RefreshToken,
		ExpiresAtUnixMs: expiresAt,
		AccountID:       accountIDFromJWT(tok.AccessToken),
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	must(err)
	must(os.WriteFile(*outPath, raw, 0o600))

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "✓ Tokens written to %s (mode 0600).\n", *outPath)
	fmt.Fprintln(os.Stderr, "  Start OpenPaw and switch provider to `xai-oauth`.")
}

// runLocalhostFlow is the browser + 127.0.0.1:56121 callback PKCE flow.
func runLocalhostFlow() *tokenResponse {
	verifier, challenge, err := generatePKCE()
	must(err)
	state, err := randomHex(16)
	must(err)

	authURL := buildAuthURL(challenge, state)
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Addr: listenAddr, Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- errors.New("state mismatch (possible CSRF)")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- errors.New("callback missing authorization code")
			return
		}
		fmt.Fprint(w, `<!doctype html><meta charset=utf-8><body style="font-family:system-ui;padding:40px;text-align:center"><h2>xAI authentication complete.</h2><p>You can close this window and return to the terminal.</p></body>`)
		codeCh <- code
	})

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("local callback server: %w", err)
		}
	}()

	fmt.Fprintln(os.Stderr, "Opening browser for xAI Grok login…")
	fmt.Fprintln(os.Stderr, "If it doesn't open, paste this URL manually:")
	fmt.Fprintln(os.Stderr, "  "+authURL)
	_ = openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		_ = srv.Shutdown(context.Background())
		fail(err)
	case <-time.After(5 * time.Minute):
		_ = srv.Shutdown(context.Background())
		fail(errors.New("timed out waiting for OAuth callback after 5 minutes"))
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	tok, err := exchangeCode(code, verifier, challenge, redirectURI)
	must(err)
	return tok
}

// runDeviceFlow uses the OAuth 2.0 device authorization grant (RFC 8628): no
// localhost callback, no local browser. Works on any headless box.
func runDeviceFlow() *tokenResponse {
	verifier, challenge, err := generatePKCE()
	must(err)

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", scope)
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")
	raw, status, err := formPost(deviceURL, form)
	must(err)
	if status != 200 {
		fail(fmt.Errorf("device authorization request failed: HTTP %d: %s", status, string(raw)))
	}
	var da struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	must(json.Unmarshal(raw, &da))
	if da.DeviceCode == "" || da.UserCode == "" {
		fail(fmt.Errorf("device response missing fields: %s", string(raw)))
	}
	verifyURL := da.VerificationURI
	if da.VerificationURIComplete != "" {
		verifyURL = da.VerificationURIComplete
	}
	interval := time.Duration(maxInt(da.Interval, 5)) * time.Second

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  ┌───────────────────────────────────────────────┐")
	fmt.Fprintf(os.Stderr, "  │  Open:  %-38s│\n", verifyURL)
	fmt.Fprintf(os.Stderr, "  │  Code:  %-38s│\n", da.UserCode)
	fmt.Fprintln(os.Stderr, "  └───────────────────────────────────────────────┘")
	fmt.Fprintln(os.Stderr, "  Sign in on ANY device, then enter the code. Waiting…")

	deadline := time.Now().Add(time.Duration(maxInt(da.ExpiresIn, 300)) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		pf := url.Values{}
		pf.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		pf.Set("device_code", da.DeviceCode)
		pf.Set("client_id", clientID)
		pf.Set("code_verifier", verifier)
		body, status, err := formPost(tokenURL, pf)
		must(err)
		if status == 200 {
			var t tokenResponse
			must(json.Unmarshal(body, &t))
			if t.AccessToken == "" || t.RefreshToken == "" {
				fail(fmt.Errorf("device token response missing fields: %s", string(body)))
			}
			return &t
		}
		// authorization_pending / slow_down → keep polling; anything else is fatal.
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		switch e.Error {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		default:
			fail(fmt.Errorf("device authorization failed: HTTP %d: %s", status, string(body)))
		}
	}
	fail(errors.New("device authorization timed out"))
	return nil
}

func generatePKCE() (verifier, challenge string, err error) {
	v := make([]byte, 32)
	if _, err := rand.Read(v); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(v)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func buildAuthURL(challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	// plan=generic opts the consent screen into xAI's generic OAuth plan so
	// loopback logins from non-allowlisted clients are accepted; referrer is
	// best-effort attribution.
	q.Set("plan", "generic")
	q.Set("referrer", referrer)
	return authorizeURL + "?" + q.Encode()
}

func exchangeCode(code, verifier, challenge, redirect string) (*tokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("client_id", clientID)
	body.Set("code", code)
	body.Set("code_verifier", verifier)
	body.Set("redirect_uri", redirect)
	// Some xAI deployments re-validate the challenge at the token step; echoing
	// it is harmless for strict RFC servers and fixes "code_challenge required".
	body.Set("code_challenge", challenge)
	body.Set("code_challenge_method", "S256")

	raw, status, err := formPost(tokenURL, body)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	if status != 200 {
		return nil, fmt.Errorf("token exchange failed: HTTP %d: %s", status, string(raw))
	}
	var t tokenResponse
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("decode token response: %w (body: %s)", err, string(raw))
	}
	if t.AccessToken == "" || t.RefreshToken == "" {
		return nil, fmt.Errorf("token response missing fields: %s", string(raw))
	}
	return &t, nil
}

// httpClient refuses redirects so a 3xx from a token/device endpoint can never
// resend the PKCE verifier or device_code body to an off-origin Location.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func formPost(target string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// accountIDFromJWT best-effort extracts a stable account/subject id from the
// access token for display. Returns "" when unavailable — it is not required.
func accountIDFromJWT(accessToken string) string {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if payload, err = base64.URLEncoding.DecodeString(parts[1]); err != nil {
			return ""
		}
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	for _, k := range []string{"sub", "account_id", "user_id"} {
		if v, ok := claims[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func openBrowser(rawURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", rawURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func must(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
