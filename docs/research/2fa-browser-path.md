> Research notes, kept for reference. Not user documentation; details may have moved since.

# 2FA 浏览器路径调研：Duo 与 WebAuthn

状态：调研笔记，**本次不实现**。所有结论尽量给出 `文件:行号` 证据；无法从源码确定的一律标注
「需实测验证」。代码标识符、字段名、路径保留英文。

## 0. 结论速览

1. 官方桌面端**没有原生 FIDO2**。WebAuthn 2FA 走的是 web vault 页面
   `webauthn-connector.html`，在 Electron 渲染进程里以 **iframe** 嵌入，
   通过 `window.postMessage` 把断言回给宿主页面
   （`libs/common/src/auth/webauthn-iframe.ts:24-49,79-91`）。
2. Duo 走 web vault 页面 `duo-redirect-connector.html`，用**系统浏览器**打开；
   完成 Duo 后由 connector 按 `client` 参数分流，桌面端最终跳
   `bitwarden://duo-callback?code=...&state=...`，再由 Electron 的
   `setAsDefaultProtocolClient("bitwarden")` 深链回到应用
   （`apps/desktop/src/main.ts:454-461`、`apps/desktop/src/app/app.component.ts:899-927`）。
3. **关键点：回调没有「服务端轮询」，全靠客户端本地的回传通道。**
   - WebAuthn：同页 `postMessage`（iframe -> 父页面）。
   - Duo：Electron 自定义协议 `bitwarden://` 深链（或 web 端的 BroadcastChannel、
     扩展的 `postMessage`）。
4. 这两条通道都预设了「应用内嵌 web vault 页面」或「注册了 `bitwarden://` 协议」。
   biowarden 是独立进程、没有内嵌 web vault、也不应该抢注 `bitwarden://`
   （会与官方桌面端冲突），**因此不能原样复用**。可行路线见第 6 节。
5. 官方 WebAuthn connector 对移动端的回跳地址是**写死的**
   `bitwarden://webauthn-callback`（`apps/web/src/connectors/webauthn.ts:13,22-32`），
   Duo 桌面回跳也是写死的 `bitwarden://duo-callback`
   （`apps/web/src/connectors/duo-redirect.ts:14,80-86`）。
   不存在「把回调指到任意 URL」的参数。

## 1. WebAuthn 浏览器路径

### 1.1 connector URL 与完整参数

桌面端 `TwoFactorAuthWebAuthnComponent` 取 `TwoFactorProviders2[7]` 作为 `providerData`，
把整块 provider 数据交给 `WebAuthnIFrame`
（`libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-webauthn/two-factor-auth-webauthn.component.ts:147-161`）。

`WebAuthnIFrame.init()` 组出的 V1 参数
（`libs/common/src/auth/webauthn-iframe.ts:24-34`）：

| 参数 | 值 | 说明 |
|---|---|---|
| `data` | `base64(JSON.stringify(providerData))` | provider 7 的原始数据，含 `challenge`、`allowCredentials` 等 |
| `parent` | `encodeURIComponent(document.location.href)` | 父页面 URL，connector 用它校验 postMessage 目标 |
| `btnText` | `encodeURIComponent(i18n("readSecurityKey"))` | 按钮文案 |
| `btnAwaitingInteractionText` | `encodeURIComponent(i18n("awaitingSecurityKeyInteraction"))` | 等待交互文案 |
| `v` | `"1"` | 协议版本 |

URL 是 `{webVaultUrl}/webauthn-connector.html?<params>`
（`libs/common/src/auth/webauthn-iframe.ts:42`）。Firefox 回退时改用
`webauthn-fallback-connector.html` 并追加 `locale`
（`libs/common/src/auth/webauthn-iframe.ts:35-41`）。

`data` 的编码是「UTF-8 安全的标准 base64」，不是 base64url：
`btoa(encodeURIComponent(str).replace(/%([0-9A-F]{2})/g, ...))`
（`libs/common/src/auth/webauthn-iframe.ts:66-73`）。connector 侧用 `b64Decode`
先 `atob` 再 `decodeURIComponent`（`apps/web/src/connectors/common.ts:49-60`）。

