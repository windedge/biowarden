package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// accountPage is the landing page: who is signed in, whether a key is stored,
// and the v1 escape hatches behind the Advanced disclosure. It keeps the
// reference KeyTab's store-and-verify flow, now under the account it belongs
// to.
func (app *appState) accountPage(c *ui.Context) {
	a := &app.account
	ui.Column(c).Grow(1).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				ui.Text(c, "账号 (Account)").FontSize(18).Bold()
				muted(c, "登录 Bitwarden 账号后即可用。密钥保存在本机凭据管理器，由 Windows Hello 放行读取；生物识别只放行读取，密钥本身没有被加密。")
				notice(c, a.notice, a.noticeOK)

				a.accountCard(c)
				a.keyCard(c)
				a.advanced(c)
			})
		})
	})
}

// accountCard is the login state: identity and server when signed in, the
// login form when not. The form is a UI-only disclosure for now; the fake
// backend answers its submit with a not-implemented error, so nothing here
// pretends a login happened.
func (a *accountState) accountCard(c *ui.Context) {
	t := c.Theme()
	groupBox(c, "账号", func() {
		switch a.session.state {
		case loginNeeds2FA:
			a.identityRow(c, t.Warning)
			ui.Form(c, func() {
				ui.Field(c, "验证码", func() {
					ui.TextInput(c.Key("twofa"), &a.twoFACode).Placeholder("6 位验证码或恢复码")
				})
			})
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.PrimaryButton(c, "提交验证码").Disabled(strings.TrimSpace(a.twoFACode) == "" || a.busy).Clicked() {
					a.submit2FA()
					c.Invalidate()
				}
				if ui.Button(c, "取消").Disabled(a.busy).Clicked() {
					a.cancelLogin()
				}
			})
		case loginLoggedIn, loginLoggingIn:
			a.identityRow(c, t.Success)
			if a.session.state == loginLoggingIn {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Label("登录中")
				})
			}
			ui.Row(c).Gap(10).Children(func() {
				if ui.PrimaryButton(c, "重新登录").Disabled(a.busy).Clicked() {
					a.relogin()
					c.Invalidate()
				}
				if ui.Button(c, "退出登录").Disabled(a.busy).Clicked() {
					a.logout()
					c.Invalidate()
				}
			})
		default:
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "尚未登录").Bold()
				muted(c, "登录 Bitwarden 账号后，biowarden 会自动取出并保存密钥，无需逐项手动配置。")
			})
			if !a.loginOpen {
				ui.PrimaryButton(c, "登录 Bitwarden 账号").Disabled(a.busy).OnClick(a.beginLogin)
				return
			}
			ui.Form(c, func() {
				ui.Field(c, "邮箱", func() {
					ui.TextInput(c.Key("login-email"), &a.email).Placeholder("you@example.com")
				})
				ui.Field(c, "主密码", func() {
					ui.TextInput(c.Key("login-password"), &a.password).Password().Placeholder("master password")
				})
			})
			ready := strings.TrimSpace(a.email) != "" && a.password != ""
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.PrimaryButton(c, "登录").Disabled(!ready || a.busy).Clicked() {
					a.login()
					c.Invalidate()
				}
				if ui.Button(c, "取消").Disabled(a.busy).Clicked() {
					a.cancelLogin()
				}
				if a.busy {
					ui.Spinner(c).Label("登录中")
				}
			})
			muted(c, "登录后端将在后续工单接入：这里的按钮只切换界面状态，不会真的登录。")
		}
	})
}

// identityRow is the avatar, email, server and state line shared by the
// signed-in and 2FA branches.
func (a *accountState) identityRow(c *ui.Context, stateColor ui.Color) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Avatar(c, avatarName(a.session.email), nil)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, a.session.email).Bold()
			muted(c, "服务器 "+a.session.serverURL)
		})
		ui.Text(c, a.session.state.label()).FontSize(12).TextColor(stateColor)
	})
}

// keyCard is the stored-key status: whether a key exists, when it was stored,
// and the two actions that touch it. It reads the local store, so it stays
// meaningful whether or not the login backend is wired.
func (a *accountState) keyCard(c *ui.Context) {
	t := c.Theme()
	groupBox(c, "密钥状态", func() {
		acc := a.primary()
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if acc == nil {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "未保存").Bold().TextColor(t.Warning)
					muted(c, "本机尚未保存密钥。登录账号后会自动保存，或展开下面的「高级」手动录入。")
				})
				if ui.Button(c, "刷新").Disabled(a.busy).Clicked() {
					a.refreshKey()
					c.Invalidate()
				}
				return
			}
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "已保存").Bold().TextColor(t.Success)
					if acc.email != "" {
						ui.Text(c, acc.email).FontSize(12)
					}
				})
				muted(c, "账户 "+shortID(acc.userID)+" · 存储于 "+acc.created)
			})
			if ui.Button(c, "刷新").Disabled(a.busy).Clicked() {
				a.refreshKey()
				c.Invalidate()
			}
			if ui.Button(c, "删除").Disabled(a.busy).Clicked() {
				a.removeAccount(acc)
				c.Invalidate()
			}
		})
	})
}

