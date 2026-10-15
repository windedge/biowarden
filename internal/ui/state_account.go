package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// accountState is the Account page's working data: the login session the UI
// mirrors, the keys stored behind Windows Hello, the manual entry form, and
// any backend call in flight. Backend calls run off the UI thread because
// Windows Hello blocks on a human and a real login hits the network, so their
// results come back through results as closures that only the UI thread
// applies.
type accountState struct {
	keys    keysBackend
	account accountBackend

	// session mirrors accountBackend.session: the signed-in identity (or the
	// pending 2FA state) the account card draws.
	session accountSession

	// The login form is a UI-only disclosure: the button opens it, it does not
	// start a login by itself.
	loginOpen bool
	email     string
	password  string
	twoFACode string

	accounts []*accountRow
	// selectedUserID is the stored account the human picked to foreground; an
	// empty value lets primary fall back to the signed-in account, then the
	// first stored one.
	selectedUserID string

	userId string
	keyB64 string

	scriptBody string
	copiedAt   time.Time

	// advancedOpen hides the v1 escape hatches (the extraction script and the
	// manual paste form) behind one disclosure, so the normal account page
	// stays short. scriptOpen keeps the long script closed until asked for.
	advancedOpen bool
	scriptOpen   bool

	busy    bool
	results chan func(*accountState)

	notice   string
	noticeOK bool
}

type accountRow struct {
	userID  string
	email   string
	created string
}

// newAccountState wires the page onto the real credential store and the real
// account backend; newRealAccountState builds both on one shared store so the
// key card and the login path agree.
func newAccountState() accountState {
	return newRealAccountState()
}

// newAccountStateWith injects both seams so tests can drive login and key
// outcomes without a network or a Windows Hello prompt.
func newAccountStateWith(keys keysBackend, account accountBackend) accountState {
	s := accountState{
		keys:       keys,
		account:    account,
		scriptBody: extractSnippet,
		session:    accountSession{state: loginLoggedOut, serverURL: defaultServerURL},
		results:    make(chan func(*accountState), 8),
	}
	// The first list and session must come from the backends, not a seed, so
	// the page shows the real accounts from the first frame.
	s.reload()
	return s
}

func (s *accountState) tick(c *ui.Context) {
	select {
	case apply := <-s.results:
		s.busy = false
		apply(s)
	default:
	}
	if s.busy {
		// A backend call is still running; poll again so its result lands
		// even though the goroutine cannot touch the UI.
		c.After(60 * time.Millisecond)
	}
	if !s.copiedAt.IsZero() {
		if left := 3*time.Second - time.Since(s.copiedAt); left > 0 {
			c.After(left)
		} else {
			s.copiedAt = time.Time{}
		}
	}
}

// async runs work on its own goroutine and hands the UI a closure that applies
// the outcome. Only one call is allowed at a time: a second Hello prompt while
// the first is open would be confusing and pointless.
func (s *accountState) async(work func() func(*accountState)) {
	if s.busy {
		return
	}
	s.busy = true
	s.notice = ""
	go func() { s.results <- work() }()
}

// primary is the account the key status card describes: the one the human
// selected, otherwise the signed-in account, otherwise the first stored one.
func (s *accountState) primary() *accountRow {
	for _, want := range []string{s.selectedUserID, s.session.userID} {
		if want == "" {
			continue
		}
		for _, a := range s.accounts {
			if a.userID == want {
				return a
			}
		}
	}
	if len(s.accounts) == 0 {
		return nil
	}
	return s.accounts[0]
}

// selectAccount foregrounds one stored account, which is what "switching"
// means without parallel sessions: the key card and its actions follow it.
func (s *accountState) selectAccount(userID string) {
	s.selectedUserID = userID
}

func (s *accountState) reload() {
	keys, account := s.keys, s.account
	s.async(func() func(*accountState) {
		rows, err := keys.accounts()
		available, availErr := keys.helloAvailable()
		sess, sessErr := account.session()
		return func(st *accountState) {
			st.applyReload(rows, err, available, availErr, sess, sessErr)
		}
	})
}

