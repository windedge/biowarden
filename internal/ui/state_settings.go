package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/internal/hello"
)

var logLevels = []string{"错误", "信息", "调试"}

// helloOutcome is one background Available result, handed to tick through a
// channel so the blocking COM call never runs on the UI thread.
type helloOutcome struct {
	ok  bool
	err error
}

// settingsState is the Settings page's working data. The login item and the
// Hello status are real; the tray switch and the log level stay in memory.
type settingsState struct {
	backend settingsBackend
	hello   hello.Verifier

	launchAtLogin bool
	showTray      bool

	helloChecking bool
	helloOK       bool
	helloResult   chan helloOutcome

	logLevel string
	logPath  string

	notice   string
	noticeOK bool
}

func newSettingsState() settingsState {
	backend := realSettingsInstance()
	s := settingsState{
		backend:     backend,
		hello:       backend.Verifier,
		helloResult: make(chan helloOutcome, 1),
		showTray:    true,
		logLevel:    "信息",
		logPath:     backend.logPath(),
	}
	if on, err := backend.autostart(); err != nil {
		s.notice = "读取自启动设置失败：" + err.Error()
		s.noticeOK = false
	} else {
		s.launchAtLogin = on
	}
	// Ask once at startup so the page shows the machine's real Hello state.
	s.startHelloCheck()
	return s
}

// tick settles the background Hello check. The goroutine's result arrives on
// a channel; until then it keeps a frame scheduled so the page stops showing
// "checking" as soon as the answer lands.
func (s *settingsState) tick(c *ui.Context) {
	if !s.helloChecking {
		return
	}
	select {
	case r := <-s.helloResult:
		s.helloChecking = false
		s.helloOK = r.err == nil && r.ok
		switch {
		case r.err != nil:
			s.notice = "Windows Hello 检测失败：" + r.err.Error()
			s.noticeOK = false
		case r.ok:
			s.notice = "Windows Hello 可用"
			s.noticeOK = true
		default:
			s.notice = "Windows Hello 不可用：尚未配置人脸 / 指纹 / PIN"
			s.noticeOK = false
		}
	default:
		c.After(100 * time.Millisecond)
	}
}

func (s *settingsState) startHelloCheck() {
	if s.hello == nil || s.helloChecking {
		return
	}
	s.helloChecking = true
	verifier, done := s.hello, s.helloResult
	go func() {
		ok, err := verifier.Available()
		done <- helloOutcome{ok: ok, err: err}
	}()
}

// setAutostart writes the login item and puts the switch back when the write
// fails, so the page never shows an entry the OS refused.
func (s *settingsState) setAutostart() {
	if err := s.backend.setAutostart(s.launchAtLogin); err != nil {
		s.launchAtLogin = !s.launchAtLogin
		s.notice = "设置自启动失败：" + err.Error()
		s.noticeOK = false
		return
	}
	if s.launchAtLogin {
		s.notice = "已开启登录时自动启动"
	} else {
		s.notice = "已关闭登录时自动启动"
	}
	s.noticeOK = true
}
