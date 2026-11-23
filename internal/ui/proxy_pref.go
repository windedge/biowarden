package ui

import (
	"log"

	"github.com/windedge/biowarden/internal/auth"
	"github.com/windedge/biowarden/internal/registry"
	"github.com/windedge/biowarden/internal/secrets"
	"github.com/windedge/biowarden/internal/sysproxy"
	"github.com/windedge/biowarden/locales"
)

// proxyMode is the Settings page's proxy choice. It is an enum so a label can
// follow the language without ever being written back as a stored value.
type proxyMode int

const (
	proxySystem proxyMode = iota
	proxyManual
	proxyOff
)

// proxyModes are the choices in display order. System is first when the
// platform can read it; otherwise the row offers only what this build can do.
func proxyModes(systemSupported bool) []proxyMode {
	modes := []proxyMode{proxyManual, proxyOff}
	if systemSupported {
		return append([]proxyMode{proxySystem}, modes...)
	}
	return modes
}

func (m proxyMode) label() string {
	switch m {
	case proxyManual:
		return locales.SettingsProxyModeManual()
	case proxyOff:
		return locales.SettingsProxyModeOff()
	default:
		return locales.SettingsProxyModeSystem()
	}
}

func (m proxyMode) stored() registry.ProxyMode {
	switch m {
	case proxyManual:
		return registry.ProxyModeManual
	case proxyOff:
		return registry.ProxyModeOff
	default:
		return registry.ProxyModeSystem
	}
}

// commitProxy writes the Settings draft through the seam. A mode without
// credentials also drops the stored password, and a blank field means the
// human cleared it rather than wanting the old one kept. Manual without an
// address is refused: saving it would leave login failing without a word
// about why.
func (s *settingsState) commitProxy() error {
	if s.proxy == nil {
		return nil
	}
	if s.proxyMode == proxyManual && trimSpace(s.proxyAddress) == "" {
		s.notice = pageNotice{kind: noticeSettingsProxyAddressEmpty}
		return auth.ErrNoProxyAddress
	}
	password := s.proxyPassword
	if s.proxyMode == proxyOff {
		password = ""
		s.proxyPassword = ""
	}
	s.proxy.Save(s.proxyMode, trimSpace(s.proxyAddress), trimSpace(s.proxyUser), password)
	s.notice = pageNotice{kind: noticeSettingsProxySaved, ok: true}
	return nil
}

// commitProxyOnLeave persists a draft only when it differs from what the seam
// holds, so browsing away from an untouched page writes nothing.
func (s *settingsState) commitProxyOnLeave() error {
	if s.hasProxyChanges() {
		return s.commitProxy()
	}
	return nil
}

func (s *settingsState) hasProxyChanges() bool {
	if s.proxy == nil {
		return false
	}
	if s.proxyMode != s.proxy.Mode() ||
		trimSpace(s.proxyAddress) != trimSpace(s.proxy.Address()) ||
		trimSpace(s.proxyUser) != trimSpace(s.proxy.User()) {
		return true
	}
	stored, _ := s.proxy.Password()
	if s.proxyMode == proxyOff {
		return stored != "" || s.proxyPassword != ""
	}
	return s.proxyPassword != stored
}

func proxyModeFromStored(value registry.ProxyMode) proxyMode {
	switch value {
	case registry.ProxyModeEnvironment:
		// Old GUI builds offered this mode. System mode still falls back to
		// the environment when no OS proxy is available.
		return proxySystem
	case registry.ProxyModeManual:
		return proxyManual
	case registry.ProxyModeOff:
		return proxyOff
	default:
		return proxySystem
	}
}

func (m proxyMode) authMode() auth.ProxyMode {
	switch m {
	case proxyManual:
		return auth.ProxyModeManual
	case proxyOff:
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
	Address() string
	User() string
	Password() (string, bool)
	// HasCredentials reports whether anything is stored, so a saved login
	// is visible without having to open the disclosure first.
	HasCredentials() bool
	SystemSupported() bool
	// system reports the OS setting auth uses in System mode.
	system() (auth.SystemProxy, bool)
	// Save writes the three preference values and the password, then drops the
	// pooled connections that may still point at the previous proxy.
	Save(mode proxyMode, address, user, password string)
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
func (p *runtimeProxyPref) Address() string { return p.pref.Address() }
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

func (p *runtimeProxyPref) Save(mode proxyMode, address, user, password string) {
	p.pref.Set(mode.stored(), trimSpace(address), trimSpace(user))
	if mode == proxyOff {
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
		setting := auth.ProxySetting{
			Mode: pref.Mode().authMode(),
			Addr: pref.Address(),
			User: pref.User(),
		}
		if password, ok := pref.Password(); ok {
			setting.Password = password
		}
		return setting
	}, pref.system)
}
