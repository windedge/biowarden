# goldwarden 登录链路移植调研（bitwarden 账号登录 → 解出 userKey）

调研工单: wayfinder research · 参照代码：`goldwarden`（HEAD 7727d3a）、`bitwarden-clients`（HEAD 9d29a0d）

## 结论摘要

1. **goldwarden 是 MIT 许可（`LICENSE:1`），KDF/HTTP/2FA 部分可以整体搬进 biowarden**，只需保留版权声明；`internal/auth/` 建议大量参考而非逐字复制（约 5-7 成代码可直接移植，2FA/Duo/WebAuthn 需重写或裁剪）。
2. 端点为 `{identity}/accounts/prelogin`、`{identity}/connect/token`、`{api}/accounts/profile`、`{api}/two-factor/send-email-login`；官方已新增 `{identity}/accounts/prelogin/password`（返回 `KdfSettings` + `Salt`），goldwarden 用的是旧 variant。
3. **goldwarden 伪装的是 web 客户端（`client_id=web`, `deviceType=10`, `deviceName=firefox`），不是桌面端**；桌面端的精确身份是 `client_id=desktop`、`deviceType=6`、`deviceName=windows`。这是移植时必须修正的差异。
4. KDF 两条路径（PBKDF2-SHA256 / Argon2id）与官方语义完全一致，`golang.org/x/crypto/argon2` 纯 Go，`CGO_ENABLED=0` 可构建；Argon2id 官方 memory 单位是 MiB，SDK 内部换算 KiB，goldwarden 的 `Memory*1024` 正确。
5. **TOTP(TOTP=0)、邮箱码(1) 纯 Go 可实现；Duo(2/6) 与 WebAuthn(7) 需要浏览器或原生 FIDO2**，goldwarden 的 FIDO2 依赖 `keys-pub/go-libfido2`（cgo），`CGO_ENABLED=0` 下被 `nofido2` build tag 排除。
6. 官方桌面端 WebAuthn 2FA 走「web vault 的 `webauthn-connector.html` iframe / 新标签页」，本质是浏览器 `navigator.credentials.get`；纯 Go 无浏览器要落地，需另寻原生路径（webauthn.dll / 系统浏览器回填），**列为需实测验证项**。

---

## A. 端口与身份

### A1 涉及哪些端点 + 官方实际用哪个 prelogin

| 用途 | 路径 | 证据 |
|---|---|---|
| prelogin（旧，goldwarden 用的） | `POST {identity}/accounts/prelogin` | goldwarden `cli/agent/bitwarden/auth.go:120` |
| prelogin（官方新，password variant） | `POST {identity}/accounts/prelogin/password` → `{ KdfSettings, Salt }` | bitwarden-clients `libs/common/src/auth/password-prelogin/password-prelogin-api.service.ts:19`，response `password-prelogin.response.ts` |
| token | `POST {identity}/connect/token` | goldwarden `auth.go:159`、`auth.go:355`；官方 `libs/common/src/services/api.service.ts:187` |
| refresh | 同一 `{identity}/connect/token`，`grant_type=refresh_token` | 官方 `api.service.ts:1498` 附近 `refreshAccessToken`；goldwarden `auth.go:314-317` |
| profile | `GET {api}/accounts/profile` | 官方 `api.service.ts:266` |
| 2FA 邮箱码发送 | `POST {api}/two-factor/send-email-login` | 官方 `libs/common/src/auth/two-factor/services/default-two-factor-api.service.ts:112` |
| 2FA WebAuthn challenge | `POST {api}/two-factor/get-webauthn-challenge` | 同上 `default-two-factor-api.service.ts`（`/two-factor/get-webauthn-challenge`） |
| 2FA provider 列表 | `GET {api}/two-factor` | 同上 `getTwoFactorProviders` |
| vault sync（解出后校验用） | `GET {api}/sync` | goldwarden `cli/agent/bitwarden/sync.go:21` |

**结论**：官方客户端现在走的是新 `password` variant（`KdfSettings` + `Salt`），且 salt 由服务端返回；旧 `/accounts/prelogin` 仍在（goldwarden 使用）。**新老差异要点**：新 variant 的 salt 与 kdf 同在一个响应里，且官方有 feature flag `PM27060_PasswordPreloginFromSdk` 决定用服务端 salt 还是本地 email（`password-login.strategy.ts:150-200`）。移植时建议先实现旧 variant（兼容 Vaultwarden），并为新 variant 留出 salt 字段。

