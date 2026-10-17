package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/windedge/biowarden/internal/registry"
)

// The dialog must name every registration it would take over, whose value is
// there now, and the biowarden manifest that would replace it. The foreign and
// hijacked rows get different wording, which is the whole point of confirming
// them together without pretending they carry the same risk.
func TestBrowsersConfirmViewListsTakeovers(t *testing.T) {
	s := &browsersState{bulk: []browserRow{
		{
			browser:     registry.Edge,
			name:        "Edge",
			path:        `C:\official\edge.json`,
			installPath: `C:\biowarden\manifests\edge.json`,
			state:       browserForeign,
		},
		{
			browser:     registry.Firefox,
			name:        "Firefox",
			path:        `C:\other\firefox.json`,
			installPath: `C:\biowarden\manifests\firefox.json`,
			state:       browserHijacked,
		},
	}}
	tester := ui.NewTester(s.confirmView, browsersConfirmWidth, browsersConfirmHeight)
	tester.Frame()

	for _, want := range []string{
		"Edge · 接管此前属于其他程序的注册",
		"Firefox · 恢复 biowarden 的配置",
		"当前指向：C:\\official\\edge.json（可能是官方 Bitwarden 桌面端）",
		"当前指向：C:\\other\\firefox.json（biowarden 的配置已被它覆盖）",
		"覆盖后指向：C:\\biowarden\\manifests\\edge.json",
		"恢复为：C:\\biowarden\\manifests\\firefox.json",
		"确认覆盖",
		"取消",
	} {
		if !tester.HasText(want) {
			t.Errorf("confirm dialog is missing %q; texts = %v", want, tester.Texts())
		}
	}
}

// A hijacked-only list is a recovery, not a takeover of someone else's
// registration, so the primary button and the intro say so.
func TestBrowsersConfirmViewHijackedOnlyReadsAsRecovery(t *testing.T) {
	s := &browsersState{bulk: []browserRow{{
		browser:     registry.Firefox,
		name:        "Firefox",
		path:        `C:\other\firefox.json`,
		installPath: `C:\biowarden\manifests\firefox.json`,
		state:       browserHijacked,
	}}}
	if got := s.confirmLabel(); got != "确认恢复" {
		t.Fatalf("confirmLabel = %q, want the recovery wording", got)
	}
	if s.hasForeign() {
		t.Fatalf("hasForeign = true for a hijacked-only list")
	}
	tester := ui.NewTester(s.confirmView, browsersConfirmWidth, browsersConfirmHeight)
	tester.Frame()
	if !tester.HasText("恢复为：C:\\biowarden\\manifests\\firefox.json") {
		t.Errorf("dialog is missing the recovery target; texts = %v", tester.Texts())
	}
	if tester.HasText("官方 Bitwarden 桌面端每次启动") {
		t.Errorf("hijacked-only dialog shows the foreign-client warning: %v", tester.Texts())
	}
}
