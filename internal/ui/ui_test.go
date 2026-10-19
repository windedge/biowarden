package ui

import (
	"testing"
)

// The tray menu is what the app set once, so it is set again when the language
// changes: RoleQuit relabels itself, the item that opens the window says the
// catalog's words again.
func TestTrayMenuFollowsTheLanguage(t *testing.T) {
	cases := []struct{ name, tag, want string }{
		{"english", "", "Open biowarden"},
		{"chinese", "zh-Hans", "打开 biowarden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appLanguage(t, tc.tag)
			items := trayMenu(nil).Items()
			if len(items) != 3 {
				t.Fatalf("the tray menu has %d items, want 3", len(items))
			}
			if items[0].Label != tc.want {
				t.Errorf("the tray menu says %q, want %q", items[0].Label, tc.want)
			}
		})
	}
}