### A2 伪装桌面端的全部身份细节

**官方桌面端（权威）**：
- `client_id = "desktop"`：`ClientType.Desktop`（`libs/client-type/src/index.ts`），由 `platformUtilsService.getClientType()` 传入 `toIdentityToken(clientId)`；桌面实现 `getClientType() { return ClientType.Desktop }`（`apps/desktop/src/platform/services/electron-platform-utils.service.ts:30`）
- `deviceType = 6`（Windows）：`DeviceType.WindowsDesktop = 6`（`libs/common/src/enums/device-type.enum.ts:18`），桌面 `getDevice()` 返回 `ipc.platform.deviceType`
- `deviceName = "windows"`：`getDeviceString() = DeviceType[6].toLowerCase().replace("desktop","")`（`electron-platform-utils.service.ts:25-28`）
- `deviceIdentifier = appId`（GUID）：`DeviceRequest`（`libs/common/src/auth/models/request/identity-token/device.request.ts`）
- 请求体：`scope="api offline_access"`、`grant_type="password"`、`username`、`password=<masterPasswordHash>`、`deviceType`/`deviceIdentifier`/`deviceName`；2FA 时附 `twoFactorToken`/`twoFactorProvider`/`twoFactorRemember`（`token.request.ts:24-52`、`password-token.request.ts`）
- 请求头：`Content-Type: application/x-www-form-urlencoded; charset=utf-8`、`Accept: application/json`、`Device-Type`、`Bitwarden-Client-Name`(=`getClientType()`)、`Bitwarden-Client-Version`、`Bitwarden-Package-Type`（`api.service.ts:164-172` 与 `applyPlatformHeaders` `api.service.ts:1324-1332`）

**goldwarden（差异明显）**：
- 主密码登录硬编码 `client_id="web"`、`deviceType="10"`（FirefoxBrowser）、`deviceName="firefox"`（`auth.go:148-153`），伪装成 web 客户端而非桌面端
- `deviceType()` 仅用于 API key / passwordless 路径，Windows 返回 `"6"`（`auth.go:58-67`）
- `deviceIdentifier = cfg.ConfigFile.DeviceUUID`（一个持久化 UUID，`auth.go:151`）
- 头：`User-Agent: Goldwarden (github.com/quexten/goldwarden)`、`Device-Type: 10`、`Bitwarden-Client-Name: goldwarden`、`Bitwarden-Client-Version: 0.0.0`、`Auth-Email: base64url(username)`（`cli/agent/bitwarden/http.go:100-105`、`:54`）

**差异结论**：goldwarden 是「web 客户端伪装 + 假 version 0.0.0」，官方桌面端用 `desktop`/`windows`/真实版本号。**biowarden 想冒充桌面端同级客户端，应使用官方桌面端的身份**（`desktop` + `6` + `windows` + 合理 Client-Version），不要照抄 goldwarden 的 `web`/`firefox`。`Auth-Email`（X-Request-Email 等价物）是 2FA 相关请求会读的头。

---

## B. KDF 与密钥解出

### B3 prelogin 返回的 KDF 参数

- 旧响应（goldwarden）：`KDF, KDFIterations, KDFMemory, KDFParallelism`（`auth.go:27-32`）
- 新响应（官方）：`KdfSettings`（含 Kdf/KdfIterations/KdfMemory/KdfParallelism）+ `Salt`（`password-prelogin.response.ts`；`KdfConfigResponse` 在 `libs/common/src/key-management/models/response/kdf-config.response.ts`）
- KDF type 取值：`0 = PBKDF2_SHA256`，`1 = Argon2id`（`libs/legacy-crypto/src/enums/kdf-type.enum.ts`；goldwarden `crypto/kdf.go:18-19`）
- salt：**email 去首尾空白 + 转小写**（`legacy-compat-key.service.ts:83-88` `email.trim().toLowerCase()`；goldwarden `kdf.go:51`、`:53` 用 `strings.ToLower(email)`）。新 variant 若用服务端 `Salt`，则直接用返回的 salt 字符串（`password-login.strategy.ts:188`）

### B4 派生与 userKey 解出

