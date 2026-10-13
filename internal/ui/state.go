package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func itoa(n int) string { return strconv.Itoa(n) }

// protoState is the prototype's whole state. Everything is fake and lives in
// memory: no credential store, no registry, no log file. A restart therefore
// shows the same sample rows, and a human can click every branch while the
// interactions and the layout are still open for confirmation.
type protoState struct {
	tab int

	browsers         []*protoBrowser
	confirmCover     int // which row awaits the overwrite confirmation; -1 when none
	confirmUninstall int // which row awaits the uninstall confirmation; -1 when none
	browserNotice    string
	browserNoticeOK  bool

	accounts     []*protoAccount
	scriptBody   string
	userId       string
	keyB64       string
	copiedAt     time.Time
	verifying    bool
	verifyUntil  time.Time
	keysNotice   string
	keysNoticeOK bool

	launchAtLogin  bool
	showTray       bool
	helloChecking  bool
	helloUntil     time.Time
	helloOK        bool
	settingsNotice string
	settingsOK     bool
	logLevel       string
	logPath        string

	logText   string
	logNotice string
	logOK     bool
}

// Manifest states of a browser, as the reference detects them.
const (
	browserMissing = iota // no manifest
	browserOurs           // our manifest
	browserForeign        // a manifest we did not write (likely the official client)
)

type protoBrowser struct {
	name  string
	path  string
	state int
}

type protoAccount struct {
	userID  string
	email   string
	created string
}

var logLevels = []string{"错误", "信息", "调试"}

func newProtoState() *protoState {
	return &protoState{
		confirmCover:     -1,
		confirmUninstall: -1,
		launchAtLogin:    true,
		showTray:         true,
		helloOK:          true,
		logLevel:         "信息",
		logPath:          `%LOCALAPPDATA%\biowarden\host.log`,
		// Windows manifest locations, so the paths look like the real ones
		// while the registry seam stays untouched.
		browsers: []*protoBrowser{
			{
				name:  "Chrome",
				path:  `%LOCALAPPDATA%\Google\Chrome\User Data\NativeMessagingHosts\com.8bit.bitwarden.json`,
				state: browserOurs,
			},
			{
				name:  "Microsoft Edge",
				path:  `%LOCALAPPDATA%\Microsoft\Edge\User Data\NativeMessagingHosts\com.8bit.bitwarden.json`,
				state: browserForeign,
			},
			{
				name:  "Firefox",
				path:  `%APPDATA%\Mozilla\NativeMessagingHosts\com.8bit.bitwarden.json`,
				state: browserMissing,
			},
		},
		// Two accounts, both with the long userId the real credential store
		// keys on, so the row layout is judged at its real width.
		accounts: []*protoAccount{
			{userID: "6f1c9a04d2e84b7f9c3a1b8e5d0f2a7c", email: "ada@example.com", created: "2026-01-12 09:24"},
			{userID: "b27ae6f80c1d4a53b6e9d7c240f8a1b5", email: "grace@example.com", created: "2026-02-03 18:07"},
		},
		scriptBody: extractSnippet,
		logText:    fakeLog,
	}
}

// tick advances the prototype's simulated work and asks for the frame that
// finishes it. Every page calls it first, so a pending action still settles
// if the human switches tabs before it ends.
func (app *protoState) tick(c *ui.Context) {
	if app.verifying {
		if left := time.Until(app.verifyUntil); left > 0 {
			c.After(left)
		} else {
			app.finishVerify()
		}
	}
	if app.helloChecking {
		if left := time.Until(app.helloUntil); left > 0 {
			c.After(left)
		} else {
			app.helloChecking = false
			app.helloOK = true
			app.settingsNotice = "Windows Hello 可用（原型：固定返回可用）"
			app.settingsOK = true
		}
	}
	if !app.copiedAt.IsZero() {
		if left := 3*time.Second - time.Since(app.copiedAt); left > 0 {
			c.After(left)
		} else {
			app.copiedAt = time.Time{}
		}
	}
}