自托管/自建 connector 时，`data` 就是 provider 7 的 JSON 原文；V2（移动端）协议则把
`{data, headerText, btnText, btnReturnText, mobile?}` 整体 JSON 后再 base64
（`apps/web/src/connectors/webauthn.ts:152-179`、`apps/web/src/connectors/README.md:19-33`）。

### 1.2 断言选项与结果格式

- connector 解析 `data`：`parseWebauthnJson()` 把 `challenge` 和 `allowCredentials[].id`
  从 base64url 转成 `Uint8Array`（`apps/web/src/connectors/common-webauthn.ts:24-35`）。
- 断言值：`navigator.credentials.get({ publicKey: obj })`
  （`apps/web/src/connectors/webauthn.ts:224-233`）。
- 成功回传的 `dataString` 是 JSON：
  `{id, rawId, type, extensions, response:{authenticatorData, clientDataJson, signature}}`，
  各二进制字段用 base64url（`apps/web/src/connectors/common-webauthn.ts:1-22`）。
  这一串就是 provider 7 的 `twoFactorToken`。

### 1.3 回调协议：postMessage

- connector 加载后向父页面发 `postMessage("info|ready", parentUrl)`
  （`apps/web/src/connectors/webauthn.ts:103-107,288-294`）。
- 成功：`parent.postMessage("success|" + dataString, postMessageOrigin)`
  （`apps/web/src/connectors/webauthn.ts:271-285`）。
- 失败：`parent.postMessage("error|" + message, postMessageOrigin)`
  （`apps/web/src/connectors/webauthn.ts:260-269`）。
- 父侧 `WebAuthnIFrame.parseMessage()` 按 `|` 拆分，`validMessage()` 校验
  `event.origin === connectorLink.origin` 后才回调
  （`libs/common/src/auth/webauthn-iframe.ts:79-104`）。
- 父页面可以反向发 `"stop"` / `"start"` 暂停/恢复
  （`libs/common/src/auth/webauthn-iframe.ts:51-64`、`apps/web/src/connectors/webauthn.ts:235-257`）。
- `postMessageOrigin` 对云端域名取 connector 自身 origin，桌面 `file://` 父页面则保留
  `parentUrl`（`apps/web/src/connectors/common.ts:22-37`）。

### 1.4 桌面端怎么接：Electron 内嵌页面

桌面端 `shouldOpenWebAuthnInNewTab()` 默认返回 `false`
（`libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-webauthn/default-two-factor-auth-webauthn-component.service.ts:3-12`），
所以走 iframe 分支，父页面是 Electron 里加载的 web vault 页面。回调一路
`WebAuthnIFrame.successCallback -> webAuthnResultEmitter.emit({token})`
（`libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-webauthn/two-factor-auth-webauthn.component.ts:110-130`），
最终由 `TwoFactorAuthComponent.submit()` 作为 token 提交
（`libs/auth/src/angular/two-factor-auth/two-factor-auth.component.ts:287-345`）。

取消或超时是**本地错误**，弹 `webauthnCancelOrTimeout` toast，不是服务端错误码
（`libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-webauthn/two-factor-auth-webauthn.component.ts:121-127`）。

### 1.5 扩展 / Firefox 路径（参考）

Firefox 无法在扩展 iframe 里跑 WebAuthn，所以 `webauthn-fallback-connector.html` 在新标签页打开，
用 `window.postMessage({command:"webAuthnResult", data, remember}, "*")`
（`apps/web/src/connectors/webauthn-fallback.ts:133-155`），由扩展 content script 转发。
移动端则用 V2 + `deeplinkScheme` 回跳写死的 `bitwarden://webauthn-callback`
（`apps/web/src/connectors/webauthn.ts:13,22-32,170-173`、`apps/web/src/connectors/common.ts:89-95`）。

### 1.6 对 biowarden 的含义

biowarden 没有「加载 connector 的宿主页面」，也就没有 `postMessage` 的接收方。
只有三条路：
- (a) 自己开一个 WebView 加载 `webauthn-connector.html`，当父页面收 `postMessage`；
- (b) 自建一个 connector 页面，结果回跳到我们自己的通道；
- (c) 完全绕开浏览器，走原生 `webauthn.dll`（见
  `docs/research/windows-hello-go.md` 与 `keyguard-app` 参考），
  这条路不需要浏览器，但要自己构造 assertion JSON 并与服务端 `challenge`/`allowCredentials` 对齐。

