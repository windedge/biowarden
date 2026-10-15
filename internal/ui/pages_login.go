package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// loginView draws the login window: the credentials form first, then the
// two-factor prompt the server asked for. It is the only login entry; the
// account page opens this window instead of drawing a form of its own.
func (s *loginWindow) loginView(c *ui.Context) {
	s.tick(c)
	// A frame after the Advanced disclosure toggled is where the window learns
	// its new height; syncHeight keeps a steady form from resizing every frame.
	s.syncHeight()
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// The window follows the form's height (syncHeight above), so the scroll
		// view only catches the rare hint that wraps past its measured target.
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				ui.Text(c, "登录 Bitwarden 账号").FontSize(16).Bold()
				muted(c, "登录会连接到 Bitwarden 身份服务器；密钥写入本机凭据管理器前需要 Windows Hello 确认。")
				if s.challenge == nil {
					s.credentialsForm(c)
				} else {
					s.twoFactorForm(c)
				}
			})
		})
	})
}

// credentialsForm is the email/password step, with the server address behind
// the Advanced disclosure.
func (s *loginWindow) credentialsForm(c *ui.Context) {
	// entered records Enter in any of the three fields; .Submitted is mygo's
	// Enter-in-a-single-line-input signal, so the form needs no key hook.
	entered := false
	ui.Form(c, func() {
		ui.Field(c, "邮箱", func() {
			entered = ui.TextInput(c.Key("login-email"), &s.email).
				Placeholder("you@example.com").Disabled(s.busy).Submitted() || entered
		})
		ui.Field(c, "主密码", func() {
			entered = ui.TextInput(c.Key("login-password"), &s.password).
				Password().Placeholder("主密码").Disabled(s.busy).Submitted() || entered
		})
	})
	ui.Collapsible(c, "高级", &s.advancedOpen, func() {
		ui.Form(c, func() {
			ui.Field(c, "服务器地址", func() {
				entered = ui.TextInput(c.Key("login-server"), &s.serverURL).
					Placeholder(defaultServerURL).Disabled(s.busy).Submitted() || entered
			}).Error(s.serverErr)
		})
		muted(c, "默认使用 Bitwarden 官方云。自托管请填自己的地址，例如 https://vault.example.com；提交时按官方规则规范化。")
	})
	notice(c, s.notice, s.noticeOK)
	ready := strings.TrimSpace(s.email) != "" && s.password != ""
	if s.submitOnEnter(entered) {
		c.Invalidate()
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, "登录").Disabled(!ready || s.busy).Clicked() {
			s.submit()
			c.Invalidate()
		}
		if ui.Button(c, "取消").Disabled(s.busy).Clicked() {
			s.cancel()
			c.Invalidate()
		}
		if s.busy {
			ui.Spinner(c).Label("登录中")
			ui.Text(c, "正在登录...").FontSize(12).TextColor(c.Theme().TextMuted)
		}
	})
}

// twoFactorForm renders the pending challenge. Duo and WebAuthn have no path
// in biowarden, so they are named but not offered; when the server offers
// nothing else, the window says so and keeps Cancel as the only exit.
func (s *loginWindow) twoFactorForm(c *ui.Context) {
	methods := s.challenge.codeMethods()
	if len(methods) == 0 {
		s.unsupportedMethods(c)
		return
	}
	if len(methods) > 1 {
		labels := make([]string, 0, len(methods))
		for _, m := range methods {
			labels = append(labels, methodTitle(m))
		}
		ui.Segmented(c, &s.methodIndex, labels...).Label("两步验证方式")
	}
	m, _ := s.selectedMethod()
	muted(c, "服务器要求两步验证："+methodTitle(m))
	entered := false
	ui.Form(c, func() {
		ui.Field(c, methodFieldLabel(m), func() {
			entered = ui.TextInput(c.Key("login-2fa"), &s.code).
				Placeholder(methodHint(m)).Disabled(s.busy).Submitted() || entered
		})
	})
	if m.provider == twoFactorEmail {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "发送验证码").Disabled(s.busy || s.emailSent).Clicked() {
				s.sendEmailCode()
				c.Invalidate()
			}
			if s.emailSent {
				ui.Text(c, "已发送").FontSize(12).TextColor(c.Theme().Success)
			}
		})
		if m.email != "" {
			muted(c, "验证码会发送到 "+m.email)
		}
	}
	if unsupported := s.challenge.unsupported(); len(unsupported) > 0 {
		muted(c, "服务器还提供了 "+methodTitles(unsupported)+"；"+errTwoFactorUnsupported.Error())
	}
	notice(c, s.notice, s.noticeOK)
	if s.submitTwoFactorOnEnter(entered) {
		c.Invalidate()
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, "提交验证码").Disabled(strings.TrimSpace(s.code) == "" || s.busy).Clicked() {
			s.submit2FA()
			c.Invalidate()
		}
		if ui.Button(c, "取消").Disabled(s.busy).Clicked() {
			s.cancel()
			c.Invalidate()
		}
		if s.busy {
			ui.Spinner(c).Label("验证中")
		}
	})
}

// unsupportedMethods is the challenge with no method biowarden can answer:
// the wording points at the account page's extraction script, and Cancel is
// the way out. This is the decision the 2FA browser-path research recorded,
// not an oversight.
func (s *loginWindow) unsupportedMethods(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(12).Gap(8).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, errTwoFactorUnsupported.Error()).Bold()
		muted(c, "服务器提供的方式："+methodTitles(s.challenge.methods))
	})
	notice(c, s.notice, s.noticeOK)
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, "取消").Disabled(s.busy).Clicked() {
			s.cancel()
			c.Invalidate()
		}
		if s.busy {
			ui.Spinner(c).Label("取消中")
		}
	})
}

// methodTitle names a method the way the prompt shows it.
func methodTitle(m twoFactorMethod) string {
	switch m.provider {
	case twoFactorAuthenticator:
		return "验证器应用（TOTP）"
	case twoFactorEmail:
		return "邮箱验证码"
	case twoFactorYubiKey:
		return "YubiKey"
	case twoFactorDuo, twoFactorOrgDuo:
		return "Duo"
	case twoFactorWebAuthn:
		return "安全密钥（WebAuthn）"
	case twoFactorRecoveryCode:
		return "恢复码"
	default:
		return m.name
	}
}

// methodTitles joins method names for the one-line hints.
func methodTitles(methods []twoFactorMethod) string {
	names := make([]string, 0, len(methods))
	for _, m := range methods {
		names = append(names, methodTitle(m))
	}
	return strings.Join(names, "、")
}

// methodFieldLabel is what the code field asks for, per method.
func methodFieldLabel(m twoFactorMethod) string {
	switch m.provider {
	case twoFactorEmail:
		return "邮箱验证码"
	case twoFactorYubiKey:
		return "YubiKey 一次性密码"
	default:
		return "验证码"
	}
}

// methodHint is the field's placeholder, per method.
func methodHint(m twoFactorMethod) string {
	switch m.provider {
	case twoFactorEmail:
		return "邮件里的 6 位验证码"
	case twoFactorYubiKey:
		return "触摸 YubiKey 后出现的 44 位密码"
	default:
		return "验证器应用里的 6 位验证码"
	}
}
