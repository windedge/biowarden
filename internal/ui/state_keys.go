package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// keysState is the Keys page's working data: the stored accounts, the entry
// form, and the fake verification in flight. The page ticket swaps the fake
// account list for the secrets seam and the fake verify for hello.
type keysState struct {
	accounts []*accountRow

	scriptBody string
	scriptOpen bool // the audit view of the extraction script starts collapsed

	userId string
	keyB64 string

	copiedAt time.Time

	verifying   bool
	verifyUntil time.Time

	notice   string
	noticeOK bool
}

type accountRow struct {
	userID  string
	email   string
	created string
}

func newKeysState() keysState {
	// Two accounts, both with the long userId the real credential store keys
	// on, so the row layout is judged at its real width.
	return keysState{
		scriptBody: extractSnippet,
		// scriptOpen stays false: the flow for a first-time user is the copy
		// button, and the curious can open the disclosure to read the script.
		accounts: []*accountRow{
			{userID: "6f1c9a04d2e84b7f9c3a1b8e5d0f2a7c", email: "ada@example.com", created: "2026-01-12 09:24"},
			{userID: "b27ae6f80c1d4a53b6e9d7c240f8a1b5", email: "grace@example.com", created: "2026-02-03 18:07"},
		},
	}
}

func (s *keysState) tick(c *ui.Context) {
	if s.verifying {
		if left := time.Until(s.verifyUntil); left > 0 {
			c.After(left)
		} else {
			s.finishVerify()
		}
	}
	if !s.copiedAt.IsZero() {
		if left := 3*time.Second - time.Since(s.copiedAt); left > 0 {
			c.After(left)
		} else {
			s.copiedAt = time.Time{}
		}
	}
}

// startVerify pretends to store the key and run one Windows Hello consent.
func (s *keysState) startVerify() {
	s.verifying = true
	s.verifyUntil = time.Now().Add(900 * time.Millisecond)
	s.notice = ""
}

func (s *keysState) finishVerify() {
	s.verifying = false
	id := strings.TrimSpace(s.userId)
	created := when(time.Now())
	// Re-saving an existing userId updates that row: duplicate keys would
	// otherwise share one element identity and confuse the rows.
	replaced := false
	for _, a := range s.accounts {
		if a.userID == id {
			a.created = created
			replaced = true
			break
		}
	}
	if !replaced {
		s.accounts = append(s.accounts, &accountRow{
			userID:  id,
			email:   "（原型：未提供邮箱）",
			created: created,
		})
	}
	s.notice = "密钥已保存，Windows Hello 验证通过（原型：未写入凭据管理器）"
	s.noticeOK = true
	s.userId, s.keyB64 = "", ""
}

func (s *keysState) removeAccount(target *accountRow) {
	out := s.accounts[:0]
	for _, a := range s.accounts {
		if a != target {
			out = append(out, a)
		}
	}
	s.accounts = out
	s.notice = "已删除该账户的密钥（原型：仅内存）"
	s.noticeOK = true
}

// extractSnippet mirrors the reference implementation's snippet: read the
// extension's session storage for the user key entry and print userId and
// keyB64 as JSON. The page ticket reviews it against the current extension.
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