**masterKey**
- PBKDF2-SHA256：`pbkdf2.Key(password, lower(email), iterations, 32, sha256)` → 32B（`kdf.go:50-51`）
- Argon2id：`salt = sha256(lower(email))`（32B），`argon2.IDKey(password, salt, iterations, memory*1024, parallelism, 32)`（`kdf.go:52-54`）
- 官方无 HKDF-Expand 拆 enc/mac 用于 masterKey 本身；**只有 stretch 时才拆**。

**masterPasswordHash**（发给 token 请求）
- `pbkdf2.Key(masterKey(32B), password, iterations=1, keyLen=32, sha256)` → base64（`legacy-compat-key.service.ts:119-132`；goldwarden `auth.go:111`、`:145`）。**注意参数顺序：password=masterKey，salt=明文密码**。

**stretchKey**（本地解 userKey 用）
- `stretchedMasterKey = HKDF-Expand-SHA256(masterKey, info="enc", 32) || HKDF-Expand-SHA256(masterKey, info="mac", 32)` = 64B（`default-key-generation.service.ts:68-96`；goldwarden `keyhierarchy.go:84-105`）

**userKey 位置与包装算法**
- token 响应字段名为 `Key`（注释 `masterKeyEncryptedUserKey`），是 EncString，类型 `2.` = `AesCbc256_HmacSha256_B64`（`identity-token.response.ts` `key?: EncString`；goldwarden `crypto/encstring.go:22` 常量 `AesCbc256_HmacSha256_B64=2`）
- 解出：`DecryptWith(Key, stretchedMasterKey)` → 64B userKey（EncString 为 64 字节时按 enc|mac 各 32B 切，`crypto/crypto.go:64-72`）；goldwarden 用 `InitKeyringFromMasterKey`（`keyhierarchy.go:20-38`，调用方 `actions/login.go:87`）
- token 响应里 `PrivateKey` = userKey 加密后的**用户私钥**（非 userKey 本身），解出后可省略；biowarden 只需 `Key` 得到 64B userKey
- 另有新字段 `AccountKeys`（`accountKeysResponseModel`），goldwarden 未用（`identity-token.response.ts`）

### B5 goldwarden 实现位置与代码量

| 文件 | 行数 | 职责 |
|---|---|---|
| `cli/agent/bitwarden/crypto/kdf.go` | 64 | masterKey 派生（可直接照抄） |
| `cli/agent/bitwarden/crypto/keyhierarchy.go` | 110 | stretchKey + 解 userKey |
| `cli/agent/bitwarden/crypto/encstring.go` | 286 | EncString 解析/加解密 |
| `cli/agent/bitwarden/crypto/crypto.go` | 273 | AES-CBC + HMAC 原语 |
| `cli/agent/bitwarden/auth.go` | 373 | prelogin/token/refresh/2FA 编排 |
| `cli/agent/bitwarden/http.go` | ~110 | 表单/JSON POST、头 |
| `cli/agent/bitwarden/twofactor/*.go` | 233 | 2FA 选择（含 cgo FIDO2） |

可直接照抄：`kdf.go`、`keyhierarchy.go` 的 stretch 逻辑、`encstring.go`/`crypto.go`（AES-CBC/HMAC/EncString 是标准实现，可精简 memguard 部分）。`auth.go` 需重写（身份改桌面端、错误/2FA 流程重构）。

---

## C. 2FA 全方式

### C6 各方式在官方协议里的形态

- 2FA 响应：`400` + body 含 `TwoFactorProviders2`（map provider→数据）与 `TwoFactorProviders`（数组）（`api.service.ts:204-209`；`identity-two-factor.response.ts`）
- provider 常量：`Authenticator=0, Email=1, Duo=2, Yubikey=3, Remember=5, OrganizationDuo=6, WebAuthn=7, RecoveryCode=8`（`libs/common/src/auth/enums/two-factor-provider-type.ts`）
- **TOTP(0)**：客户端弹输入框，token = 6 位码，直接回填 `twoFactorToken`/`twoFactorProvider=0`
- **邮箱(1)**：先 `POST /two-factor/send-email-login`（带 `Email`、可选 `SsoEmail2FaSessionToken`、`MasterPasswordHash`），再回填码（`default-two-factor-api.service.ts:112`；CLI 逻辑 `apps/cli/src/auth/commands/login.command.ts:305-313`）
- **Duo(2/6)**：服务端 `TwoFactorProviders2[2]` 返回 `AuthUrl`（frameless URL）；桌面端通过 web vault 的 `duo-redirect-connector.html` 打开**系统浏览器**，回调返回 `code|state` 组成 token（`apps/desktop/src/auth/services/desktop-two-factor-auth-duo-component.service.ts`；`two-factor-auth-duo.component.ts`）
- **YubiKey(3)**：客户端读 YubiKey OTP 回填（无服务端前置）
- **WebAuthn(7)**：`TwoFactorProviders2[7]` 含 `challenge` 与 `allowCredentials`；官方桌面端构造 `WebAuthnIFrame`，iframe 指向 `{webVaultUrl}/webauthn-connector.html`（或新标签页 `webauthn-fallback-connector.html`），由**浏览器 WebAuthn API**（`navigator.credentials.get`）完成断言（`libs/common/src/auth/webauthn-iframe.ts:36-50`；`two-factor-auth-webauthn.component.ts`）。**官方桌面端没有原生 WebAuthn，走的是 web vault 页面**。
- token 成功响应里的 `TwoFactorToken` 是「记住此设备」的凭据（见 C8）。

