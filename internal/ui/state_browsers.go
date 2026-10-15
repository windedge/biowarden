package ui

import (
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/registry"
)

// Manifest states of a browser, as the registry seam reports them.
const (
	browserMissing  = iota // no manifest registered
	browserOurs            // our manifest
	browserForeign         // a manifest we did not write (likely the official client)
	browserHijacked        // our manifest file exists, but the key points elsewhere
)

// browsersState is the Browsers page's working data, backed by the registry
// seam.
type browsersState struct {
	backend browsersBackend

	rows []browserRow

	// confirmCover and confirmUninstall are the row indexes (into rows)
	// awaiting a second confirming click; -1 when none. Overwriting or
	// deleting a manifest that may be the official client's needs the same
	// deliberateness as the reference's overwrite flow.
	confirmCover     int
	confirmUninstall int

	notice   string
	noticeOK bool
}

type browserRow struct {
	browser registry.Browser // the registration point install/uninstall target
	name    string           // display name, from registry.Browsers
	path    string           // manifest path currently registered, "" when none
	state   int              // browserMissing / browserOurs / browserForeign / browserHijacked

	// connected reports whether this browser's extension has ever reached the
	// host. It is a placeholder for a data source that does not exist yet:
	// nothing on the host side records connections, so every backend leaves it
	// false and the row reads "扩展未连接". Wire it when the host keeps a
	// per-browser last-seen.
	connected bool
}

func newBrowsersState() browsersState {
	return newBrowsersStateWith(newRealBrowsers(nil))
}

// newBrowsersStateWith is the injectable form; tests pass an in-memory
// backend, the app builds the registry-backed one.
func newBrowsersStateWith(backend browsersBackend) browsersState {
	s := browsersState{
		backend:          backend,
		confirmCover:     -1,
		confirmUninstall: -1,
	}
	s.reload()
	return s
}

// tick refreshes on every rebuild, so reopening or switching to the page shows
// the registry as it is now rather than what it was when the app started.
func (s *browsersState) tick(c *ui.Context) {
	s.reload()
}

// reload re-reads every browser's manifest state.
func (s *browsersState) reload() {
	rows, err := s.backend.refresh()
	if err != nil {
		s.notice = "读取浏览器注册状态失败：" + err.Error()
		s.noticeOK = false
		return
	}
	s.rows = rows
}

// installAll writes our manifest for every browser we may touch: rows with no
// registration, and rows already ours (a repair). A foreign or hijacked value
// is left to the per-row confirmation, because one click should not decide to
// overwrite another program's registration.
func (s *browsersState) installAll() {
	installed, skipped := 0, 0
	var failed []string
	for i := range s.rows {
		switch s.rows[i].state {
		case browserMissing, browserOurs:
			if err := s.backend.install(s.rows[i]); err != nil {
				failed = append(failed, s.rows[i].name)
				continue
			}
			installed++
		default:
			skipped++
		}
	}
	s.reload()
	// One notice carries all three facts, so a failure never hides the
	// rows the bulk install skipped for a per-row confirmation.
	parts := []string{"已安装 " + itoa(installed) + " 个"}
	if len(failed) > 0 {
		parts = append(parts, "失败："+strings.Join(failed, "、"))
	}
	if skipped > 0 {
		parts = append(parts, itoa(skipped)+" 个需逐行确认（可能属于官方客户端）")
	}
	s.notice = strings.Join(parts, "；")
	s.noticeOK = len(failed) == 0 && skipped == 0
}

// install writes our manifest for the row, replacing whatever was registered.
func (s *browsersState) install(row *browserRow) {
	if err := s.backend.install(*row); err != nil {
		s.notice = "写入 " + row.name + " 的 manifest 失败：" + err.Error()
		s.noticeOK = false
		return
	}
	s.reload()
	s.notice = "已为 " + row.name + " 写入 manifest"
	s.noticeOK = true
}

// uninstall removes our manifest, unless the registration is foreign: then the
// registry leaves it alone and the page says so.
func (s *browsersState) uninstall(row *browserRow) {
	err := s.backend.uninstall(*row)
	s.reload()
	switch {
	case errors.Is(err, errForeignRegistration):
		s.notice = "未删除 " + row.name + " 的注册：它可能属于官方 Bitwarden 桌面客户端"
		s.noticeOK = false
	case err != nil:
		s.notice = "删除 " + row.name + " 的 manifest 失败：" + err.Error()
		s.noticeOK = false
	default:
		s.notice = "已删除 " + row.name + " 的 manifest"
		s.noticeOK = true
	}
}
