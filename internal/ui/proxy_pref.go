package ui

import (
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/internal/sysproxy"
	"github.com/windedge/biowarden/locales"
)

// proxyMode is the Settings page's proxy type. It is an enum so a label can
// follow the language without ever being written back as a stored value.
type proxyMode int

const (
	proxyNone proxyMode = iota
	proxyHTTP
	proxySOCKS5
	proxySystem
)

// proxyModes are the choices in display order. System is last and only when
// the platform can read it; otherwise the row offers only what this build can
// do.
func proxyModes(systemSupported bool) []proxyMode {
	modes := []proxyMode{proxyNone, proxyHTTP, proxySOCKS5}
	if systemSupported {
		return append(modes, proxySystem)
	}
	return modes
}

func (m proxyMode) label() string {
	switch m {
	case proxyNone:
		return locales.SettingsProxyModeNone()
	case proxyHTTP:
		return locales.SettingsProxyModeHTTP()
	case proxySOCKS5:
		return locales.SettingsProxyModeSOCKS5()
	default:
		return locales.SettingsProxyModeSystem()
	}
}

// manual reports whether a mode asks for a host and a port.
func (m proxyMode) manual() bool { return m == proxyHTTP || m == proxySOCKS5 }

func (m proxyMode) stored() registry.ProxyMode {
	switch m {
	case proxyNone:
		return registry.ProxyModeNone
	case proxyHTTP:
		return registry.ProxyModeHTTP
	case proxySOCKS5:
		return registry.ProxyModeSOCKS5
	default:
		return registry.ProxyModeSystem
	}
}

// proxyURL assembles the full address auth expects from the type and the two
// form fields. The scheme lives in the mode, never in the stored address.
func proxyURL(mode proxyMode, host, port string) string {
	if !mode.manual() {
		return ""
	}
	scheme := "http"
	if mode == proxySOCKS5 {
		scheme = "socks5"
	}
	return scheme + "://" + net.JoinHostPort(strings.TrimSpace(host), strings.TrimSpace(port))
}

// validProxyPort is the form's light check before auth validates the composed
// URL: a port is a decimal number in the TCP range.
func validProxyPort(port string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(port))
	return err == nil && n >= 1 && n <= 65535
}

// commitProxy writes the Settings draft through the seam and reports only a
// failure on the page. A mode without credentials also drops the stored
// password, and a blank password means the human cleared it rather than
// wanting the old one kept.
func (s *settingsState) commitProxy() error {
	return s.commitProxyDraft(true)
}

// commitProxyQuiet is for a mode switch or leaving the page: with no host or
// port yet there is nothing to save, and the inline hint under the field
// already explains that traffic goes direct.
func (s *settingsState) commitProxyQuiet() error {
	return s.commitProxyDraft(false)
}

func (s *settingsState) commitProxyDraft(noticeEmpty bool) error {
	if s.proxy == nil {
		return nil
	}
	host, port := trimSpace(s.proxyHost), trimSpace(s.proxyPort)
	if s.proxyMode.manual() {
		if host == "" {
			if noticeEmpty {
				s.notice = pageNotice{kind: noticeSettingsProxyAddressEmpty}
			} else {
				s.notice = pageNotice{}
			}
			return auth.ErrNoProxyAddress
		}
		if !validProxyPort(port) {
			s.notice = pageNotice{kind: noticeSettingsProxyAddressInvalid}
			return auth.ErrInvalidProxyAddress
		}
		if err := auth.ValidateProxyAddress(proxyURL(s.proxyMode, host, port)); err != nil {
			s.notice = pageNotice{kind: noticeSettingsProxyAddressInvalid}
			return err
		}
	}
	password := s.proxyPassword
	if s.proxyMode == proxyNone {
		password = ""
		s.proxyPassword = ""
	}
	s.proxy.Save(s.proxyMode, host, port, trimSpace(s.proxyUser), password)
	// A successful save clears an earlier failure notice; the section itself
	// already shows what took effect.
	s.notice = pageNotice{}
	return nil
}

// commitProxyOnLeave persists a draft only when it differs from what the seam
// holds, so browsing away from an untouched page writes nothing.
func (s *settingsState) commitProxyOnLeave() error {
	if s.hasProxyChanges() {
		return s.commitProxyQuiet()
	}
	return nil
}

func (s *settingsState) hasProxyChanges() bool {
	if s.proxy == nil {
		return false
	}
	if s.proxyMode != s.proxy.Mode() ||
		trimSpace(s.proxyHost) != trimSpace(s.proxy.Host()) ||
		trimSpace(s.proxyPort) != trimSpace(s.proxy.Port()) ||
		trimSpace(s.proxyUser) != trimSpace(s.proxy.User()) {
		return true
	}
	stored, _ := s.proxy.Password()
	if s.proxyMode == proxyNone {
		return stored != "" || s.proxyPassword != ""
	}
	return s.proxyPassword != stored
}

