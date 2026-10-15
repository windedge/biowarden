# v2 验收归档：安装即用（工单 #16 地图终点）

v2 的目标是「安装即用」：新用户装上 biowarden -> 首启自动注册已安装浏览器 -> 直接弹登录窗 ->
邮箱+主密码+TOTP -> Windows Hello 确认存钥 -> 扩展里开启生物识别 -> 锁定后解锁，全程零控制台脚本。
v1 的验收清单（`docs/acceptance.md`）仍然有效，本文档只归档 v2 相对 v1 的新增与结论。

前置：`go tool mygo build` 产出的 `build/windows-amd64/biowarden.exe`（或 NSIS 安装器装出的版本）。
验收对象是**真实浏览器 + 官方 Bitwarden 扩展 + 真实 Windows Hello**，不是测试桩。

## 1. 验收目标

- 新账号（本机凭据管理器里没有该账号密钥）从登录到扩展解锁，全程不碰控制台脚本。
- 首启只在满足「零配置」条件时自动动作，且每次安装只跑一次。
- 密钥写入前必须经过 Windows Hello；拒绝弹窗不落任何数据。
- 2FA 的 TOTP / 邮箱 / YubiKey 三条路径可交互；WebAuthn / Duo 明确拒绝并指路脚本后备。

## 2. 已验收的路径（真机）

以下路径在一台装有官方 Bitwarden 扩展的真机上走通（本机为 Chrome/Chromium 系）：

- [x] 首启自动注册：零配置时探测 App Paths，为已安装且未注册的浏览器写入 manifest；已有 foreign
      注册的行被跳过并在浏览器页摘要里点名，未覆盖。
- [x] 首启只跑一次：`HKCU\Software\biowarden\FirstRunCompleted` 写入后，再次启动不再自动注册、不再弹登录窗。
- [x] 登录窗：邮箱 + 主密码；服务器地址在「高级」折叠区，默认官方云，自托管可填。
- [x] 登录链路：prelogin 读 KDF 参数 -> 请求 token -> 派生主密钥 -> 解出 64 字节 userKey。
- [x] 2FA（TOTP）：服务器返回两步验证挑战时，登录窗渲染验证码输入框，提交正确 TOTP 后完成登录。
- [x] Windows Hello：登录成功后弹 Hello，通过后把密钥写入凭据管理器 `Biowarden/<userId>`。
- [x] 扩展解锁：扩展里开启「Unlock with biometrics」并完全重启浏览器后，锁定扩展点解锁 ->
      弹 Hello -> 通过即解锁，能进密码库。
- [x] 刷新/重登：会话在内存里保留主密码，「刷新密钥」与「重新登录」可重新派生并重写密钥。
- [x] 退出登录：Hello 确认后删除密钥；清除内存中的会话与 token。
- [x] 脱敏：`%LOCALAPPDATA%\biowarden\host.log` 里搜不到主密码、token、keyB64 明文。

## 3. 验收中修掉的真实 bug

这两个 bug 都是真机走查时暴露、并已修复的（提交见 git 记录）：

1. **2FA 挑战解析失败（`fix(auth): parse the two-factor challenge from the real identity response`）。**
   身份服务器把 `TwoFactorProviders` 序列化成 **JSON 字符串数组**（`["0","1"]`），而早期代码按 `[]int`
   解码。类型不匹配让整个 `json.Unmarshal` 失败，`parseTwoFactorChallenge` 返回 nil，登录停在
   `ErrTwoFactorRequired` 且没有方式列表，UI 只显示「服务器要求两步验证，但没有返回可用的验证方式」，
   TOTP 输入框根本不会渲染。修复：按字符串或数字解码 provider 号（`providerNumbers`）、对响应做尽力解码
   （单个坏字段不能掩盖方式列表）、把带空格的消息（`"Two factor required."`）归一化后也能分类为 2FA。