官方 connector 的移动端深链写死到 `bitwarden://webauthn-callback`，我们无法把自己的 URL 塞进去。

## 2. Duo 浏览器路径

### 2.1 AuthUrl 来源

2FA 挑战里 `TwoFactorProviders2[2]`（或 `[6]`，OrganizationDuo）的 `AuthUrl`
是 Duo frameless OAuth URL；组件直接取 `providerData.AuthUrl`
（`libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-duo/two-factor-auth-duo.component.ts:74`）。

### 2.2 打开路径

桌面端 `launchDuoFrameless()` 组 URL 后用系统浏览器打开
（`apps/desktop/src/auth/services/desktop-two-factor-auth-duo-component.service.ts:37-54`）：

```
{webVaultUrl}/duo-redirect-connector.html
  ?duoFramelessUrl=<encodeURIComponent(duoFramelessUrl)>
  &handOffMessage=<encodeURIComponent(JSON.stringify({title,message,isCountdown}))>
```

`duo-redirect-connector.html` 由 `apps/web/webpack.base.js:149-153` 从
`apps/web/src/connectors/duo-redirect.ts` 构建。它先按严格规则校验 Duo URL 再跳转：
hostname 必须匹配 `/^api-[a-zA-Z0-9]+\.(duosecurity|duofederal)\.com$/`、https、无内嵌凭证、
无自定义端口、path 必须 `/oauth/v1/authorize`
（`apps/web/src/connectors/duo-redirect.ts:88-135`）。

### 2.3 回跳与分流

Duo 完成后回跳到 connector，带 `client`、`code`、`state`
（`apps/web/src/connectors/duo-redirect.ts:8-51`）：

| `client` | 回传方式 | 证据 |
|---|---|---|
| `web` | `BroadcastChannel("duoResult").postMessage({code,state})` | `duo-redirect.ts:29-34` |
| `browser` | `window.postMessage({command:"duoResult",code,state}, origin)` | `duo-redirect.ts:35-36` |
| `mobile` | `document.location.replace(bitwarden://duo-callback?code&state)` | `duo-redirect.ts:37-43` |
| `desktop` | 先显示 handoff，再 `document.location.replace(bitwarden://duo-callback?code&state)` | `duo-redirect.ts:44-51` |

桌面端的 `bitwarden://` 由 Electron 注册：
`app.setAsDefaultProtocolClient("bitwarden")`（`apps/desktop/src/main.ts:454-461`），
`processDeepLink()` 识别 `bitwarden://duo-callback` 后发 `"duoCallback"` 消息
（`apps/desktop/src/app/app.component.ts:899-927`），
`DesktopTwoFactorAuthDuoComponentService` 监听并组装 `token = code|state`
（`apps/desktop/src/auth/services/desktop-two-factor-auth-duo-component.service.ts:14-33`）。
这个 `code|state` 就是 provider 2/6 的 `twoFactorToken`。

### 2.4 对 biowarden 的含义

- 打开浏览器这一步简单：`ShellExecuteW("open", url)` 即可；mygo 已封装
  `internal/windows/system.go:266-278`（`shellExecute` / `ShellOpenExternal` 语义），
  框架面是 `internal/surface/surface.go:45-47` 的 `OpenURL`。
- 收结果这一步同样卡在写死的 `bitwarden://duo-callback`。我们既不能改 Duo AuthUrl 的
  `redirect_uri`（Duo 侧注册校验），也不能让 connector 回跳到任意 URL。
  可行路线同 WebAuthn：自建 WebView / 自建 redirect 页 / 或让用户手动粘贴
  （但官方 connector 跳到 `bitwarden://` 后浏览器通常只显示错误页，手动复制并不顺手）。

## 3. 通用 2FA 提交与「记住此设备」

- 挑战判定：token 端点 400 且 body 含非空 `TwoFactorProviders2`
  （`libs/common/src/services/api.service.ts:204-209`）。
- 重提字段：`twoFactorToken`、`twoFactorProvider`、`twoFactorRemember`（`"1"`/`"0"`），
  仅当 token 与 provider 同时存在才发（`libs/common/src/auth/models/request/identity-token/token.request.ts:21-30`）。
