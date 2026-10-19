package ui

import (
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/locales"
)

// accountState is the Account page's working data: the login session the UI
// mirrors, the keys stored behind Windows Hello, the manual entry form, and
// any backend call in flight. Backend calls run off the UI thread because
// Windows Hello blocks on a human and a real login hits the network, so their
// results come back through results as closures that only the UI thread
// applies. The login window drives the same backend through loginWindow, and
// lands its outcome on this state: the session, the reload and the notice.
type accountState struct {
	keys    keysBackend
	account accountBackend

	// session mirrors accountBackend.session: the signed-in identity (or the
	// pending 2FA state) the account card draws.
	session accountSession

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

	// verified records that the first load has already dealt with the
	// persisted login: either restoreSession settled it, or there was nothing
	// stored to verify. Later reloads (after a save, logout, ...) only read
	// the in-memory session and must not dial the server again.
	verified bool
	// verifying is true while the startup restore is still on the wire. The
	// card shows a weak "checking" hint and tick keeps a frame coming for its
	// result.
	verifying bool
	// restoreResult carries the finished startup restore on its own lane, so a
	// slow server cannot hold results, which the Hello and key calls share.
	restoreResult chan func(*accountState)

	busy    bool
	results chan func(*accountState)

	notice pageNotice
}

type accountRow struct {
	userID  string
	email   string
	created string
	// serverURL is the address the account signed in against, from its stored
	// metadata; empty for entries written before the field existed.
	serverURL string
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
		keys:          keys,
		account:       account,
		scriptBody:    extractSnippet,
		session:       accountSession{state: loginLoggedOut, serverURL: defaultServerURL},
		results:       make(chan func(*accountState), 8),
		restoreResult: make(chan func(*accountState), 1),
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
	// The startup restore lands on its own channel, so a slow server cannot
	// hold the results lane the Hello and key calls share.
	select {
	case apply := <-s.restoreResult:
		apply(s)
	default:
	}
	if s.busy || s.verifying {
		// A backend call - or the startup restore - is still running; poll
		// again so its result lands even though the goroutine cannot touch
		// the UI.
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
	s.notice = pageNotice{}
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

// effectiveServerURL is the address the UI shows when it is not tied to one
// live session: the session's address, else the primary stored account's
// metadata, else the official cloud. A stored entry from before the metadata
// carried an address has none, which falls through to the default.
func (s *accountState) effectiveServerURL() string {
	if v := strings.TrimSpace(s.session.serverURL); v != "" {
		return v
	}
	if p := s.primary(); p != nil {
		if v := strings.TrimSpace(p.serverURL); v != "" {
			return v
		}
	}
	return defaultServerURL
}

// selectAccount foregrounds one stored account, which is what "switching"
// means without parallel sessions: the key card and its actions follow it.
func (s *accountState) selectAccount(userID string) {
	s.selectedUserID = userID
}

// reload reads the local state synchronously - the credential store, the
// Hello check and the in-memory session all answer without the network - so
// the stored keys render in the first frame. The first call also re-checks any
// persisted login in the background: that restore lands on its own lane and
// updates only the session fields, never the rows already on screen.
func (s *accountState) reload() {
	rows, err := s.keys.accounts()
	available, availErr := s.keys.helloAvailable()
	sess, sessErr := s.account.session()
	s.applyLocal(rows, err, available, availErr)
	s.applySession(sess, sessErr)

	if s.verified {
		return
	}
	// The persisted login is checked exactly once, so a later save or logout
	// never dials the server again.
	s.verified = true
	s.verifying = true
	account := s.account
	go func() {
		sess, sessErr := account.restoreSession()
		s.restoreResult <- func(st *accountState) {
			st.verifying = false
			st.applySession(sess, sessErr)
		}
	}()
}

// applyReload lands a full reload answer, the local stage and the session
// stage in the order the caller already holds them.
func (s *accountState) applyReload(rows []accountRow, err error, available bool, availErr error, sess accountSession, sessErr error) {
	s.applyLocal(rows, err, available, availErr)
	s.applySession(sess, sessErr)
}

// applyLocal installs what a reload read without the network: the stored rows,
// the Hello availability and the selection. It renders in the first frame,
// before the startup restore has answered.
func (s *accountState) applyLocal(rows []accountRow, err error, available bool, availErr error) {
	switch {
	case err != nil:
		s.notice = pageNotice{kind: noticeAccountKeysReadFailed, err: err}
	case availErr != nil:
		s.notice = pageNotice{kind: noticeAccountHelloCheckFailed, err: availErr}
	case !available:
		s.notice = pageNotice{kind: noticeAccountHelloUnavailable}
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

// applySession installs the login state and its notice. It runs on its own
// when the startup restore answers off the UI thread, so it never touches the
// rows already on screen.
func (s *accountState) applySession(sess accountSession, sessErr error) {
	switch {
	case errors.Is(sessErr, errSessionRevoked):
		s.notice = pageNotice{kind: noticeAccountSessionRevoked}
	case errors.Is(sessErr, errSessionUnverified):
		s.notice = pageNotice{kind: noticeAccountSessionUnverified}
	case sessErr != nil:
		s.notice = pageNotice{kind: noticeAccountSessionReadFailed, err: sessErr}
	}
	if sessErr == nil || errors.Is(sessErr, errSessionRevoked) || errors.Is(sessErr, errSessionUnverified) {
		s.session = sess
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
		s.notice = failureNotice(err)
		return
	}
	s.notice = pageNotice{kind: noticeAccountKeySaved, ok: true}
	s.userId, s.keyB64 = "", ""
	s.reload()
}

func (s *accountState) testUnlock(acc *accountRow) {
	keys := s.keys
	id := acc.userID
	s.async(func() func(*accountState) {
		err := keys.verify(locales.HelloReasonTestUnlock())
		return func(st *accountState) { st.applyUnlock(id, err) }
	})
}

func (s *accountState) applyUnlock(id string, err error) {
	if err != nil {
		s.notice = failureNotice(err)
		return
	}
	s.notice = pageNotice{kind: noticeAccountVerified, args: []string{shortID(id)}, ok: true}
}

func (s *accountState) removeAccount(target *accountRow) {
	keys := s.keys
	account := s.account
	id := target.userID
	s.async(func() func(*accountState) {
		err := keys.remove(id)
		var clearErr error
		if err == nil {
			// The key is gone, so the persisted login for it must go too:
			// otherwise a restart would restore the session and show the
			// account as signed in with no key.
			clearErr = account.forgetSession(id)
		}
		return func(st *accountState) { st.applyRemove(id, err, clearErr) }
	})
}

func (s *accountState) applyRemove(id string, err, clearErr error) {
	if err != nil {
		s.notice = failureNotice(err)
		return
	}
	out := s.accounts[:0]
	for _, a := range s.accounts {
		if a.userID != id {
			out = append(out, a)
		}
	}
	s.accounts = out
	if clearErr != nil {
		// The key is deleted either way; only the session cleanup failed.
		s.notice = pageNotice{kind: noticeAccountRemoveSessionFailed, err: clearErr}
		return
	}
	s.notice = pageNotice{kind: noticeAccountKeyRemoved, ok: true}
}

func (s *accountState) logout() {
	account := s.account
	s.async(func() func(*accountState) {
		err := account.logout()
		return func(st *accountState) {
			if err != nil {
				st.notice = failureNotice(err)
				return
			}
			st.session = accountSession{state: loginLoggedOut, serverURL: defaultServerURL}
			// The persisted login was just deleted on purpose; the card must
			// not wait for any restore that could still be on the wire.
			st.verified = true
			// Reload first: a local read error it reports must not clobber
			// the logged-out line, so the line is set after it.
			st.reload()
			st.notice = pageNotice{kind: noticeAccountLoggedOut, ok: true}
		}
	})
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
