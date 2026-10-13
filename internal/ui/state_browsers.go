package ui

import "github.com/egoist/mygo/ui"

// Manifest states of a browser, as the registry seam reports them.
const (
	browserMissing = iota // no manifest registered
	browserOurs           // our manifest
	browserForeign        // a manifest we did not write (likely the official client)
)

// browsersState is the Browsers page's working data. The prototype fills it
// with fake rows; the page ticket swaps in the registry seam.
type browsersState struct {
	rows []*browserRow

	// confirmCover and confirmUninstall are the row indexes (into rows)
	// awaiting a second confirming click; -1 when none. Overwriting or
	// deleting a manifest that may be the official client's needs the same
	// deliberateness as the reference's overwrite flow.
	confirmCover     int
	confirmUninstall int

	notice   string
	noticeOK bool

	reload func(c *ui.Context) // the page ticket replaces this with a real refresh
}

type browserRow struct {
	name  string // display name, from registry.Browsers
	path  string // manifest path currently registered, "" when none
	state int    // browserMissing / browserOurs / browserForeign
}

func newBrowsersState() browsersState {
	// Fake rows stand where the registry seam will stand: the three
	// supported browsers, one of them showing the foreign case this machine
	// really has (the official client's registration).
	return browsersState{
		confirmCover:     -1,
		confirmUninstall: -1,
		rows: []*browserRow{
			{name: "Chrome", path: `%LOCALAPPDATA%\biowarden\manifests\chrome.json`, state: browserOurs},
			{name: "Microsoft Edge", path: `C:\Users\<user>\scoop\apps\bitwarden\...\browsers\edge.json`, state: browserForeign},
			{name: "Firefox", path: "", state: browserMissing},
		},
	}
}

func (s *browsersState) tick(c *ui.Context) {
	if s.reload != nil {
		s.reload(c)
	}
}