// advanced holds the two v1 escape hatches: the extraction script for a
// browser extension the auto-login path cannot reach yet, and the manual
// paste form. Both were the whole Keys page before; folding them away keeps
// the default account page short.
func (a *accountState) advanced(c *ui.Context) {
	ui.Collapsible(c, "高级", &a.advancedOpen, func() {
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, "提取脚本兼容路径").Bold()
			muted(c, "在已解锁的浏览器扩展里取出 User ID 与加密密钥（keyB64）。Chrome / Edge：chrome://extensions → 开发者模式 → Bitwarden → 服务工作进程；Firefox：about:debugging#/runtime/this-firefox → Bitwarden → 检查。")
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "复制提取脚本").Clicked() {
					mygo.Clipboard.WriteText(extractSnippet)
					a.copiedAt = time.Now()
				}
				if !a.copiedAt.IsZero() {
					ui.Text(c, "已复制到剪贴板（"+itoa(len(extractSnippet))+" 字符），去控制台粘贴运行").
						FontSize(12).TextColor(c.Theme().Success)
				}
			})
			// The reference hides the snippet because a long script wrecks its
			// fixed-size SwiftUI layout; the page scrolls here, but the script
			// still stays behind a collapsed disclosure so it can be audited.
			ui.Collapsible(c, "查看提取脚本（可审计）", &a.scriptOpen, func() {
				ui.TextArea(c, &a.scriptBody).ReadOnly(true).Lines(6, 14).
					FontSize(11).Placeholder("（脚本预览）")
			})
			ui.Divider(c)
			a.manualEntry(c)
		})
	})
}

// manualEntry is the v1 form and the stored-account list, kept as the
// fallback for accounts the script path still handles.
func (a *accountState) manualEntry(c *ui.Context) {
	ui.Text(c, "手动粘贴录入（v1 旧方式）").Bold()
	muted(c, "从上面的脚本输出里复制 userId 与 keyB64，粘贴到这里保存。")
	ui.Form(c, func() {
		ui.Field(c, "User ID", func() {
			ui.TextInput(c.Key("userId"), &a.userId).
				Placeholder("控制台输出的 userId")
		})
		ui.Field(c, "加密密钥 (base64)", func() {
			ui.TextInput(c.Key("keyB64"), &a.keyB64).Password().
				Placeholder("keyB64")
		})
	})
	ready := strings.TrimSpace(a.userId) != "" && strings.TrimSpace(a.keyB64) != ""
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, "保存并验证").Disabled(!ready || a.busy).Clicked() {
			a.saveKey()
			// The consent runs off the UI thread, so ask for the frame
			// that starts polling for its result.
			c.Invalidate()
		}
		if a.busy {
			ui.Spinner(c).Label("验证中")
			ui.Text(c, "正在调用 Windows Hello 验证身份...").
				FontSize(12).TextColor(c.Theme().TextMuted)
		}
	})
	muted(c, "保存成功后，还要在 Bitwarden 扩展 → 设置 → 安全中开启「解锁时使用生物识别」。")

	if len(a.accounts) == 0 {
		muted(c, "本机还没有已保存的账户。")
		return
	}
	ui.Divider(c)
	muted(c, "已保存的账户")
	for _, acc := range a.accounts {
		acc := acc
		ui.Row(c).Key(acc.userID).Gap(12).Padding(8, 6).
			AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, acc.userID).FontSize(12).SingleLine().Tooltip(acc.userID)
				meta := "创建于 " + acc.created
				if acc.email != "" {
					meta = acc.email + " · " + meta
				}
				muted(c, meta)
			})
			if ui.Button(c, "测试解锁").Disabled(a.busy).Clicked() {
				a.testUnlock(acc)
				c.Invalidate()
			}
			// Removal runs off the UI thread, so the row is dropped by
			// a later tick rather than while this list is built.
			if ui.Button(c, "删除").Disabled(a.busy).Clicked() {
				a.removeAccount(acc)
				c.Invalidate()
			}
		})
	}
}

// avatarName gives ui.Avatar a name to take initials and a color from. An
// empty email still needs something, or the avatar draws nothing.
func avatarName(email string) string {
	if strings.TrimSpace(email) == "" {
		return "biowarden"
	}
	return email
}