func (s *accountState) applyReload(rows []accountRow, err error, available bool, availErr error, sess accountSession, sessErr error) {
	switch {
	case err != nil:
		s.notice = "读取已保存账户失败：" + err.Error()
		s.noticeOK = false
	case availErr != nil:
		s.notice = "检查 Windows Hello 失败：" + availErr.Error()
		s.noticeOK = false
	case sessErr != nil:
		s.notice = "读取登录状态失败：" + sessErr.Error()
		s.noticeOK = false
	case !available:
		s.notice = "此电脑未配置 Windows Hello，无法保存或删除密钥。"
		s.noticeOK = false
	}
	if sessErr == nil {
		s.session = sess
	}
	s.accounts = s.accounts[:0]
	for i := range rows {
		s.accounts = append(s.accounts, &rows[i])
	}
	// A selected account that no longer exists must not keep the card on a
	// stale row; falling back to primary's default is the honest state.
	if s.selectedUserID != "" {
		found := false
		for _, a := range s.accounts {
			if a.userID == s.selectedUserID {
				found = true
				break
			}
		}
		if !found {
			s.selectedUserID = ""
		}
	}
}

// saveKey decodes and stores the form's values behind one Hello consent. The
// backend picks up the trimmed values so a pasted newline does not spoil the
// key, and it runs off the UI thread because consent blocks.
func (s *accountState) saveKey() {
	keys := s.keys
	id := strings.TrimSpace(s.userId)
	key := strings.TrimSpace(s.keyB64)
	s.async(func() func(*accountState) {
		err := keys.save(id, key)
		return func(st *accountState) { st.applySave(err) }
	})
}

func (s *accountState) applySave(err error) {
	if err != nil {
		s.notice = err.Error()
		s.noticeOK = false
		return
	}
	s.notice = "密钥已保存，Windows Hello 验证通过。"
	s.noticeOK = true
	s.userId, s.keyB64 = "", ""
	s.reload()
}

// refreshKey asks the account backend to re-read the vault's key, then reloads
// so the card shows the new storage time.
func (s *accountState) refreshKey() {
	account := s.account
	s.async(func() func(*accountState) {
		err := account.refreshKey()
		return func(st *accountState) {
			if err != nil {
				st.notice = err.Error()
				st.noticeOK = false
				return
			}
			// Reload first: it clears the notice while starting its async
			// read, so the success line has to be set after it.
			st.reload()
			st.notice = "密钥已从账号刷新"
			st.noticeOK = true
		}
	})
}

func (s *accountState) testUnlock(acc *accountRow) {
	keys := s.keys
	id := acc.userID
	s.async(func() func(*accountState) {
		err := keys.verify("测试解锁 biowarden 密钥")
		return func(st *accountState) { st.applyUnlock(id, err) }
	})
}

func (s *accountState) applyUnlock(id string, err error) {
	if err != nil {
		s.notice = err.Error()
		s.noticeOK = false
		return
	}
	s.notice = "验证通过：Windows Hello 已确认身份（userId=" + shortID(id) + "）"
	s.noticeOK = true
}

func (s *accountState) removeAccount(target *accountRow) {
	keys := s.keys
	id := target.userID
	s.async(func() func(*accountState) {
		err := keys.remove(id)
		return func(st *accountState) { st.applyRemove(id, err) }
	})
}

func (s *accountState) applyRemove(id string, err error) {
	if err != nil {
		s.notice = err.Error()
		s.noticeOK = false
		return
	}
	out := s.accounts[:0]
	for _, a := range s.accounts {
		if a.userID != id {
			out = append(out, a)
		}
	}
	s.accounts = out
	s.notice = "已删除该账户的密钥。"
	s.noticeOK = true
}

// beginLogin opens the login form. It is a UI state change only: no call goes
// out until the form is submitted, and the fake backend answers that call with
// errAccountBackend until the auth ticket lands.
func (s *accountState) beginLogin() {
	s.loginOpen = true
	s.notice = ""
}

// cancelLogin abandons a pending login attempt. The backend returns the
// session that was in force before the attempt (the signed-in account after a
// cancelled relogin, signed out otherwise) and drops the cached credentials
// and 2FA challenge; the page clears the form. The call runs off the UI thread
// like every other backend action, and a failure is shown rather than hidden.
func (s *accountState) cancelLogin() {
	account := s.account
	s.async(func() func(*accountState) {
		sess, err := account.cancelLogin()
		return func(st *accountState) { st.applyCancel(sess, err) }
	})
}

