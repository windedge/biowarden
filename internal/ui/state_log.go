package ui

// logState is the Log page's working data: a tail of the host's log file,
// redacted. The page ticket swaps the fake text for the real file tail.
type logState struct {
	text     string
	notice   string
	noticeOK bool
}

func newLogState() logState {
	// Fake lines stand where the real file tail will, including the redacted
	// forms the host logger must produce.
	return logState{text: fakeLog}
}

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
