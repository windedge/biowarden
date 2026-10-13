package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// keysState is the Keys page's working data: the stored accounts, the entry
// form, and any backend call in flight. Backend calls run off the UI thread
// because Windows Hello blocks on a human, so their results come back through
// results as closures that only the UI thread applies.
type keysState struct {
	backend keysBackend

	accounts []*accountRow

	scriptBody string
	scriptOpen bool // the audit view of the extraction script starts collapsed

	userId string
	keyB64 string

	copiedAt time.Time

	busy    bool
	results chan func(*keysState)

	notice   string
	noticeOK bool
}

type accountRow struct {
	userID  string
	email   string
	created string
}

func newKeysState() keysState { return newKeysStateWith(newRealKeys()) }

func newKeysStateWith(backend keysBackend) keysState {
	s := keysState{
		backend:    backend,
		scriptBody: extractSnippet,
		results:    make(chan func(*keysState), 8),
	}
	// The first list must come from the credential store, not a seed, so the
	// page shows real accounts from the first frame.
	s.reload()
	return s
}

func (s *keysState) tick(c *ui.Context) {
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
func (s *keysState) async(work func() func(*keysState)) {
	if s.busy {
		return
	}
	s.busy = true
	s.notice = ""
	go func() { s.results <- work() }()
}

func (s *keysState) reload() {
	b := s.backend
	s.async(func() func(*keysState) {
		rows, err := b.accounts()
		available, availErr := b.helloAvailable()
		return func(st *keysState) { st.applyReload(rows, err, available, availErr) }
	})
}

func (s *keysState) applyReload(rows []accountRow, err error, available bool, availErr error) {
	switch {
	case err != nil:
		s.notice = "读取已保存账户失败：" + err.Error()
		s.noticeOK = false
	case availErr != nil:
		s.notice = "检查 Windows Hello 失败：" + availErr.Error()
		s.noticeOK = false
	case !available:
		s.notice = "此电脑未配置 Windows Hello，无法保存或删除密钥。"
		s.noticeOK = false
	}
	s.accounts = s.accounts[:0]
	for i := range rows {
		s.accounts = append(s.accounts, &rows[i])
	}
}

// saveKey decodes and stores the form's values behind one Hello consent. The
// backend picks up the trimmed values so a pasted newline does not spoil the
// key, and it runs off the UI thread because consent blocks.
func (s *keysState) saveKey() {
	b := s.backend
	id := strings.TrimSpace(s.userId)
	key := strings.TrimSpace(s.keyB64)
	s.async(func() func(*keysState) {
		err := b.save(id, key)
		return func(st *keysState) { st.applySave(err) }
	})
}

func (s *keysState) applySave(err error) {
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

func (s *keysState) testUnlock(acc *accountRow) {
	b := s.backend
	id := acc.userID
	s.async(func() func(*keysState) {
		err := b.verify("测试解锁 biowarden 密钥")
		return func(st *keysState) { st.applyUnlock(id, err) }
	})
}

func (s *keysState) applyUnlock(id string, err error) {
	if err != nil {
		s.notice = err.Error()
		s.noticeOK = false
		return
	}
	s.notice = "验证通过：Windows Hello 已确认身份（userId=" + shortID(id) + "）"
	s.noticeOK = true
}

func (s *keysState) removeAccount(target *accountRow) {
	b := s.backend
	id := target.userID
	s.async(func() func(*keysState) {
		err := b.remove(id)
		return func(st *keysState) { st.applyRemove(id, err) }
	})
}

func (s *keysState) applyRemove(id string, err error) {
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

// extractSnippet is the reference implementation's extraction script verbatim:
// read the extension's session storage for the user key entry and print
// userId and keyB64 as JSON. If a future extension update changes the
// storage layout, this is the first thing to re-check.
const extractSnippet = `// biowarden: extract the user key from the unlocked extension
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
