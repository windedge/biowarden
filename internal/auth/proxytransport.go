package auth

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/windedge/biowarden/internal/hostlog"
)

// ProxyMode is one of the four ways the GUI can reach the network. Auth owns
// its own enum so this package stays independent of registry and platform
// seams; the injection side maps its preferences onto these values.
type ProxyMode uint8

const (
	ProxyModeSystem ProxyMode = iota
	ProxyModeEnvironment
	ProxyModeManual
	ProxyModeOff
)

// ProxySetting is the current Manual configuration. The password is supplied
// by the caller so auth never has to read the credential store.
type ProxySetting struct {
	Mode     ProxyMode
	Addr     string
	User     string
	Password string
}

// SystemProxy is the narrow view of the OS setting the transport needs.
type SystemProxy struct {
	HTTP     string
	HTTPS    string
	SOCKS    string
	Override string // Recorded by the platform seam; bypass rules are out of scope.
}

type proxySources struct {
	setting func() ProxySetting
	system  func() (SystemProxy, bool)
}

var (
	transportOnce sync.Once
	shared        *http.Transport

	sourcesMu sync.RWMutex
	sources   proxySources

	// ProxyFromEnvironment caches the first read process-wide. Keeping it in
	// a variable lets a test inject a fresh resolver without changing prod.
	environmentProxy = func(req *http.Request) (*url.URL, error) {
		return http.ProxyFromEnvironment(req)
	}
)

// SharedTransport is the single Transport used by every auth client. It is
// cloned from the default so HTTP/2, TLS and dial defaults stay intact, and
// its Proxy function reads the injected sources on every request.
func SharedTransport() *http.Transport {
	transportOnce.Do(func() {
		base := http.DefaultTransport.(*http.Transport).Clone()
		base.Proxy = proxyForRequest
		shared = base
	})
	return shared
}

// ConfigureProxy installs the configuration sources and drops pooled
// connections, which may still point at the previous proxy. Passing nil for
// both sources restores the original environment-variable behaviour.
func ConfigureProxy(setting func() ProxySetting, system func() (SystemProxy, bool)) {
	sourcesMu.Lock()
	sources = proxySources{setting: setting, system: system}
	sourcesMu.Unlock()
	SwitchProxy()
}

// SwitchProxy clears the connection pool after a proxy change. net/http allows
// CloseIdleConnections to run alongside in-flight requests.
func SwitchProxy() { SharedTransport().CloseIdleConnections() }

func proxyForRequest(req *http.Request) (*url.URL, error) {
	sourcesMu.RLock()
	current := sources
	sourcesMu.RUnlock()

	if current.setting == nil {
		return environmentProxy(req)
	}
	setting := current.setting()
	switch setting.Mode {
	case ProxyModeSystem:
		if current.system != nil {
			if cfg, ok := current.system(); ok {
				addr := cfg.HTTP
				if req.URL != nil && req.URL.Scheme == "https" {
					addr = cfg.HTTPS
				}
				if u := parseProxyURL(addr, "", ""); u != nil {
					return u, nil
				}
				// WinINET can publish only socks=, which has no HTTP fallback.
				if u := parseProxyURL(socksAddress(cfg.SOCKS), "", ""); u != nil {
					return u, nil
				}
			}
		}
		return environmentProxy(req)
	case ProxyModeEnvironment:
		return environmentProxy(req)
	case ProxyModeManual:
		if u := parseProxyURL(setting.Addr, setting.User, setting.Password); u != nil {
			return u, nil
		}
		logInvalidProxyAddress()
		return nil, nil
	case ProxyModeOff:
		return nil, nil
	default:
		return environmentProxy(req)
	}
}

// parseProxyURL accepts both host:port and a full URL. Go's net/http dials
// socks5 and socks5h proxy URLs itself, including username/password auth, so
// no extra dialer is needed.
func parseProxyURL(raw, user, password string) *url.URL {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil
	}
	if user != "" || password != "" {
		u.User = url.UserPassword(user, password)
	}
	return u
}

func socksAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" || strings.Contains(addr, "://") {
		return addr
	}
	return "socks5://" + addr
}

func logInvalidProxyAddress() {
	if hostlog.ParseLevel(os.Getenv("BIOWARDEN_LOG")) != hostlog.LevelDebug {
		return
	}
	// The address may embed credentials, so only the outcome is logged.
	logger().Debug("auth: invalid proxy address, using a direct connection")
}
