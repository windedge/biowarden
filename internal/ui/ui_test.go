package ui

import (
	"testing"
)

// The tray menu is what the app set once, so it is set again when the language
// changes: RoleQuit relabels itself, the item that opens the window says the
// catalog's words again.
//
// The item's label ends with a space: MyGo sizes a menu item from the width
// of its text, but Windows draws that text a few pixels wider and cuts what
// does not fit - the cut fell on the app name's last letter. The space gives
// the text the room it needs, and the menu shows it as nothing.
func TestTrayMenuFollowsTheLanguage(t *testing.T) {
	cases := []struct{ name, tag, want string }{
		{"english", "", "Open biowarden "},
		{"chinese", "zh-Hans", "打开 biowarden "},
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