- 记住设备：成功响应若带 `TwoFactorToken` 且请求 `twoFactorRemember=1`，按 email 存起来
  （`libs/auth/src/common/login-strategies/login.strategy.ts:257-266`、
  `libs/common/src/auth/models/response/identity-token.response.ts:43,84`），
  下次登录自动以 `provider=5 (Remember)` 带上（`login.strategy.ts:157-176`）。
- biowarden 决策：**默认不做**，「记住此设备」留字段不做实现（见
  `internal/auth/login.go` 的 `TwoFactorSubmission` 注释）。平台侧改用 Windows Hello
  门控本地 userKey，而不是存服务端 remember token。

## 4. 关键点：回调怎么回来

| 方式 | 回传通道 | biowarden 可用性 |
|---|---|---|
| WebAuthn（桌面 iframe） | 同页 `postMessage`（connector -> 父页面） | 否，需要宿主页面 |
| WebAuthn（移动/深链） | `bitwarden://webauthn-callback?data=...` 写死 | 否，除非抢注 `bitwarden://` |
| WebAuthn（扩展） | 页面 `postMessage` -> content script -> 扩展 | 否 |
| Duo（desktop/mobile） | `bitwarden://duo-callback?code&state` 写死 | 否，除非抢注 `bitwarden://` |
| Duo（web） | `BroadcastChannel("duoResult")` | 否 |
| Duo（browser） | 页面 `postMessage` -> content script -> 扩展 | 否 |

不存在服务端轮询：客户端必须在本地拿到 `code|state` / assertion JSON，再自己重发
`connect/token` 请求（`login.strategy.ts:101-108`）。

## 5. 超时与失败语义

- 登录会话超时 5 分钟：`DefaultLoginStrategySessionTimeoutService`
  （`libs/auth/src/common/services/login-strategies/default-login-strategy-session-timeout.service.ts:14,50-59`），
  超时发 `loginSessionExpired` 并清登录缓存。这是整条登录链的超时，不是单个 2FA 方式。
- WebAuthn 取消/超时：本地 toast `webauthnCancelOrTimeout`
  （`two-factor-auth-webauthn.component.ts:121-127`）。
- Duo：服务端不提供额外超时；connector 只负责跳转，失败即停在浏览器页面。
  桌面端没有针对 Duo 的显式定时器。
- 2FA 校验失败：HTTP 400，走统一 `ErrorResponse`
  （`libs/common/src/services/api.service.ts:204-221`、`docs/research/goldwarden-auth.md` C9）。
  本地重试就是把 token 再提交一次。
- 服务端可能再次返回 400 + `TwoFactorProviders2`，即要求另一种方式；官方按新挑战重新渲染
  （`libs/auth/src/common/login-strategies/login.strategy.ts:334-354`）。

## 6. 平台缝接口建议

调研确认**至少两个平台缝**：

1. 打开默认浏览器（Duo / 若走浏览器版 WebAuthn）：
   ```go
   // Browser opens a URL in the user's default browser.
   type Browser interface {
       Open(ctx context.Context, url string) error
   }
   ```
   实现可直接用 mygo 的 `shell.OpenExternal`（`mygo/internal/windows/system.go:266-278`）
   或 `surface.OpenURL`（`internal/surface/surface.go:45-47`），无需新依赖。

2. 接收回调。两条候选：
   - **URI scheme handler**（`HKCU\Software\Classes\<scheme>`）：与现有 `internal/registry`
     的 HKCU 写入同一类操作。但官方 connector 用的是写死的 `bitwarden://`，
     抢注会与官方桌面端冲突，**不建议**；若自建 connector 页则可自定义 scheme（如
     `biowarden://`）。
   - **localhost HTTP 回调服务**：自建 connector 重定向到
     `http://127.0.0.1:<随机端口>/callback`。参考官方 SSO/CLI 的
     `sso-localhost-callback.service.ts` 做法（`apps/web/src/connectors/README.md` SSO 段），
     端口尝试 8065-8070。可封装为：
     ```go
     // CallbackServer receives an external-auth redirect on a loopback URL.
     type CallbackServer interface {
         // Listen starts on an ephemeral port and returns the redirect base URL.
         Listen(ctx context.Context) (redirectURL string, err error)
         // Wait blocks until the browser hits redirectURL or ctx ends.
         Wait(ctx context.Context) (params url.Values, err error)
         Close() error
     }
     ```
   - 若走原生 WebAuthn，则不需要回调缝，只需要一个「请求 Hello/安全密钥」的缝
     （可复用现有 `internal/hello` 或新增 `webauthn` seam）。

