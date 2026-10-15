package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Desktop identity. biowarden impersonates the official Bitwarden desktop
// client, not a browser (goldwarden uses web/firefox, which is a different
// identity). These values go into the token request body and into the
// Bitwarden-* request headers.
const (
	// ClientID is the OAuth client_id sent to {identity}/connect/token.
	ClientID = "desktop"
	// ClientType is the Bitwarden-Client-Name header value and equals the
	// platform's getClientType().
	ClientType = "desktop"
	// DeviceType is DeviceType.WindowsDesktop, sent as deviceType.
	DeviceType = "6"
	// DeviceName is the desktop client's device name on Windows.
	DeviceName = "windows"
	// ClientVersion is the Bitwarden-Client-Version header value. It trails
	// the official desktop release so the server sees a current client.
	ClientVersion = "2026.10.0"
	// UserAgent is the desktop User-Agent. The official desktop sends an
	// Electron UA; this is its stable, identifying part.
	UserAgent = "Bitwarden/desktop v" + ClientVersion + " (Windows)"
	// PackageType is the Bitwarden-Package-Type header value. The official
	// desktop sends it on every request through applyPlatformHeaders
	// (libs/common/src/services/api.service.ts:1330-1332). packageType()
	// returns "WindowsStore" for the Store build and "WindowsUnknown" for a
	// non-Store Windows desktop install
	// (apps/desktop/src/platform/services/electron-platform-utils.service.ts:162-163
	// and :170-171). biowarden is a non-Store Windows desktop, so it sends
	// WindowsUnknown.
	PackageType = "WindowsUnknown"
)

// authEmailHeader carries the base64url email on identity requests that need
// it, matching what the official client sends for 2FA lookups.
const authEmailHeader = "Auth-Email"

// Sentinel errors. Every failure the login flow cares about is one of these,
// so callers can branch without parsing strings.
var (
	// ErrNotImplemented marks the skeleton: the core login ticket replaces
	// these stubs.
	ErrNotImplemented = errors.New("auth: not implemented yet")
	// ErrUnauthorized is a rejected credential: HTTP 401, or a prelogin /
	// token request the server refused. A wrong password arrives as HTTP 400
	// invalid_grant and is mapped here too.
	ErrUnauthorized = errors.New("auth: unauthorized")
	// ErrForbidden is an authenticated-but-refused request: HTTP 403. The
	// official identity answers a wrong password with 400 invalid_grant, so a
	// 403 is not a bad credential but a blocked account or an expired
	// session; folding it into ErrUnauthorized would tell the user to re-enter
	// the password. It stays distinct from ErrServer so callers can still tell
	// a policy refusal from a transport or server fault.
	ErrForbidden = errors.New("auth: forbidden")
	// ErrTwoFactorRequired means the token endpoint wants a second factor;
	// the response carries the offered providers.
	ErrTwoFactorRequired = errors.New("auth: two-factor required")
	// ErrCaptchaRequired means the server demands a CAPTCHA, which cannot be
	// solved headless.
	ErrCaptchaRequired = errors.New("auth: captcha required")
	// ErrNetwork wraps a transport failure.
	ErrNetwork = errors.New("auth: network error")
	// ErrServer wraps an unexpected HTTP status or an unreadable response.
	ErrServer = errors.New("auth: server error")
)

// Client is the HTTP transport shared by every identity and API call. It owns
// the base URLs and applies the desktop identity headers to each request.
type Client struct {
	http     *http.Client
	urls     ServiceURLs
	deviceID string
}

// NewClient builds a client for cfg. deviceID is the stable per-install
// identifier sent as deviceIdentifier; it substitutes for the official
// desktop app's appId.
func NewClient(cfg ServerConfig, deviceID string) *Client {
	return &Client{
		http:     &http.Client{Timeout: 30 * time.Second},
		urls:     cfg.URLs(),
		deviceID: deviceID,
	}
}

// URLs returns the service URLs the client was built with.
func (c *Client) URLs() ServiceURLs { return c.urls }

// DeviceID returns the deviceIdentifier this client sends.
func (c *Client) DeviceID() string { return c.deviceID }