func proxyModeFromStored(value registry.ProxyMode) proxyMode {
	switch value {
	case registry.ProxyModeHTTP:
		return proxyHTTP
	case registry.ProxyModeSOCKS5:
		return proxySOCKS5
	case registry.ProxyModeNone:
		return proxyNone
	default:
		return proxySystem
	}
}

func (m proxyMode) authMode() auth.ProxyMode {
	switch m {
	case proxyHTTP, proxySOCKS5:
		return auth.ProxyModeManual
	case proxyNone:
		return auth.ProxyModeOff
	default:
		return auth.ProxyModeSystem
	}
}

// proxyPref is the Settings page's whole view of proxy configuration: the
// persisted choice, the password and the platform's system-proxy capability.
// The pages never import the platform seams themselves.
type proxyPref interface {
	Mode() proxyMode
	Host() string
	Port() string
	User() string
	Password() (string, bool)
	// HasCredentials reports whether anything is stored, so a saved login
	// is visible without having to open the disclosure first.
	HasCredentials() bool
	SystemSupported() bool
	// system reports the OS setting auth uses in System mode.
	system() (auth.SystemProxy, bool)
	// Save writes the preference values and the password, then drops the
	// pooled connections that may still point at the previous proxy.
	Save(mode proxyMode, host, port, user, password string)
	ClearPassword()
}

// runtimeProxyPref is the production seam over the registry preference, the
// credential store and the platform's system-proxy reader.
type runtimeProxyPref struct {
	pref    registry.ProxyPref
	secrets secrets.ProxySecrets
	reader  sysproxy.Reader
}

func newProxyPref() proxyPref {
	var reader sysproxy.Reader
	if sysproxy.Supported() {
		reader = sysproxy.NewReader()
	}
	return &runtimeProxyPref{
		pref:    registry.NewProxyPref(),
		secrets: secrets.NewProxySecrets(),
		reader:  reader,
	}
}

func (p *runtimeProxyPref) Mode() proxyMode { return proxyModeFromStored(p.pref.Mode()) }
func (p *runtimeProxyPref) Host() string    { return p.pref.Host() }
func (p *runtimeProxyPref) Port() string    { return p.pref.Port() }
func (p *runtimeProxyPref) User() string    { return p.pref.User() }

func (p *runtimeProxyPref) Password() (string, bool) {
	password, err := p.secrets.GetProxyPassword()
	if err != nil {
		return "", false
	}
	return password, true
}

func (p *runtimeProxyPref) HasCredentials() bool {
	if trimSpace(p.pref.User()) != "" {
		return true
	}
	_, ok := p.Password()
	return ok
}

func (p *runtimeProxyPref) SystemSupported() bool { return p.reader != nil }

func (p *runtimeProxyPref) system() (auth.SystemProxy, bool) {
	if p.reader == nil {
		return auth.SystemProxy{}, false
	}
	cfg, ok := p.reader.System()
	return auth.SystemProxy{HTTP: cfg.HTTP, HTTPS: cfg.HTTPS, SOCKS: cfg.SOCKS, Override: cfg.Override}, ok
}

func (p *runtimeProxyPref) Save(mode proxyMode, host, port, user, password string) {
	p.pref.Set(mode.stored(), trimSpace(host), trimSpace(port), trimSpace(user))
	if mode == proxyNone {
		p.ClearPassword()
	} else if password == "" {
		// An empty field removes a stored password: leaving the old one would
		// silently go on authenticating with credentials the human cleared.
		p.ClearPassword()
	} else if err := p.secrets.PutProxyPassword(password); err != nil {
		// The preference still took effect; a failed credential write only
		// means the next connection may lack authentication.
		logProxyError("write proxy password", err)
	}
	auth.SwitchProxy()
}

func (p *runtimeProxyPref) ClearPassword() {
	if err := p.secrets.RemoveProxyPassword(); err != nil {
		logProxyError("remove proxy password", err)
	}
}

// logProxyError names only the failed operation: an error from the credential
// store must never be allowed to carry the password into a log.
func logProxyError(operation string, err error) {
	log.Println("biowarden:", operation, "failed:", err)
}

// wireProxySources installs the UI seam as auth's configuration source. The
// closures read the live preference on every request, so a Save needs no
// re-wiring; only the pooled connections need dropping, which Save does.
func wireProxySources(pref proxyPref) {
	if pref == nil {
		auth.ConfigureProxy(nil, nil)
		return
	}
	auth.ConfigureProxy(func() auth.ProxySetting {
		mode := pref.Mode()
		setting := auth.ProxySetting{
			Mode: mode.authMode(),
			Addr: proxyURL(mode, pref.Host(), pref.Port()),
			User: pref.User(),
		}
		if password, ok := pref.Password(); ok {
			setting.Password = password
		}
		return setting
	}, pref.system)
}
