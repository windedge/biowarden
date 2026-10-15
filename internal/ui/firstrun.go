package ui

import (
	"log"
	"strings"

	"github.com/windedge/biowarden/internal/registry"
)

// firstRunBackend is what the zero-configuration first launch needs from the
// world: the marker that keeps the setup to one run, the stored-key count that
// decides whether this is a first launch at all, and the registration seam the
// Browsers page writes through. It is an interface so the flow can be driven
// without a registry, a credential store, or a window server.
type firstRunBackend interface {
	// done reports whether an earlier launch already completed the setup.
	done() (bool, error)
	// mark records that this launch completed it.
	mark() error
	// storedKeys counts the keys already in the credential store.
	storedKeys() (int, error)
	// installed lists the browsers found on this machine, in Browsers order.
	installed() []registry.Browser
	// status reports one browser's manifest state.
	status(registry.Browser) (registry.Status, error)
	// install writes our manifest for the browser; it is the same call the
	// Browsers page makes, so there is one write path.
	install(registry.Browser) error
}

// firstRunState is the launch-time zero-configuration setup: no key is stored
// and no browser points at biowarden, so the app configures itself instead of
// asking the human to visit the Browsers page first. It runs at most once per
// process and, through the marker, at most once per installation.
type firstRunState struct {
	backend firstRunBackend
	// ran records the attempt inside this process, so a failed marker write
	// cannot make the same launch register twice or open a second login
	// window.
	ran bool
}

func newFirstRunState(backend firstRunBackend) firstRunState {
	return firstRunState{backend: backend}
}

// needsFirstRun is the zero-configuration test: nothing stored and no browser
// registered to us. A registration that belongs to another client does not
// count as ours, so a machine already running the official desktop client is
// still a first run for biowarden and gets its own setup.
func needsFirstRun(storedKeys int, oursRegistered bool) bool {
	return storedKeys == 0 && !oursRegistered
}

// anyRegisteredOurs reports whether any supported browser already points at
// our manifest. A read that fails counts as "not ours": a probe error must
// never be mistaken for an existing setup.
func anyRegisteredOurs(status func(registry.Browser) (registry.Status, error)) bool {
	for _, b := range registry.Browsers {
		st, err := status(b)
		if err != nil {
			continue
		}
		if st.IsOurs {
			return true
		}
	}
	return false
}

// firstRunResult is what the setup did, in display names, ready for the one
// summary line the Browsers page keeps.
type firstRunResult struct {
	registered []string // our manifest was written
	skipped    []string // installed, but the registration may belong to another client
	failed     []string // the status read or the write failed
	absent     []string // not installed on this machine
	markErr    error    // the marker write failed; the setup itself still stands
}

// notice renders the summary, and ok is false whenever a human has something
// to look at: a skipped registration, a failed step, or a lost marker.
func (r firstRunResult) notice() (string, bool) {
	parts := []string{"首启自动配置：已注册 " + itoa(len(r.registered)) + " 个浏览器"}
	if len(r.registered) > 0 {
		parts[0] += "（" + strings.Join(r.registered, "、") + "）"
	}
	if len(r.skipped) > 0 {
		parts = append(parts, "跳过 "+itoa(len(r.skipped))+" 个（注册可能属于官方客户端，未覆盖："+
			strings.Join(r.skipped, "、")+"）")
	}
	if len(r.failed) > 0 {
		parts = append(parts, "失败："+strings.Join(r.failed, "、"))
	}
	if len(r.absent) > 0 {
		parts = append(parts, "未安装："+strings.Join(r.absent, "、"))
	}
	if r.markErr != nil {
		parts = append(parts, "首启标记异常："+r.markErr.Error())
	}
	ok := len(r.failed) == 0 && len(r.skipped) == 0 && r.markErr == nil
	return strings.Join(parts, "；"), ok
}

// setup performs the one-time setup and reports what happened. The boolean is
// false when this launch has nothing to do: the marker says an earlier launch
// already ran, the machine is not zero-configuration, or the stored-key count
// could not be read (guessing there could overwrite a working setup).
//
// Every failure along the way is folded into the result instead of returning
// an error: registering a browser is a convenience, and the human still has to
// log in either way.
func (s firstRunState) setup() (firstRunResult, bool) {
	var result firstRunResult

	// A failed marker read is treated as "not done yet" and reported: the
	// setup is safe to repeat, because it never touches a registration that is
	// not ours.
	done, err := s.backend.done()
	if err != nil {
		result.markErr = err
	}
	if done {
		return result, false
	}

	stored, err := s.backend.storedKeys()
	if err != nil {
		return result, false
	}
	if !needsFirstRun(stored, anyRegisteredOurs(s.backend.status)) {
		return result, false
	}

	installed := make(map[registry.Browser]bool)
	for _, b := range s.backend.installed() {
		installed[b] = true
	}
	// Walk Browsers order, not probe order, so the summary is stable.
	for _, b := range registry.Browsers {
		if installed[b] {
			continue
		}
		result.absent = append(result.absent, b.String())
	}
	for _, b := range registry.Browsers {
		if !installed[b] {
			continue
		}
		st, err := s.backend.status(b)
		switch {
		case err != nil:
			result.failed = append(result.failed, b.String())
		case st.IsOurs:
			// Already ours: nothing to write and nothing to warn about.
		case st.Registered:
			// The value may be the official desktop client's. A silent setup
			// must not overwrite it; that needs the per-row confirmation on
			// the Browsers page.
			result.skipped = append(result.skipped, b.String())
		default:
			if err := s.backend.install(b); err != nil {
				result.failed = append(result.failed, b.String())
				continue
			}
			result.registered = append(result.registered, b.String())
		}
	}

	// Recording the run comes last: it must describe a setup that happened,
	// not one that died halfway. A failed write supersedes an earlier read
	// failure, since the value on disk is what the next launch reads.
	if err := s.backend.mark(); err != nil {
		result.markErr = err
	}
	return result, true
}

// runFirstRun is the launch-time entry point: the one-time setup, then the
// login window. It is guarded so a launch at most registers once and at most
// opens one login window, and a machine that is already configured is left
// completely alone.
func (app *appState) runFirstRun() {
	if app.firstRun.ran {
		return
	}
	app.firstRun.ran = true

	result, firstRun := app.firstRun.setup()
	if !firstRun {
		return
	}
	app.browsers.notice, app.browsers.noticeOK = result.notice()
	if notice := app.browsers.notice; notice != "" {
		log.Println("biowarden: " + notice)
	}
	// The login window opens whatever the setup did above: unlocking never
	// depended on the browser registration.
	app.login.begin()
}