结论：**推荐路线** = WebAuthn 走原生 `webauthn.dll`（无浏览器回调问题）；
Duo 走自建 localhost 回跳页或自建 WebView。官方 connector 原样复用不可行。

## 7. 需实测验证清单

1. 服务端是否接受我们自建的 Duo `redirect_uri` / 是否允许 `deeplinkScheme` 影响回跳；
   官方 connector 写死的 `bitwarden://duo-callback` 是否可被自建页面替换。**需实测验证**
2. 自托管 / Vaultwarden 是否部署 `webauthn-connector.html` / `duo-redirect-connector.html`，
   以及 `{webVaultUrl}` 的具体取值。**需实测验证**
3. 原生 `webauthn.dll` 断言 JSON 能否被身份服务的 WebAuthn 2FA 校验接受
   （字段大小写、`clientDataJSON` 的 origin/challenge 拼装）。**需实测验证**
4. 抢注 `bitwarden://` 与官方桌面端共存时的行为（仅作风险评估，不建议采用）。**需实测验证**

## 8. 证据清单

bitwarden-clients（`bitwarden-clients`）:
- `libs/common/src/auth/webauthn-iframe.ts:24-49,51-64,66-73,79-104`
- `apps/web/src/connectors/webauthn.ts:13,18-32,103-107,138-179,181-233,235-257,260-294`
- `apps/web/src/connectors/common-webauthn.ts:1-35`
- `apps/web/src/connectors/common.ts:5-37,49-60,71-73,89-95`
- `apps/web/src/connectors/webauthn-fallback.ts:133-155`
- `apps/web/src/connectors/duo-redirect.ts:8-51,88-135`
- `apps/web/src/connectors/README.md:9-125`
- `apps/web/webpack.base.js:123-153`
- `libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-webauthn/two-factor-auth-webauthn.component.ts:59,106-130,147-161`
- `libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-webauthn/default-two-factor-auth-webauthn-component.service.ts:3-12`
- `libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-duo/two-factor-auth-duo.component.ts:49-101`
- `libs/auth/src/angular/two-factor-auth/child-components/two-factor-auth-email/two-factor-auth-email.component.ts:98-155`
- `libs/auth/src/angular/two-factor-auth/two-factor-auth.component.ts:287-345`
- `apps/desktop/src/auth/services/desktop-two-factor-auth-duo-component.service.ts:14-54`
- `apps/desktop/src/app/app.component.ts:899-927`
- `apps/desktop/src/main.ts:454-461`
- `libs/common/src/services/api.service.ts:161-221`
- `libs/common/src/auth/models/request/identity-token/token.request.ts:21-30`
- `libs/common/src/auth/models/request/identity-token/token-two-factor.request.ts`
- `libs/common/src/auth/models/response/identity-two-factor.response.ts`
- `libs/common/src/auth/models/response/identity-token.response.ts:43,84`
- `libs/common/src/auth/enums/two-factor-provider-type.ts`
- `libs/auth/src/common/login-strategies/login.strategy.ts:101-108,157-176,257-266,334-358`
- `libs/auth/src/common/services/login-strategies/default-login-strategy-session-timeout.service.ts:14,50-59`
- `libs/common/src/auth/two-factor/services/default-two-factor-api.service.ts:110-112`

biowarden:
- `internal/auth/twofactor.go`（`TwoFactorChallenge`、`ChallengeInteraction`、`TwoFactorSolver`）
- `internal/auth/login.go`（`LoginFlow.SolveTwoFactor`）
- `docs/research/goldwarden-auth.md` C6-C9
- `docs/research/windows-hello-go.md`（原生 WebAuthn 候选）

mygo:
- `internal/windows/system.go:266-278`（`shellExecute` / `OpenExternal`）
- `internal/surface/surface.go:45-47`（`OpenURL`）
- `examples/menubar-native`（GUI 起点）
