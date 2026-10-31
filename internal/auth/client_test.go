package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestClientEndpointURLs(t *testing.T) {
	cfg, err := NewServerConfig("https://vault.example.com")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(cfg, "device-123")
	if got := c.identityURL("/connect/token"); got != "https://vault.example.com/identity/connect/token" {
		t.Errorf("identityURL() = %q", got)
	}
	if got := c.apiURL("/accounts/profile"); got != "https://vault.example.com/api/accounts/profile" {
		t.Errorf("apiURL() = %q", got)
	}
	if got := c.DeviceID(); got != "device-123" {
		t.Errorf("DeviceID() = %q", got)
	}
}

func TestClientSendsDesktopIdentityHeaders(t *testing.T) {
	var got http.Header
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg, err := NewServerConfig(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(cfg, "device-123")
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.postForm(context.Background(), c.identityURL("/connect/token"), url.Values{
		"client_id":  {ClientID},
		"grant_type": {"password"},
	}, &out); err != nil {
		t.Fatalf("postForm: %v", err)
	}
	if !out.OK {
		t.Error("response not decoded into out")
	}

	checks := map[string]string{
		"Device-Type":              DeviceType,
		"Bitwarden-Client-Name":    ClientType,
		"Bitwarden-Client-Version": ClientVersion,
		"Bitwarden-Package-Type":   PackageType,
		"Accept":                   "application/json",
		"Content-Type":             "application/x-www-form-urlencoded; charset=utf-8",
	}
	for header, want := range checks {
		if gotValue := got.Get(header); gotValue != want {
			t.Errorf("header %s = %q, want %q", header, gotValue, want)
		}
	}
	if ua := got.Get("User-Agent"); !strings.Contains(ua, "desktop") {
		t.Errorf("User-Agent = %q, want a desktop UA", ua)
	}
	if !strings.Contains(gotBody, "client_id=desktop") {
		t.Errorf("form body = %q, want client_id=desktop", gotBody)
	}
}

func TestClientIdentityConstantsMatchContract(t *testing.T) {
	if ClientID != "desktop" {
		t.Errorf("ClientID = %q, want desktop", ClientID)
	}
	if ClientType != "desktop" {
		t.Errorf("ClientType = %q, want desktop", ClientType)
	}
	if DeviceType != "6" {
		t.Errorf("DeviceType = %q, want 6 (WindowsDesktop)", DeviceType)
	}
	if DeviceName != "windows" {
		t.Errorf("DeviceName = %q, want windows", DeviceName)
	}
	if PackageType != "WindowsUnknown" {
		t.Errorf("PackageType = %q, want WindowsUnknown (non-Store Windows desktop)", PackageType)
	}
}

func TestTokenResponseRedactsSecrets(t *testing.T) {
	const accessToken = "access-token-value-4321"
	const refreshToken = "refresh-token-value-8765"
	const twoFactorToken = "two-factor-token-value-1234"
	resp := TokenResponse{
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		TwoFactorToken: twoFactorToken,
	}
	printed := fmt.Sprintf("%v", resp)
	if want := "auth.TokenResponse(<redacted>)"; printed != want {
		t.Errorf("TokenResponse String() = %q, want %q", printed, want)
	}
	for _, secret := range []string{accessToken, refreshToken, twoFactorToken} {
		if strings.Contains(printed, secret) {
			t.Errorf("TokenResponse %q leaks %q", printed, secret)
		}
	}
}

func TestUserKeyRedactsBytes(t *testing.T) {
	key := UserKey([]byte("0123456789abcdef0123456789abcdef"))
	printed := fmt.Sprintf("%v", key)
	if want := "auth.UserKey(<redacted>)"; printed != want {
		t.Errorf("UserKey String() = %q, want %q", printed, want)
	}
	if strings.Contains(printed, "0123456789abcdef") {
		t.Errorf("UserKey %q leaks the raw key", printed)
	}
	result := fmt.Sprintf("%v", LoginResult{UserKey: key})
	if strings.Contains(result, "0123456789abcdef") {
		t.Errorf("LoginResult %q leaks the raw user key", result)
	}
}

func TestSetAuthEmailHeaderIsBase64URL(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	setAuthEmail(req, "ada@example.com")
	want := base64.RawURLEncoding.EncodeToString([]byte("ada@example.com"))
	if got := req.Header.Get(authEmailHeader); got != want {
		t.Errorf("%s = %q, want %q", authEmailHeader, got, want)
	}

	empty, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	setAuthEmail(empty, "")
	if got := empty.Header.Get(authEmailHeader); got != "" {
		t.Errorf("%s = %q, want empty", authEmailHeader, got)
	}
}

func TestClassifyResponse(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"ok", http.StatusOK, `{}`, nil},
		{"created", http.StatusCreated, `{}`, nil},
		{"unauthorized", http.StatusUnauthorized, `{}`, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, `{}`, ErrForbidden},
		{"forbidden with body", http.StatusForbidden, `{"error":"forbidden"}`, ErrForbidden},
		{"invalid grant", http.StatusBadRequest, `{"error":"invalid_grant"}`, ErrUnauthorized},
		{"two factor required", http.StatusBadRequest, `{"TwoFactorProviders2":{"0":{"challenge":"x"}}}`, ErrTwoFactorRequired},
		// The identity server's real 400: string array plus map, error
		// invalid_grant (login.strategy.spec.ts:357-363).
		{"two factor server shape", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Two factor required.","TwoFactorProviders":["0"],"TwoFactorProviders2":{"0":null}}`, ErrTwoFactorRequired},
		// The array alone still classifies (some servers/tests).
		{"two factor array only", http.StatusBadRequest, `{"TwoFactorProviders":["1"]}`, ErrTwoFactorRequired},
		// "Two factor required." has a space; before normalization this fell
		// through to the invalid_grant branch and looked like a bad password.
		{"two factor message", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Two factor required."}`, ErrTwoFactorRequired},
		{"captcha message", http.StatusBadRequest, `{"ErrorModel":{"Message":"Captcha required."}}`, ErrCaptchaRequired},
		{"captcha field", http.StatusBadRequest, `{"CaptchaRequired":true}`, ErrCaptchaRequired},
		// 429 is a throttle verdict before anything else; the body is advisory
		// and never parsed (Vaultwarden throttles the token endpoint).
		{"rate limited json", http.StatusTooManyRequests, `{"error":"Rate limit exceeded."}`, ErrRateLimited},
		{"rate limited html", http.StatusTooManyRequests, `<!DOCTYPE html><html>Slow down</html>`, ErrRateLimited},
		{"rate limited empty", http.StatusTooManyRequests, ``, ErrRateLimited},
		// A 4xx whose body is not JSON never reached the application (a
		// Cloudflare challenge page, an empty gateway reply), so it must not
		// read as a dead credential or a revoked session.
		{"challenge page not forbidden", http.StatusForbidden, `<!DOCTYPE html><html>Checking your browser</html>`, ErrServer},
		{"challenge page not unauthorized", http.StatusUnauthorized, `<html>Just a moment...</html>`, ErrServer},
		{"empty body not unauthorized", http.StatusUnauthorized, ``, ErrServer},
		{"invalid json not forbidden", http.StatusForbidden, `not json at all`, ErrServer},
		{"server error", http.StatusInternalServerError, ``, ErrServer},
		{"server error html", http.StatusInternalServerError, `<html>502 Bad Gateway</html>`, ErrServer},
		{"bad request", http.StatusBadRequest, `{"ErrorModel":{"Message":"nope"}}`, ErrServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status)}
			got := classifyResponse(resp, []byte(tc.body))
			if tc.want == nil {
				if got != nil {
					t.Fatalf("classifyResponse() = %v, want nil", got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Fatalf("classifyResponse() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A 200 with a non-JSON body is still ErrServer: classifyResponse passes 2xx
// through and decodeJSON rejects the payload.
func TestClientMapsInvalidBodyToServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html>not the vault</html>`))
	}))
	defer srv.Close()

	cfg, err := NewServerConfig(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(cfg, "device-123")
	var out struct {
		OK bool `json:"ok"`
	}
	err = c.getJSON(context.Background(), c.apiURL("/accounts/profile"), &out)
	if !errors.Is(err, ErrServer) {
		t.Fatalf("getJSON error = %v, want ErrServer", err)
	}
}

func TestClientMapsErrorStatusToSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg, err := NewServerConfig(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(cfg, "device-123")
	err = c.getJSON(context.Background(), c.apiURL("/accounts/profile"), nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("getJSON error = %v, want ErrUnauthorized", err)
	}
}

func TestClientMapsTransportFailureToNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg, err := NewServerConfig(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()

	c := NewClient(cfg, "device-123")
	err = c.getJSON(context.Background(), c.apiURL("/accounts/profile"), nil)
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("getJSON error = %v, want ErrNetwork", err)
	}
}

// WithTimeout bounds one request and leaves the client it came from alone:
// the startup restore needs a short deadline while the login path keeps the
// constructor's 30 seconds.
func TestWithTimeoutBoundsOneClientOnly(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	cfg, err := NewServerConfig(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	base := NewClient(cfg, "device-123")
	short := base.WithTimeout(50 * time.Millisecond)

	if base.http.Timeout != 30*time.Second {
		t.Fatalf("base timeout = %v, want the constructor's 30s untouched", base.http.Timeout)
	}
	if short.http.Timeout != 50*time.Millisecond {
		t.Fatalf("short timeout = %v, want 50ms", short.http.Timeout)
	}

	start := time.Now()
	err = short.getJSON(context.Background(), short.apiURL("/accounts/profile"), nil)
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("getJSON error = %v, want ErrNetwork after the short deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("the short timeout took %v; it did not bound the request", elapsed)
	}
}

func TestTwoFactorProvidersMatchContract(t *testing.T) {
	cases := map[TwoFactorProvider]int{
		TwoFactorAuthenticator:   0,
		TwoFactorEmail:           1,
		TwoFactorDuo:             2,
		TwoFactorYubiKey:         3,
		TwoFactorRemember:        5,
		TwoFactorOrganizationDuo: 6,
		TwoFactorWebAuthn:        7,
		TwoFactorRecoveryCode:    8,
	}
	for provider, want := range cases {
		if int(provider) != want {
			t.Errorf("provider %s = %d, want %d", provider, int(provider), want)
		}
	}
}

func TestTwoFactorChallengeHas(t *testing.T) {
	ch := &TwoFactorChallenge{Providers: []TwoFactorProvider{TwoFactorAuthenticator, TwoFactorEmail}}
	if !ch.Has(TwoFactorAuthenticator) || !ch.Has(TwoFactorEmail) {
		t.Error("Has() false for an offered provider")
	}
	if ch.Has(TwoFactorWebAuthn) {
		t.Error("Has() true for a provider that was not offered")
	}
	var nilChallenge *TwoFactorChallenge
	if nilChallenge.Has(TwoFactorEmail) {
		t.Error("Has() true on a nil challenge")
	}
}