func (s *accountState) applyCancel(sess accountSession, err error) {
	if err != nil {
		s.notice = err.Error()
		s.noticeOK = false
		return
	}
	s.session = sess
	s.loginOpen = false
	s.email, s.password, s.twoFACode = "", "", ""
	s.notice = "已取消登录"
	s.noticeOK = true
}

func (s *accountState) login() {
	account := s.account
	email, password := strings.TrimSpace(s.email), s.password
	serverURL := s.session.serverURL
	s.async(func() func(*accountState) {
		sess, err := account.login(email, password, serverURL)
		return func(st *accountState) { st.applySession(sess, err) }
	})
}

func (s *accountState) submit2FA() {
	account := s.account
	code := strings.TrimSpace(s.twoFACode)
	s.async(func() func(*accountState) {
		sess, err := account.submit2FA(code)
		return func(st *accountState) { st.applySession(sess, err) }
	})
}

func (s *accountState) logout() {
	account := s.account
	s.async(func() func(*accountState) {
		err := account.logout()
		return func(st *accountState) {
			if err != nil {
				st.notice = err.Error()
				st.noticeOK = false
				return
			}
			st.session = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
			st.loginOpen = false
			// Reload before the notice: reload clears it while starting its
			// async read, so the order keeps "已退出登录" on screen.
			st.reload()
			st.notice = "已退出登录"
			st.noticeOK = true
		}
	})
}

// relogin runs the whole flow again; the backend owns any prompts it needs.
func (s *accountState) relogin() {
	account := s.account
	s.async(func() func(*accountState) {
		err := account.relogin()
		return func(st *accountState) {
			if err != nil {
				st.notice = err.Error()
				st.noticeOK = false
				return
			}
			st.reload()
			st.notice = "已重新登录"
			st.noticeOK = true
		}
	})
}

// applySession lands a login outcome. A failure keeps the state the page
// draws and shows the backend's wording; a 2FA stop keeps the challenge so the
// prompt can render it, and the key is not written until the flow finishes.
func (s *accountState) applySession(sess accountSession, err error) {
	if err != nil {
		s.notice = err.Error()
		s.noticeOK = false
		return
	}
	s.session = sess
	switch sess.state {
	case loginLoggedIn:
		s.loginOpen = false
		s.password, s.twoFACode = "", ""
		// Reload before setting the notice: reload clears it while starting
		// its async read, so the order keeps the success line on screen.
		s.reload()
		s.notice = "已登录 " + sess.email
		s.noticeOK = true
	case loginNeeds2FA:
		s.notice = "需要两步验证码，请输入后继续"
		s.noticeOK = false
	case loginLoggingIn:
		s.notice = "正在登录..."
		s.noticeOK = false
	default:
		s.notice = ""
	}
}

// extractSnippet mirrors the reference implementation's script, updated for
// the 2026.8.x extension: state values are wrapped as {__json__,value}, the
// key may sit in session (unlocked) or in local under <userId>_user_auto
// ("never lock" policy), and a miss must tell the user why instead of
// printing nothing.
const extractSnippet = `// biowarden: 从已解锁的扩展里取出账号密钥（配合 GUI 账号页的高级选项录入）
// 前提：扩展处于已解锁状态。找不到时会打印原因。
(async () => {
  const uw = (v) => v && v.__json__ ? JSON.parse(v.value) : v;
  const found = [];
  for (const [k, v] of Object.entries(await chrome.storage.session.get(null))) {
    const m = k.match(/^user_(.+)_crypto_userKey$/);
    if (m) { const val = uw(v); found.push({ userId: m[1], keyB64: val.keyB64 ?? val }); }
  }
  if (!found.length)
    for (const [k, v] of Object.entries(await chrome.storage.local.get(null))) {
      const m = k.match(/^(.+)_user_auto$/);
      if (m) found.push({ userId: m[1], keyB64: uw(v) });
    }
  if (found.length) console.table(found);
  else console.log("未找到 userKey：扩展可能处于锁定状态（session 无记录），或未开启「从不锁定」策略（local 无 _user_auto 记录）。");
})();
`
