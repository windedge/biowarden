package ui

// logTailBytes is how much of the host log the page keeps: enough to show a
// recent unlock without loading a rotated multi-megabyte file.
const logTailBytes = 64 * 1024

// logState is the Log page's working data: a tail of the host's log file,
// already redacted by the host that wrote it.
type logState struct {
	backend settingsBackend
	text    string
	notice  pageNotice
}

// newLogState takes the shared settings backend; newAppState passes the same
// realSettings the Settings page uses, so both pages read one store instance.
func newLogState(backend settingsBackend) logState {
	s := logState{backend: backend}
	s.load()
	return s
}

// load replaces the view with a fresh tail. A missing file reads as empty, so
// the page shows its placeholder until the host has run.
func (s *logState) load() {
	s.notice = pageNotice{}
	if s.backend == nil {
		return
	}
	text, err := s.backend.logTail(logTailBytes)
	if err != nil {
		s.notice = pageNotice{kind: noticeLogReadFailed, err: err}
		return
	}
	s.text = text
}

func (s *logState) refresh() {
	s.load()
	if s.notice.text() == "" {
		s.notice = pageNotice{kind: noticeLogRefreshed, ok: true}
	}
}

// fakeLog stays for the prototype backend in backend_fake.go; the live page
// reads the real file through logState.load.
const fakeLog = `[2026-02-14 10:02:11.482] [INFO] host started: ` + appName + ` ` + appVersion + ` (windows/amd64)
[2026-02-14 10:02:11.483] [INFO] registry: 3 browsers known, 1 manifest owned by us
[2026-02-14 10:03:40.115] [INFO] native message: action=unlock userId=6f1c9a04...
[2026-02-14 10:03:40.116] [DEBUG] sdk request: {"keyB64":"<redacted>","encryptedString":"<redacted>","iv":"<redacted>","mac":"<redacted>"}
[2026-02-14 10:03:40.402] [INFO] secrets.Key(<redacted>) read for userId=6f1c9a04...
[2026-02-14 10:03:41.900] [INFO] hello: consent granted (windows hello, face)
[2026-02-14 10:03:41.905] [INFO] unlock ok userId=6f1c9a04...
[2026-02-14 10:05:02.010] [ERROR] hello: consent denied or cancelled (hresult=0x800704C7)
[2026-02-14 10:07:18.774] [INFO] manifest repair: firefox written (prototype)
`
