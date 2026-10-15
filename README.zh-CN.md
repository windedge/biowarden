# biowarden

[English](./README.md) · 简体中文

> Windows 上的 Bitwarden 浏览器扩展「生物识别解锁」独立工具 - 用一个 exe 顶替官方桌面客户端，用 Windows Hello 放行账号密钥。

biowarden 是 [WardenBio](https://github.com/xVanTuring/WardenBio) 的 Windows 移植。它自带一个
native messaging host（与主程序同一个 exe，由浏览器按需拉起）。在浏览器扩展眼里它就是桌面端：扩展发起
生物识别解锁时，biowarden 弹出 Windows Hello 验证，通过后从 Windows 凭据管理器取出该账户的加密密钥，
在扩展建立的通道上回传。

协议参考 [quexten/bw-bio-handler](https://github.com/quexten/bw-bio-handler) 与
[quexten/goldwarden](https://github.com/quexten/goldwarden)（Bitwarden 桌面 <-> 浏览器 IPC 协议的子集实现）。
本项目非 Bitwarden 官方项目，相关解锁问题请勿反馈给 Bitwarden 团队。

## 工作原理

```
浏览器扩展 -stdio(native messaging)-> biowarden host -> Windows Hello -> 凭据管理器
```

- 一个 exe，两种模式。stdin 是管道时按 native messaging host 运行（浏览器按需拉起）；其他方式启动则打开
  GUI - 托盘加一个四页窗口：**浏览器** / **密钥** / **设置** / **日志**。
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
  元数据条目。
- 日志在 `%LOCALAPPDATA%\biowarden\host.log`（5 MB 轮转，密钥字段一律脱敏）；设 `BIOWARDEN_LOG=debug`
  可看到协议级细节。

**安全模型：Windows Hello 只是读取密钥前的访问控制，它不加密密钥。** 账号密钥以原始字节存在 Windows
凭据管理器里。通过 Hello 只代表 biowarden 获准读取该条目 - 存储的密钥没有任何成分来自你的生物特征或 PIN。
同一用户下的其他进程原则上也能读到这条凭据；这在使用官方桌面客户端时同样是既成事实的威胁模型。

## 安装

1. 从本仓库 Releases 页下载 `biowarden Setup <版本> amd64.exe` 运行安装（按用户安装），或下载免安装的
   `biowarden.exe` 放到固定位置 - manifest 里记的是 host 的绝对路径，**换位置后要重新「安装」一次**。
2. 启动 biowarden，按下面「使用步骤」配置。
3. 想自己编译见「从源码构建」。

要求 Windows 10 1709 及以上，并已设置 Windows Hello。

## 使用步骤

1. 运行 `biowarden.exe`，在 **浏览器** 页为你的浏览器点「安装」。Chrome 的安装会同时写
   `Google\Chrome` 与 `Chromium` 两个注册表键；2026.1 之前的 biowarden 版本只写前者。
2. 在扩展处于解锁状态下，点 **密钥** 页的「复制提取脚本」，把脚本粘贴到扩展的后台控制台
   （Chrome：`chrome://extensions` -> Bitwarden 的「服务工作进程」；Firefox：`about:debugging`），
   运行后得到 `{userId, keyB64}`。Bitwarden 2026.8.x 的键在 session 存储（`{__json__}` 包装）里，
   或在「从不锁定」策略下的 local `<userId>_user_auto`，脚本两种都兼容，找不到时会打印原因。
   把两个值粘回 **密钥** 页保存，并按 Windows Hello 确认。
3. 在 Bitwarden 扩展的 设置 -> 安全 中开启「Unlock with biometrics」（会授予 `nativeMessaging`
   权限并重载扩展）。**安装/修复 manifest 后需完全重启浏览器** - 浏览器只在启动时读
   `NativeMessagingHosts` 注册表。
4. 锁定扩展，点解锁：弹出 Windows Hello（提示已居中屏幕中央），通过即解锁。

## 已知限制

- **官方 Bitwarden 桌面客户端会与 biowarden 冲突。** 它每次启动都会无条件重写全部浏览器的
  `NativeMessagingHosts` 注册表键，把注册抢回去。**浏览器** 页会显示红色「已被其他程序覆盖」并支持
  一键恢复，但根本办法是不要同时运行官方桌面客户端。
- 提取脚本依赖扩展的内部存储结构（`chrome.storage.session` 的 `user_<id>_crypto_userKey`）。
  Bitwarden 扩展大版本更新可能改变它；脚本找不到时会明确说明原因，而不是静默失败。
- v1 只覆盖 Chrome、Chromium、Edge、Firefox 四个注册点，其中 Edge 与 Firefox 的真机验收暂缓。
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

```sh
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && go test ./...
```

## 卸载

- **浏览器** 页：对每个已配置的浏览器点「卸载」（删除 manifest 与注册表值）
- **密钥** 页：删除各账户密钥
- 退出 biowarden（托盘菜单 Exit），删除程序与安装器

## 免责声明

非 Bitwarden 官方项目，与 Bitwarden 及 8bit Solutions LLC 无任何关联。解锁问题请反馈到本仓库，
不要提给 Bitwarden 的 issue 追踪。录入的密钥等同于保险库的加密密钥，请勿在任何地方明文留存。
使用风险自负。

## 目录结构

```
cmd/biowarden/          双模式入口（模式判定、分发）
internal/protocol/      帧、旧命令协议
internal/protocol/sdkipc/   桌面端 IPC（Noise/CBOR/JSON RPC）
internal/host/          serve 循环：分发、解锁流程、日志
internal/{hostlog,hello,secrets,registry,autostart}/   平台接缝
internal/ui/            GUI（托盘 + 浏览器/密钥/设置/日志四页）
```

参考实现：WardenBio（macOS/Swift）、bw-bio-handler、goldwarden。