### C7 goldwarden 覆盖度与纯 Go 落地路径

- 覆盖：TOTP(0)、Email(1)、WebAuthn(7)（仅 cgo FIDO2）、Api Key、passwordless
- 缺：Duo(2/6)、YubiKey(3)、RecoveryCode(8)、Remember(5)
- 证据：`twofactor/twofactor.go:17-41`（只处理 WebAuthn/Authenticator/Email）；provider 常量注释标 `Duo/YubiKey/... Not supported`

纯 Go 无浏览器可选路径：
| 方式 | 可行性 |
|---|---|
| TOTP / Email | 直接可行（纯输入） |
| YubiKey OTP | 用户读 OTP 粘贴即可，可行 |
| Duo | 需打开系统浏览器到 `AuthUrl` 回调取 code；纯 Go 可发请求但需用户交互，建议先用「打开浏览器 + 用户粘贴结果」兜底 |
| WebAuthn | 两条路：(a) 打开系统浏览器到 web vault `webauthn-fallback-connector.html`，用户完成后回填 token；(b) Windows 原生 `webauthn.dll`（`WebAuthNGetAssertion`）。后者 goldwarden 未做；本项目调研笔记 `docs/research/windows-hello-go.md` 已指出 `go-ctap/winhello` 是 webauthn.dll C API 的 cgo-free 绑定，可作为候选。**列为需实测验证** |

### C8 「记住此设备」机制

- 首次 2FA 成功时，若请求带 `twoFactorRemember=1`，token 响应返回 `TwoFactorToken`（`identity-token.response.ts` `twoFactorToken`）
- 客户端把它存到 token service（按 email 索引）：`libs/auth/src/common/login-strategies/login.strategy.ts:275-279` `tokenService.setTwoFactorToken(email, response.twoFactorToken)`
- 下次登录自动带上 `TokenTwoFactorRequest(Remember=5, storedToken, false)`：`login.strategy.ts:150-160` `buildTwoFactor`
- goldwarden 每次硬编码 `twoFactorRemember=1`（`auth.go:353`），但**不保存 `TwoFactorToken`**，所以「记住」对它无效（下次仍会要 2FA）
- **建议纳入**：biowarden 可把 `TwoFactorToken` 与 userKey 一起存（或单独存），下次登录跳过 2FA；代价是需要额外 store 一条 per-account 记录（可复用 `secrets` seam）

### C9 2FA 失败/取消的错误结构与重试语义

- 需要 2FA：HTTP `400` + body 含非空 `TwoFactorProviders2`（`api.service.ts:204-209`；goldwarden 用 `bytes.Contains(errsc.body, []byte("TwoFactor"))` 判断，`auth.go:160`）
- 2FA 校验失败：`400`，body 含 `ErrorModel.Message`（官方统一按 `ErrorResponse` 处理）
- WebAuthn 取消/超时：本地 toast `webauthnCancelOrTimeout`（`two-factor-auth-webauthn.component.ts`），即取消属本地错误，不是服务端错误码
- `invalid_grant`：`application/x-www-form-urlencoded` 且 `Accept: application/json` 时 IdentityServer 返回 OAuth 错误体（`error=invalid_grant`）；goldwarden 未区分
- 新设备验证：`400` + `ErrorModel.Message == NEW_DEVICE_VERIFICATION_REQUIRED_MESSAGE` → 需要 `newDeviceOtp`（`api.service.ts:210-213`；`password-token.request.ts` `newDeviceOtp`）
- goldwarden 的失败处理：`auth.go:160-169`（2FA→Perform2FA；captcha → 要求网页登录）；`Perform2FA` 失败直接返回 `could not login via two-factor`

