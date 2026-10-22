# biowarden

[English](./README.md) · 简体中文

> Windows 上的 Bitwarden 浏览器扩展「生物识别解锁」独立工具 - 用一个 exe 顶替官方桌面客户端。
> 直接在 biowarden 里登录账号，再用 Windows Hello 放行账号密钥。

biowarden 是 [WardenBio](https://github.com/xVanTuring/WardenBio) 的 Windows 移植。它自带一个
native messaging host（与主程序同一个 exe，由浏览器按需拉起）。在浏览器扩展眼里它就是桌面端：扩展发起
生物识别解锁时，biowarden 弹出 Windows Hello 验证，通过后从 Windows 凭据管理器取出该账户的加密密钥，
在扩展建立的通道上回传。

现在不再需要从扩展控制台里复制脚本了。在一台干净的机器上，biowarden 会自己登录你的 Bitwarden 账号、
派生出账号密钥、在 Windows Hello 确认后写入凭据管理器，扩展里的「Unlock with biometrics」即可使用。
旧的提取脚本仍保留在账号页的「高级」折叠区，作为故障排查与兼容路径。

协议参考 [quexten/bw-bio-handler](https://github.com/quexten/bw-bio-handler) 与
[quexten/goldwarden](https://github.com/quexten/goldwarden)（Bitwarden 桌面 <-> 浏览器 IPC 协议的子集实现）。
本项目非 Bitwarden 官方项目，相关解锁问题请勿反馈给 Bitwarden 团队。

## 工作原理

```
浏览器扩展 -stdio(native messaging)-> biowarden host -> Windows Hello -> 凭据管理器
```

- 一个 exe，两种模式。stdin 是管道时按 native messaging host 运行（浏览器按需拉起）；其他方式启动则打开
  GUI - 托盘加一个左侧栏窗口，四个页面：**账号** / **浏览器** / **设置** / **日志**。
- 登录链路是真实的。你在登录窗里输入邮箱和主密码，biowarden 伪装官方桌面客户端
  （`client_id=desktop`、`deviceType=6`、`deviceName=windows`）与 Bitwarden 的身份与 API 端点通信：
  先走 prelogin 读取 KDF 参数（PBKDF2-SHA256 或 Argon2id），再请求 token、派生主密钥、解出被封装的
  64 字节 userKey，最后才写入凭据管理器。Windows Hello 在写入前放行，因此拒绝弹窗不会留下任何数据。
- 与扩展之间两条通道：
  - 旧命令协议（当前主路径）：`setupEncryption` 用 RSA-OAEP-SHA1 把一个 64 字节传输密钥交给 host；
    之后全部是 AES-256-CBC + HMAC，每条 `message` 必须同时带 `encryptedString`
    （`2.<iv>|<data>|<mac>`）和 iv/data/mac
  - 新版桌面 IPC，仅在扩展远端开关 `BiometricsSDKIPC` 放量后启用：`Noise_NN_P256_AESGCM_SHA256`
    握手，Noise 帧封在 CBOR 里、载荷走 AES-256-GCM，之后是 JSON RPC（`DiscoverRequest`、
    `GetBiometricsStatus`、`UnlockBiometrics`、`AuthenticateBiometrics`），应答必须套
    `forwarded-bitwarden-ipc-message` 信封
  - 扩展会同时开这两条连接，所以一个浏览器出现两个 host 进程属正常
- 密钥以 `Biowarden/<userId>` 存在 Windows 凭据管理器里，每个账号一条，另有 `Biowarden/Index`
  元数据条目。GUI 与 host 共用这个存储，所以 GUI 里登录保存的密钥就是 host 解锁时读取的那条。
- 日志在 `%LOCALAPPDATA%\biowarden\host.log`（5 MB 轮转，密钥字段一律脱敏）；设 `BIOWARDEN_LOG=debug`
  可看到协议级细节。
- 界面语言跟随系统（英文、简体中文、繁体中文、日文、德文、法文或西班牙文），可在**设置**页的**语言**项里覆盖。

**安全模型：Windows Hello 只是读取密钥前的访问控制，它不加密密钥。** 账号密钥以原始字节存在 Windows
凭据管理器里。通过 Hello 只代表 biowarden 获准读取该条目 - 存储的密钥没有任何成分来自你的生物特征或 PIN。
同一用户下的其他进程原则上也能读到这条凭据；这在使用官方桌面客户端时同样是既成事实的威胁模型。

登录窗里输入的主密码只在内存里。它会在本次运行期间保留，供登录/重新登录在改密码或改 KDF 后
重新派生密钥使用，绝不落盘、不进日志。access token 同样只在内存。为让重启后仍保持登录，biowarden 会把
refresh token、邮箱和服务器地址作为独立的 `Biowarden/Session/<userId>` 条目持久化，由 Windows DPAPI 按当前
用户加密；该条目不含主密码，也不含账号密钥。两步验证码在单次尝试结束后即清除。由于只保留 refresh token，
刷新密钥仍需重新输入主密码。干净地「退出登录」会在 Windows Hello 确认后删除已保存的密钥与登录会话。

## 安装

1. 从本仓库 Releases 页下载 `biowarden Setup <版本> amd64.exe` 运行安装（按用户安装）。Releases 上只发布
   安装器，另附一个 `SHA256SUMS.txt`，可用 `sha256sum -c SHA256SUMS.txt` 校验下载的文件。
2. 启动 biowarden。在干净的机器上它会自行配置；见下面「使用步骤」。
3. 想自己编译见「从源码构建」。

要求 Windows 10 1709 及以上，并已设置 Windows Hello。

## 使用步骤

1. **首启会注册你的浏览器。** 当本机没有保存任何密钥、且没有任何浏览器注册到 biowarden 时，首启会通过各
   浏览器的 App Paths 条目探测已安装的浏览器，并为每个找到的浏览器写入 manifest。这每次安装只发生一次，
   标记在 `HKCU\Software\biowarden`。若某个注册可能属于其他客户端（官方桌面端），会被**跳过而不覆盖**，
   浏览器页的摘要在会点名它；想接管就到对应行做二次确认。之后的启动什么都不做，也不会再弹登录窗。
2. **登录。** 首启会打开登录窗。输入邮箱和主密码。服务器地址在「高级」里，默认是 Bitwarden 官方云，也支持
   自托管地址。两步验证支持 **TOTP（验证器应用）、邮箱验证码、YubiKey**。**WebAuthn 与 Duo 暂不支持** -
   若你的账号依赖这两种方式，请取消登录，改用下面第 5 步的后备提取脚本。
3. **Windows Hello 确认。** 登录成功后，biowarden 会请求 Windows Hello 确认，通过后把账号密钥写入凭据
   管理器。拒绝则不会保存任何东西。
4. **在扩展里开启生物识别。** 在 Bitwarden 扩展的 设置 -> 安全 中开启「Unlock with biometrics」
   （会授予 `nativeMessaging` 权限并重载扩展）。**安装/修复 manifest 后需完全重启浏览器** - 浏览器只在
   启动时读 `NativeMessagingHosts` 注册表。
5. **锁定并解锁。** 锁定扩展，点解锁：弹出 Windows Hello（提示已居中屏幕中央），通过即解锁。

### 故障排查与兼容：提取脚本

手动路径保留给登录流程覆盖不到的场景：账号受 WebAuthn 或 Duo 保护、自托管服务器的端点 biowarden 连不上、
或登录出问题需要诊断。

在 **账号** 页展开「高级」。点 **复制提取脚本** 会复制一段脚本，在扩展处于解锁状态下粘贴到扩展的后台控制台
（Chrome：`chrome://extensions` -> Bitwarden 的「服务工作进程」；Firefox：`about:debugging`）运行，输出
`{userId, keyB64}`。Bitwarden 2026.8.x 的键在 session 存储（`{__json__}` 包装）里，或在「从不锁定」策略下
的 local `<userId>_user_auto`，脚本两种都兼容；找不到时会打印原因（扩展处于锁定、未开启「从不锁定」策略，
或存储结构变了）。把两个值粘回同一页面保存，并按 Windows Hello 确认。

## 已知限制

- **官方 Bitwarden 桌面客户端会与 biowarden 冲突。** 它每次启动都会无条件重写全部浏览器的
  `NativeMessagingHosts` 注册表键，把注册抢回去。**浏览器** 页会显示红色「已被其他程序覆盖」并支持
  一键恢复，但根本办法是不要同时运行官方桌面客户端。
- **manifest 变更后需完全重启浏览器。** 浏览器只在启动时读注册表值，所以新装或修复要到下次浏览器启动才生效。
- **Chromium 读独立注册表键。** Chrome 的安装会同时写 `Google\Chrome` 与 `Chromium` 两个键；2026.1 之前的
  biowarden 版本只写前者。
- **只有零配置首启会自动注册。** 之后新装的浏览器要在浏览器页手动配置；后续启动不会再自动注册任何东西。
- **WebAuthn 与 Duo 两步验证不支持。** biowarden 没有内嵌 web vault，也不抢注 `bitwarden://` 协议，
  两种 connector 的回调都到不了这里。依赖它们的账号需要用提取脚本后备。
- 提取脚本依赖扩展的内部存储结构（`chrome.storage.session` 的 `user_<id>_crypto_userKey`）。
  Bitwarden 扩展大版本更新可能改变它；脚本找不到时会明确说明原因，而不是静默失败。
- v2 有 Chrome、Chromium、Edge、Firefox 四个注册点，其中 Edge 与 Firefox 的真机验收暂缓。
  Brave、Vivaldi、Safari 等浏览器不支持。
- host 进程随连接常驻（每个浏览器两条连接、两个进程），锁定弹窗期间扩展每秒轮询一次属正常。
- 解锁请求在 Hello 弹窗期间阻塞该连接，与官方桌面客户端行为一致。

## 从源码构建

依赖：Go 1.27+，以及做安装器所需的 NSIS（Windows 上缺 `makensis` 时 `mygo build` 会自动把
NSIS 3.13 下到 `%LOCALAPPDATA%\mygo`）。

```powershell
pwsh scripts/build.ps1
```

产物在 `build/windows-amd64/`：`biowarden.exe`、`biowarden Setup 0.1.0 amd64.exe`、`icon.png`。
脚本先构建到 staging 目录，再停掉运行中的 biowarden（GUI 与 host），然后换上新构建 - 运行中的 exe 可以
改名但不能覆盖。没有任何进程占用时，直接 `go tool mygo build` 也可以。

版本号在两处，发版时必须同步：`mygo.json` 的 `version`（构建会把它链进二进制并用它命名安装器）与
`internal/ui/ui.go` 的 `appVersion`（显示在侧边栏底部与设置页）。当前两者都是 `0.1.0`。

```sh
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
```

更多构建细节见 [docs/build.md](./docs/build.md)。

## 卸载

- **浏览器** 页：对每个已配置的浏览器点「卸载」（删除 manifest 与注册表值）
- **账号** 页：删除各账户密钥（需 Hello 确认），或用「退出登录」
- 退出 biowarden（托盘菜单 Exit），删除程序与安装器

## 免责声明

非 Bitwarden 官方项目，与 Bitwarden 及 8bit Solutions LLC 无任何关联。解锁问题请反馈到本仓库，
不要提给 Bitwarden 的 issue 追踪。录入的密钥等同于保险库的加密密钥，请勿在任何地方明文留存。
使用风险自负。

## 目录结构

```
cmd/biowarden/          双模式入口（模式判定、分发）
internal/auth/          Bitwarden 登录链路（prelogin/KDF/token/userKey，TOTP/邮箱/YubiKey 2FA）
internal/protocol/      帧、旧命令协议、桌面端 IPC（Noise/CBOR/RPC）
internal/host/          serve 循环：分发、解锁流程、日志
internal/hoststate/     每个浏览器的连接历史（哪个浏览器拉起了 host）
internal/{hello,secrets,registry,autostart}/   平台接缝
internal/ui/            GUI（登录窗、首启配置、真实账号后端、各页面）
```

参考实现：WardenBio（macOS/Swift）、bw-bio-handler、goldwarden。

## 致谢

biowarden 基于他人的工作构建：

- [goldwarden](https://github.com/quexten/goldwarden) - MIT，Copyright (c) 2023 Bernd Schoolmann。
  登录、KDF 与加密实现参考并移植了其中部分代码。
- [WardenBio](https://github.com/xVanTuring/WardenBio) - biowarden 移植的 macOS/Swift 实现。
- [bw-bio-handler](https://github.com/quexten/bw-bio-handler) - 旧命令协议。

依赖及其许可证：

- [github.com/danieljoos/wincred](https://github.com/danieljoos/wincred) v1.2.3 - MIT
- [github.com/egoist/mygo](https://github.com/egoist/mygo) v0.4.8 - MIT
- [golang.org/x/sys](https://cs.opensource.google/go/x/sys) v0.48.0 - BSD-3-Clause
- [github.com/ebitengine/purego](https://github.com/ebitengine/purego) v0.11.1 - Apache-2.0
- [github.com/go-text/typesetting](https://github.com/go-text/typesetting) v0.3.5 - Unlicense 或
  BSD-3-Clause
- [golang.org/x/image](https://cs.opensource.google/go/x/image) v0.46.0 - BSD-3-Clause

完整许可证文本与版权行见 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。