// startVerify pretends to store the key and run one Windows Hello consent.
func (app *protoState) startVerify() {
	app.verifying = true
	app.verifyUntil = time.Now().Add(900 * time.Millisecond)
	app.keysNotice = ""
}

func (app *protoState) finishVerify() {
	app.verifying = false
	id := strings.TrimSpace(app.userId)
	created := time.Now().Format("2006-01-02 15:04")
	// Re-saving an existing userId updates that row: duplicate keys would
	// otherwise share one element identity and confuse the rows.
	replaced := false
	for _, a := range app.accounts {
		if a.userID == id {
			a.created = created
			replaced = true
			break
		}
	}
	if !replaced {
		app.accounts = append(app.accounts, &protoAccount{
			userID:  id,
			email:   "（原型：未提供邮箱）",
			created: created,
		})
	}
	app.keysNotice = "密钥已保存，Windows Hello 验证通过（原型：未写入凭据管理器）"
	app.keysNoticeOK = true
	app.userId, app.keyB64 = "", ""
}

func (app *protoState) removeAccount(target *protoAccount) {
	out := app.accounts[:0]
	for _, a := range app.accounts {
		if a != target {
			out = append(out, a)
		}
	}
	app.accounts = out
	app.keysNotice = "已删除该账户的密钥（原型：仅内存）"
	app.keysNoticeOK = true
}

func (app *protoState) startHelloCheck() {
	app.helloChecking = true
	app.helloUntil = time.Now().Add(700 * time.Millisecond)
	app.settingsNotice = ""
}

// notice renders a page's status line: orange for what still needs a human,
// green for a simulated success. Empty text draws nothing, so a settled page
// keeps its layout.
func notice(c *ui.Context, text string, ok bool) {
	if text == "" {
		return
	}
	col := c.Theme().Warning
	if ok {
		col = c.Theme().Success
	}
	ui.Text(c, text).FontSize(12).TextColor(col)
}

func muted(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted)
}

// groupBox is the prototype's stand-in for the reference's GroupBox: visible
// grouping without a real form layout behind it.
func groupBox(c *ui.Context, title string, body func()) {
	t := c.Theme()
	ui.Column(c).Padding(12).Gap(8).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, title).Bold()
		body()
	})
}

// shortID keeps a long userId from pushing the row buttons out of view.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

// extractSnippet is invented for the prototype. It is not the script the
// released tool ships; it is here so the copy button and the paste-location
// instructions have something concrete behind them.
const extractSnippet = `// biowarden prototype snippet - not the shipped script
chrome.storage.session.get(null, d => {
  for (const [k, v] of Object.entries(d)) {
    const m = k.match(/^user_([^_]+)_crypto_userKey$/);
    if (m) {
      const inner = JSON.parse(v.value ?? "{}");
      const entry = inner[""] ?? Object.values(inner)[0];
      console.log(JSON.stringify({ userId: m[1], keyB64: entry.keyB64 }));
    }
  }
});
`

const fakeLog = `[2026-02-14 10:02:11.482] [INFO] host started: biowarden 0.1.0 (windows/amd64)
[2026-02-14 10:02:11.483] [INFO] registry: 3 browsers known, 1 manifest owned by us
[2026-02-14 10:03:40.115] [INFO] native message: action=unlock userId=6f1c9a04...
[2026-02-14 10:03:40.116] [DEBUG] sdk request: {"keyB64":"<redacted>","encryptedString":"<redacted>","iv":"<redacted>","mac":"<redacted>"}
[2026-02-14 10:03:40.402] [INFO] secrets.Key(<redacted>) read for userId=6f1c9a04...
[2026-02-14 10:03:41.900] [INFO] hello: consent granted (windows hello, face)
[2026-02-14 10:03:41.905] [INFO] unlock ok userId=6f1c9a04...
[2026-02-14 10:05:02.010] [ERROR] hello: consent denied or cancelled (hresult=0x800704C7)
[2026-02-14 10:07:18.774] [INFO] manifest repair: firefox written (prototype)
`