---

## D. 工程与合规

### D10 LICENSE 与可搬边界

- goldwarden：**MIT**（`goldwarden/LICENSE:1` "MIT License, Copyright (c) 2023 Bernd Schoolmann"）
- biowarden：**当前仓库没有 LICENSE 文件**（仅 `README.md`/`AGENTS.md`，无 license 声明）→ 引用 MIT 代码时必须保留 MIT 版权与许可声明
- 边界建议：
  - **可搬（MIT，附版权声明）**：`crypto/kdf.go`、`crypto/keyhierarchy.go` 的 stretch/解钥部分、`crypto/encstring.go`、`crypto/crypto.go` 的 AES-CBC/HMAC
  - **参照重写**：`auth.go`（身份/流程须改成桌面端）、`http.go`、`twofactor/twofactor.go`（provider 集合与错误处理要重做）
  - **不可直接参考**：`twofactor/fido2twofactor.go`（cgo + 依赖 libfido2，且 license 上虽是 MIT 但技术上不可用于 no-cgo 构建）；其余 goldwarden 模块（memguard/keybase/sqlite 等）与本任务无关
- 建议给 biowarden 补一个 LICENSE（MIT 或 Apache-2.0），并在被搬运文件头保留 goldwarden 的 MIT 声明

### D11 依赖是否 cgo / Argon2id 语义

- goldwarden `go.mod` 依赖中需要 cgo 的：`github.com/keys-pub/go-libfido2`（FIDO2，链接 libfido2）、`github.com/keybase/client/go`（pinentry，无可避免时用 build tag）、`github.com/tink-crypto/...`（纯 Go）
- goldwarden 自身用 build tag `nofido2` 把 FIDO2 排除（`fido2twofactor.go:1` `//go:build !nofido2`；`fido2twofactorunsupported.go:1` `//go:build nofido2`），**证明其 KDF/HTTP/2FA 主体可 CGO_ENABLED=0 构建**
- 只取 KDF/HTTP/2FA（不含 FIDO2）：**可以保持纯 Go**，依赖仅 `golang.org/x/crypto`（argon2/pbkdf2/hkdf）
- Argon2id 语义一致性：`golang.org/x/crypto/argon2.IDKey(secret, salt, time, memory, threads, keyLen)` 中 `memory` 单位是 **KiB**；官方 `Argon2KdfConfig.memory` 单位是 **MiB**（校验信息 "Argon2 memory must be between 16 MiB and 1024 MiB"，`kdf-config.ts`），SDK 内部换算。goldwarden `Memory*1024`（`kdf.go:54`）= MiB→KiB，**与官方一致**

### D12 自托管

- goldwarden 用**四个独立 URL**：`ApiUrl / IdentityUrl / NotificationsUrl / VaultUrl`（`cli/agent/config/config.go:52-56`），默认值是 Bitwarden 官方四个不同主机（`config.go:92-95`）
- 用户**单独设**每个 URL（`actions/config.go` 的 `handleSetApiURL/SetIdentity/SetNotifications/SetVaultURL`），或用 `handleSetURLsAutomatically`：`GET {base}/api/config` → 读 `Environment.{Vault,Api,Identity,Notifications}`（`actions/config.go:89-127`）
- 官方客户端等价：用户填一个 web vault 地址，客户端读 `{base}/api/config` 推导 identity/api/events/vault（goldwarden 复刻的正是这一行为）
- Vaultwarden 兼容性：goldwarden 源码里**没有 Vaultwarden 专门分支**（全仓库 `rg -i vaultwarden` 无命中）；Vaultwarden 同样提供 `/api/config`、`/identity/connect/token`、`/identity/accounts/prelogin`（自托管时 identity 与 api 同源）。**兼容性差异需实测验证**

### D13 令牌刷新与登出

