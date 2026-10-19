package ui

import (
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/windedge/biowarden/internal/hello"
	"github.com/windedge/biowarden/locales"
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
	// locale is the app's language, which the Language row reads and sets.
	locale localeBackend
	// languageTag is the Language row's choice: "" follows the system's
	// languages. mygo keeps the choice across launches and reports only the
	// language it shows, so newSettingsState guesses it back from those two.
	languageTag string

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
		locale:      mygoLocale{},
		helloResult: make(chan helloOutcome, 1),
		showTray:    true,
		logLevel:    "信息",
		logPath:     backend.logPath(),
	}
	s.languageTag = s.guessLanguageTag()
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

// languageOptions returns the Language row's choices: "" for the system's
// languages, then the languages the app is written in, each named in itself so
// a reader finds their own, as mygo's own language menu names them.
func (s *settingsState) languageOptions() (tags, labels []string) {
	tags, labels = []string{""}, []string{locales.LanguageSystem()}
	if s.locale == nil {
		return tags, labels
	}
	for _, tag := range s.locale.languages() {
		name := i18n.Autonym(tag)
		if name == "" {
			name = tag
		}
		tags, labels = append(tags, tag), append(labels, name)
	}
	return tags, labels
}

// languageSelect renders the Language row and reports the tag chosen this
// frame and whether the user chose one. The value bound to the control is the
// tag mygo SetLocale takes, so the row never has to map a display label back
// to a tag: the trigger and the options only show labels.
func (s *settingsState) languageSelect(c *ui.Context) (chosen string, changed bool) {
	tags, labels := s.languageOptions()
	chosen = s.languageTag
	t := c.Theme()
	sel := ui.SelectBase(c, &chosen)
	sel.Trigger.Gap(6).Padding(4, 10).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).
		Justify(ui.SpaceBetween).Width(settingsSelectWidth).
		Children(func() {
			ui.Text(c, s.languageLabelFor(chosen)).SingleLine()
			ui.Text(c, "▾").TextColor(t.TextMuted)
		})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(t.Radius+2).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		for i, tag := range tags {
			item := sel.Item(tag).Padding(6, 10).Radius(t.Radius)
			if item.Highlighted() {
				item.Background(t.Accent).TextColor(t.AccentText)
			}
			item.Children(func() { ui.Text(c, labels[i]).SingleLine() })
		}
	})
	return chosen, sel.Trigger.Changed()
}

// languageLabel names the choice the row shows.
func (s *settingsState) languageLabel() string {
	return s.languageLabelFor(s.languageTag)
}

// languageLabelFor names one choice, the first when the tag is unknown.
func (s *settingsState) languageLabelFor(tag string) string {
	tags, labels := s.languageOptions()
	for i, t := range tags {
		if t == tag {
			return labels[i]
		}
	}
	return labels[0]
}

// guessLanguageTag reports in which language the app shows a choice of its
// own: it follows the system's languages while it shows the one they ask for.
func (s *settingsState) guessLanguageTag() string {
	if s.locale == nil {
		return ""
	}
	if shown := s.locale.locale(); shown != "" && shown != systemLanguage(s.locale) {
		return shown
	}
	return ""
}

// setLanguage shows tag at once, "" for the system's languages again. mygo
// keeps it for the next launches, so the page stores nothing.
func (s *settingsState) setLanguage(tag string) {
	s.languageTag = tag
	if s.locale != nil {
		s.locale.setLocale(tag)
	}
}