// identityURL joins path onto the identity service root.
func (c *Client) identityURL(path string) string { return joinURL(c.urls.Identity, path) }

// apiURL joins path onto the API service root.
func (c *Client) apiURL(path string) string { return joinURL(c.urls.API, path) }

// applyHeaders stamps the desktop identity headers onto req. The official
// client sets these through applyPlatformHeaders; the values must stay in sync
// with the constants above or the server may reject the login.
func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Device-Type", DeviceType)
	req.Header.Set("Bitwarden-Client-Name", ClientType)
	req.Header.Set("Bitwarden-Client-Version", ClientVersion)
	req.Header.Set("Bitwarden-Package-Type", PackageType)
}

// setAuthEmail adds the base64url-encoded email header some 2FA endpoints
// require. An empty email is a no-op.
func setAuthEmail(req *http.Request, email string) {
	if email == "" {
		return
	}
	req.Header.Set(authEmailHeader, base64.RawURLEncoding.EncodeToString([]byte(email)))
}

// postForm sends an application/x-www-form-urlencoded body and decodes a JSON
// response into out.
func (c *Client) postForm(ctx context.Context, rawurl string, form url.Values, out any) error {
	body := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawurl, body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	c.applyHeaders(req)
	return c.decode(req, out)
}

// postJSON sends a JSON body and decodes a JSON response into out.
func (c *Client) postJSON(ctx context.Context, rawurl string, in any, out any) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(in); err != nil {
		return fmt.Errorf("%w: encode request: %v", ErrServer, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawurl, buf)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyHeaders(req)
	return c.decode(req, out)
}

// getJSON issues a GET with the desktop identity headers and decodes the JSON
// response into out.
func (c *Client) getJSON(ctx context.Context, rawurl string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	c.applyHeaders(req)
	return c.decode(req, out)
}

// decode runs req, maps the status onto the sentinel errors and unmarshals a
// successful body into out.
func (c *Client) decode(req *http.Request, out any) error {
	body, err := c.do(req)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrServer, err)
	}
	return nil
}

// do performs one request and returns the response body. A non-2xx status is
// turned into a sentinel error by classifyResponse.
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrNetwork, err)
	}
	if err := classifyResponse(resp, body); err != nil {
		return body, err
	}
	return body, nil
}

// errorResponse is the shape of a Bitwarden API error body.
type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorModel       struct {
		Message string `json:"Message"`
	} `json:"ErrorModel"`
	// TwoFactorProviders2 is present, keyed by provider number, on the 400
	// that asks for a second factor.
	TwoFactorProviders2 map[string]json.RawMessage `json:"TwoFactorProviders2"`
	// CaptchaRequired is sometimes a dedicated field.
	CaptchaRequired bool `json:"CaptchaRequired"`
}

// classifyResponse maps an HTTP status and body onto the sentinel errors.
// It returns nil for any 2xx status.
func classifyResponse(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var er errorResponse
	_ = json.Unmarshal(body, &er)
	message := strings.ToLower(strings.Join([]string{
		er.Error, er.ErrorDescription, er.ErrorModel.Message,
	}, " "))

	if strings.Contains(message, "captcha") || er.CaptchaRequired {
		return fmt.Errorf("%w: %s", ErrCaptchaRequired, statusDetail(resp))
	}
	if len(er.TwoFactorProviders2) > 0 || strings.Contains(message, "twofactor") {
		return fmt.Errorf("%w: %s", ErrTwoFactorRequired, statusDetail(resp))
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, statusDetail(resp))
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrForbidden, statusDetail(resp))
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: %s", ErrServer, statusDetail(resp))
	default:
		// 400 invalid_grant and friends: the credentials were rejected.
		if strings.Contains(message, "invalid_grant") {
			return fmt.Errorf("%w: %s", ErrUnauthorized, statusDetail(resp))
		}
		return fmt.Errorf("%w: %s", ErrServer, statusDetail(resp))
	}
}

// statusDetail renders a short, safe description of a failed response. The
// body is not included because it may echo request fields.
func statusDetail(resp *http.Response) string {
	return fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
}