- 刷新：`POST {identity}/connect/token`，`grant_type=refresh_token` + `refresh_token` + `client_id`；官方 client_id 从 access token 解码得到（`api.service.ts` `refreshAccessToken`），goldwarden 硬编码 `"connector"`（`auth.go:317`）
- 过期处理：access token 有 `expires_in`（秒）；官方在 `expires_in` 到期前刷新（`token.service.ts` / `refreshAccessToken`）；goldwarden 保存 `ExpiresIn` 但不在请求前主动刷新，只在业务失败后 `RefreshToken`
- 登出：goldwarden `Config.Purge()` 只清本地字段（`config.go:157-172`），**不通知服务端**；官方客户端登出同样只清本地 token/state（`libs/auth/src/common/services/logout/`），**未查到时 `POST /connect/revocation` 的调用**（如服务端要吊销，需自行加，标注为需实测验证）

---

## E. 落地建议

### E14 建议包结构与文件清单

```
internal/auth/                 // 纯登录逻辑，无 UI 依赖
  client.go      // http.Client、头（Bitwarden-Client-Name=desktop 等）、表单/JSON POST   ~150 行
  identity.go    // prelogin、token(password)、refresh、2FA 提交、profile                    ~250 行
  kdf.go         // DeriveMasterKey(PBKDF2/Argon2id) + HashMasterPassword + StretchMasterKey  ~90 行
  keys.go        // EncString 解析 + AES-CBC/HMAC 解密 userKey（从 token.Key 解 64B）        ~200 行
  twofactor.go   // provider 选择 + TOTP/Email 交互 + WebAuthn/Duo 钩子                      ~150 行
  server.go      // 单地址 → 四个 base URL（/api/config 推导 + 手动覆盖）                    ~80 行
  login.go       // Login(email, password) → userKey，编排上面各步                            ~200 行
internal/auth/api/  // （可选）token 响应/错误结构体，就放 identity.go 亦可
internal/ui/    backend_login.go  // Login/Keys 页面接缝：调用 internal/auth + secrets.Put
internal/secrets/   // 复用：存 userKey + AccountKeys，并加一条可选 twoFactor token
```

与现有 seam 对接：`internal/auth` 只产出 `userKey`（64B），由 `internal/ui` 调 `secrets.Store.Put` 落库；任何 secret 只在内存中传递，不落盘不写日志（`hostlog` 已有脱敏）。

### E15 实现顺序与测试向量

1. **先做端到端最小闭环**：`server.go`（单地址推导）→ `kdf.go` → `identity.go`（prelogin+token，先不做 2FA）→ `keys.go`（stretch+解 userKey）。这样无 2FA 账号能最早跑通「登录→拿 userKey→写入 Credential Manager」
2. 再做 2FA：`twofactor.go`（TOTP/Email）
3. 最后做 refresh、Duo/WebAuthn 兜底、self-host/新 prelogin variant
4. 单测向量来源：官方 KDF 测试在 `bitwarden-clients` 的 `libs/legacy-crypto/src/services/*.spec.ts` 与 `libs/auth/src/common/login-strategies/*.spec.ts`；`password-prelogin` 的 model/api spec 在 `libs/common/src/auth/password-prelogin/*.spec.ts`。goldwarden 侧无独立 KDF 单测（`rg _test.go` 未命中 crypto 目录），需自建向量。**建议以「派生出的 masterKey 再 stretch 后能解开真实账号的 `Key`」作为验收向量**（真账号端到端，比单元向量更可靠）

### E16 风险清单

| 风险 | 说明 | 缓解 |
|---|---|---|
| 伪装 desktop 被拒 | 官方可能校验 `Bitwarden-Client-Name`/版本或 device 一致性；goldwarden 用 `web`/`firefox` 是已知可用的假身份 | 先按官方桌面端身份（`desktop`/`6`/`windows`）实现，若被拒回落 goldwarden 的 `web`/`firefox`；失败信号是 token 端点返回 400/invalid_client |
| WebAuthn 2FA 无浏览器 | 官方桌面端依赖 web vault 的 connector 页面，纯 Go 无 WebView 无法直接跑 | 先用系统浏览器 + 回填 token 兜底；原生 `webauthn.dll`（`go-ctap/winhello`）列为实验项，**需实测** |
| Argon2id 参数差异 | MiB vs KiB 单位、salt=sha256(email) 还是服务端 salt | 已确认 goldwarden `Memory*1024` 正确；新 prelogin 的 `Salt` 须优先使用、勿再 hash |
| 自托管兼容 | Vaultwarden 的 `/api/config`、prelogin variant、identity/api 同源/分源 | 支持「单地址 + /api/config 推导」+ 手动覆盖四个 URL；真机测 Vaultwarden |
| 新 prelogin variant 迁移 | 官方正迁移到 `/accounts/prelogin/password`（带 Salt） | 同时实现两个 endpoint，优先新 variant，失败回落旧 variant |
| 加密密钥迁移账号 | `response.key` 为空表示老账号需 key migration，官方已不支持 | 检测 `Key` 为空时报错并提示用户（goldwarden `encryptionKeyMigrationRequired` 对应） |

