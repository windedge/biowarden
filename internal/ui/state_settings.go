package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

var logLevels = []string{"错误", "信息", "调试"}

// settingsState is the Settings page's working data. The page ticket swaps
// the fake switches for the autostart seam and hello.
type settingsState struct {
	launchAtLogin bool
	showTray      bool

	helloChecking bool
	helloUntil    time.Time
	helloOK       bool

	logLevel string
	logPath  string

	notice   string
	noticeOK bool
}

func newSettingsState() settingsState {
	return settingsState{
		launchAtLogin: true,
		showTray:      true,
		helloOK:       true,
		logLevel:      "信息",
		logPath:       `%LOCALAPPDATA%\biowarden\host.log`,
	}
}

func (s *settingsState) tick(c *ui.Context) {
	if s.helloChecking {
		if left := time.Until(s.helloUntil); left > 0 {
			c.After(left)
		} else {
			s.helloChecking = false
			s.helloOK = true
			s.notice = "Windows Hello 可用（原型：固定返回可用）"
			s.noticeOK = true
		}
	}
}

func (s *settingsState) startHelloCheck() {
	s.helloChecking = true
	s.helloUntil = time.Now().Add(700 * time.Millisecond)
	s.notice = ""
}