2. **Hello 提示窗有时不在最前（`fix(hello): keep the consent prompt above other windows`）。**
   Hello 提示由 native messaging host 的一个隐藏窗口持有。host 是后台进程，Windows 拒绝它调用
   `SetForegroundWindow`，而从未被激活的窗口又无法把提示提到最前，于是任何时候有普通窗口盖在 owner 上，
   提示就可能被挡住。修复：owner 窗口加 `WS_EX_TOPMOST`，并在请求前短暂借用前台
   （`GetForegroundWindow`、`AttachThreadInput`、`SetForegroundWindow`、解绑）让提示拿到键盘焦点，
   请求后再归还原前台；`WS_EX_TOOLWINDOW` 让零尺寸 owner 不进 Alt+Tab。用户真机矩阵确认通过。

## 4. 暂缓 / 不支持项

- **WebAuthn 与 Duo 2FA：明确不支持。** 官方 connector 的回调依赖内嵌 web vault 页面（`postMessage`）
  或写死的 `bitwarden://` 深链；biowarden 是独立进程、没有内嵌页面、也不抢注该协议。决策见
  `docs/research/2fa-browser-path.md`（工单 #29）。UI 会点名这两种方式并指向「高级」里的提取脚本。
- **首启干净环境验证：暂缓。** 开发机不是全新机器，首启自动注册是在已有 Chrome/Chromium 注册的环境里
  验证的；「一台从没装过 biowarden、也没装过官方桌面端的机器」的全新首启还需要单独走一遍。
- **Vaultwarden / 自托管实测：暂缓。** 登录端点的旧 prelogin variant 是按兼容 Vaultwarden 设计的，但还没
  在真实 Vaultwarden 实例上验证过（`docs/research/goldwarden-auth.md` A1、D11）。
- **SDK IPC 通道实测：暂缓。** 新版桌面 IPC（Noise/CBOR/JSON RPC）代码在位，但扩展的
  `BiometricsSDKIPC` 远端开关未放量，尚未在真机上观察走该通道的解锁。旧命令协议通道已验收。
- **Edge 与 Firefox：暂缓。** v2 有四个注册点，真机只走了 Chrome/Chromium 系；Edge 与 Firefox 的真机
  验收还没做。Brave、Vivaldi、Safari 等不在支持范围。

## 5. 怎么重跑验收

1. 构建：`go tool mygo build`（或 `pwsh scripts/build.ps1`），确认 `build/windows-amd64/biowarden.exe`。
2. 若机器上跑着官方 Bitwarden 桌面客户端，先退出，避免它重写 `NativeMessagingHosts` 注册表键。
3. 清掉首启标记以便重验自动注册：删除注册表值
   `HKCU\Software\biowarden\FirstRunCompleted`（或用一台没有该值的机器）。
4. 双击 `biowarden.exe`：确认托盘出现图标、窗口打开、浏览器页摘要显示自动注册结果；若本机凭据管理器里
   没有该账号密钥，登录窗应自动打开。
5. 登录窗里输入邮箱 + 主密码；若账号开了 2FA，输入 TOTP / 邮箱码 / 触碰 YubiKey 提交。
6. Hello 通过后到账号页确认密钥状态为「已保存」；用 `reg query` 或凭据管理器确认
   `Biowarden/<userId>` 已写入。
7. 在扩展的 设置 -> 安全 开启「Unlock with biometrics」，然后**完全退出并重启浏览器**（不是只重载扩展）。
8. 锁定扩展，点解锁：确认弹出 Hello（应在最前、居中），通过后解锁成功。
9. 检查 `%LOCALAPPDATA%\biowarden\host.log` 尾部有没有报错，并确认搜不到主密码、token、keyB64 明文。
10. 走一遍负路径：Hello 取消 -> 扩展保持锁定且不转圈；删除账号密钥（Hello 确认）-> 扩展再解锁应失败。

对接 v1 覆盖的其余场景（三浏览器 x 两通道、manifest 冲突恢复、移动 exe 后修复），继续用
`docs/acceptance.md` 的清单。

## 退出标准

上述「已验收路径」全部通过，且第 4 节列出的暂缓项各有明确工单后续处理。任何「转圈」或
「解锁失败但 Hello 已通过」都要记录浏览器 + 扩展版本 + `host.log`，回到协议工单排查。