### 需实测验证清单（最多 3 处之外的集中列表）

1. Windows 原生 WebAuthn（webauthn.dll / go-ctap winhello）能否完成 2FA 断言 —— **需实测验证**
2. Vaultwarden 的 `/api/config` 与 prelogin variant 兼容性 —— **需实测验证**
3. 服务端是否调用 `/connect/revocation` 吊销 —— **未查到，需实测验证**

---

## 引用清单

**goldwarden（goldwarden）**
- `LICENSE:1` MIT
- `cli/agent/bitwarden/auth.go:27-32,53-67,71-176,179-254,256-338,342-356`
- `cli/agent/bitwarden/http.go:52-54,98-105`
- `cli/agent/bitwarden/crypto/kdf.go:18-19,50-54`
- `cli/agent/bitwarden/crypto/keyhierarchy.go:20-38,84-105`
- `cli/agent/bitwarden/crypto/encstring.go:22-28,126-160`
- `cli/agent/bitwarden/crypto/crypto.go:64-72`
- `cli/agent/bitwarden/twofactor/twofactor.go:17-41`、`fido2twofactor.go:1,12,40-110`、`fido2twofactorunsupported.go:1`
- `cli/agent/actions/login.go:87,107,132`、`cli/agent/actions/config.go:81-127`
- `cli/agent/config/config.go:52-56,92-95,157-172`、`cli/agent/bitwarden/sync.go:21`

**bitwarden-clients（bitwarden-clients）**
- `libs/common/src/auth/password-prelogin/password-prelogin-api.service.ts:19`、`password-prelogin.response.ts`、`password-prelogin.model.ts`
- `libs/common/src/services/api.service.ts:164-172,187,266,1324-1332,1498+`
- `libs/client-type/src/index.ts`（ClientType）、`libs/common/src/enums/device-type.enum.ts:1-38`
- `apps/desktop/src/platform/services/electron-platform-utils.service.ts:21-31,89-95`
- `libs/common/src/auth/models/request/identity-token/{token,password-token,device}.request.ts`
- `libs/common/src/auth/models/response/identity-token.response.ts`、`identity-two-factor.response.ts`、`refresh-token.response.ts`
- `libs/legacy-crypto/src/services/legacy-compat-key.service.ts:83-88,119-132`
- `libs/legacy-crypto/src/services/default-key-generation.service.ts:49-96`
- `libs/legacy-crypto/src/models/kdf-config.ts`（Argon2 MiB 校验）
- `libs/common/src/auth/enums/two-factor-provider-type.ts`、`libs/common/src/auth/two-factor/services/default-two-factor-api.service.ts`、`default-two-factor.service.ts:93-131`
- `libs/auth/src/common/login-strategies/login.strategy.ts:150-160,260-280`、`password-login.strategy.ts:90-200`
- `libs/auth/src/angular/two-factor-auth/two-factor-auth.component.ts`、`child-components/two-factor-auth-{duo,webauthn}/*`
- `libs/common/src/auth/webauthn-iframe.ts:36-50`
- `apps/desktop/src/auth/services/desktop-two-factor-auth-duo-component.service.ts`
- `apps/cli/src/auth/commands/login.command.ts:305-333`

**biowarden（本项目，只读了解接缝）**
- `internal/secrets/secrets.go`（`Store` seam）、`internal/hello/hello.go`（`Verifier` seam）
- `internal/ui/backend.go`（`keysBackend.save`）、`internal/host/legacy.go`、`cmd/biowarden/main.go`
- `docs/research/windows-hello-go.md`（go-ctap/winhello = webauthn.dll cgo-free 绑定的线索）

**keyguard（对照）**
- `desktopLibNative/src/src/biometrics/windows.rs`（Windows Hello：WinRT UserConsentVerifier + NCrypt，非 WebAuthn）
