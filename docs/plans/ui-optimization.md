# UI 优化方案（v0.1.5 之后的缺口清单）

现状水平：令牌纪律、语义色、notice/badge/button 体系已经达到成熟桌面工具水准，i18n 布局防线完整。最大缺口不在视觉，而在 (1) 键盘与逃生路线（模态无 Esc、登录无初始焦点）、(2) 反馈诚实性（假开关/假下拉说"已生效"）、(3) 空态与节奏细节。以下按 用户价值 x 成本 排序，均为增量修改，不动 style.go 的调色板与四页结构。

## 优化清单

1. **模态窗 Esc 出口缺失**（S，价值高）
   - 问题：`window_login.go:79` 与 `window_browsers.go:56` 的 OnClose 只接住 X/系统关闭；键盘用户无法 Esc 取消模态。
   - 方案：在 `loginView`/`confirmView` 根部用 `c.OnShortcut(0, ui.KeyEscape, ...)`（`mygo/ui/actions.go:51`，root 上即窗口级）分别调 `s.requestClose()` / `browsers.applyBulkCancel()+closeConfirm()`。busy 时不响应。
2. **登录窗初始焦点**（S，价值高）
   - 问题：`pages_login.go:54` email 字段无 AutoFocus，每次打开都要鼠标点击。
   - 方案：email 输入 `.AutoFocus()`（`mygo/ui/api_gen.go:2869`）；2FA 阶段对 code 字段同理。
3. **假开关与假下拉的欺骗性反馈**（S，价值高）
   - 问题：`pages_settings.go:55` showTray 是 memory-only 却报"已显示托盘"；`state_settings.go:237` logLevel 同样写不进后端，用户以为生效。
   - 方案：短期把两行改为 disabled + caption 说明"即将支持"（诚实优先），或干脆隐藏；若要实现走独立工单（托盘重建、host 环境变量）。
4. **密码可见性切换**（S，价值中高）
   - 问题：`pages_login.go:57` 密码框无 reveal；输错排查困难。mygo 的 `Password()` 是按帧开关（`textinput.go:256` 注释即为此设计）。
   - 方案：字段尾部 plainButton 画眼睛图标，`s.showPassword` 为 false 才链 `.Password()`。
5. **浏览器页空态缺失**（S，价值中）
   - 问题：`pages_browsers.go:60` rows 为 0 时 List 画空白，页面只剩标题和禁用按钮。
   - 方案：rows 空时画居中空态（复用 `iconBrowsers` 放大 + muted 一句"未发现受支持的浏览器"），loca­les 加一条文案。
6. **browsersState.tick 每帧重读注册表**（M，价值高，需先验证）
   - 问题：`state_browsers.go:105` tick 无条件 `reload()`；按钮 hover 的 `AnimateColor`（widgets.go:117）会驱动连续动画帧，若动画帧触发 view 重建则每帧读注册表。先写一个测试数 refresh 次数证实。
   - 方案：tick 节流（`c.After(2s)` 轮询或仅页面可见时 reload），或 reload 移出 tick 只在进入页面/操作后触发。
7. **页头节奏不统一**（S，价值中低）
   - 问题：account/settings 页头与内容间用 `spaceMedium`+独立滚动区，browsers `Gap(spaceRow)`、log `Gap(spaceSmall)`（`pages_browsers.go:47`、`pages_log.go:13`）；browsers/log 页无独立滚动头。settings 的 `Padding(pagePadding, pagePadding, pagePadding, pagePadding)` 可写单值。
   - 方案：统一为"固定页头 + Divider + 滚动体"或统一 Gap 档为 `spaceMedium`，只动包装不动内容。
8. **stored accounts 行缺少分隔**（S，价值中低）
   - 问题：`pages_account.go:151` 起的行纯 Gap 堆叠，多账号时挤成一团，与 browsers 行的卡片感不一致。
   - 方案：行间加 `ui.Divider` 或行内套轻背景（`surface` + radius），与 browserRow 同构。
9. **焦点环验证**（S，价值中）
   - 问题：`applyPalette` 设了 `t.Focus`（style.go:141），但未验证 ButtonBase 在 Tab 导航下画不画环、四页 Tab 顺序是否合理。mygo 有 `FocusRing`/`FocusVisible`（element.go:1436）。
   - 方案：写一个 Tab 遍历测试（`ui.Tester`），确认按钮/输入/侧栏项可见焦点；缺环的控件在 `button()` 里补自绘环。
10. **布局测试补 MinHeight 用例**（S，价值低防御性）
    - 问题：`pages_layout_test.go:41` 只测 800x640，未测 MinHeight=520（`ui.go:70`）下的纵向截断。
    - 方案：加一组 800x520 断言，顺手看 ja/de 在窄高的表现。

## 推荐批次

- **批次 1（键盘与诚实反馈）**：条目 1/2/3/4。全是 S 成本、同触"操作路线"文件（window_login/window_browsers/pages_login/pages_settings），一次验收一组。
- **批次 2（空态与节奏）**：条目 5/7/8。纯绘制层，无行为变更，截图测试直接守住。
- **批次 3（防御与验证）**：条目 6/9/10。先证实再修，产出是测试 + 小修。

批次 1 验收标准：登录窗打开即焦点在 email；Esc 三处（登录窗凭据/2FA、批量确认）等效于 Cancel 且 busy 中不误关；设置页不再存在"点了报成功但实际没生效"的控件；密码可显示/隐藏且隐藏为默认。56 组布局断言与截图全绿。
