# Biowarden

[![Latest release](https://img.shields.io/github/v/release/windedge/biowarden)](https://github.com/windedge/biowarden/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

[English](./README.md) · 简体中文

Windows 上的 Bitwarden 浏览器扩展「生物识别解锁」独立工具 - 用一个 exe 顶替官方桌面客户端。
在 Biowarden 里登录账号，由 Windows Hello 放行账号密钥。支持深色与浅色外观、七种界面语言，
账号密钥不会以明文写入任何地方。本项目非 Bitwarden 官方项目，解锁问题请勿反馈给 Bitwarden 团队。

**Windows Hello 只放行「读取」密钥，它不加密密钥。** 密钥以原始字节存在 Windows 凭据管理器里；
通过验证只表示 Biowarden 可以去读它。这与官方桌面客户端的威胁模型一致。

| 浅色 | 深色 |
|---|---|
| ![浅色外观下的浏览器页](./docs/images/screenshot-light.png) | ![同一页面的深色外观](./docs/images/screenshot-dark.png) |

## 安装

1. 从 [Releases](https://github.com/windedge/biowarden/releases) 下载
   `biowarden-setup-<版本>-amd64.exe` 运行安装（按用户安装）。Releases 上只发布安装器，另附一个
   `SHA256SUMS.txt`。
2. 用它与下载文件比对：

   ```powershell
   (Get-FileHash .\biowarden-setup-<版本>-amd64.exe).Hash   # 与 SHA256SUMS.txt 里的值对照
   ```

3. 启动 Biowarden。在干净的机器上它会自行注册浏览器并弹出登录窗。

程序未做代码签名，首次运行会出现 SmartScreen 的「Windows 已保护你的电脑」：点**更多信息** ->
**仍要运行**。上面的哈希校验就是你防篡改的手段。需要 Windows 10 1709 或更高版本，并已配置
Windows Hello。

## 使用

1. **登录** - 在登录窗输入邮箱与主密码。服务器地址在「高级」里，默认官方 Bitwarden 云，
   也接受任何自托管地址（含 Vaultwarden）。支持 TOTP、邮箱验证码和 YubiKey，不支持 WebAuthn 与 Duo。
2. **Windows Hello 确认** - 这个提示才是写入密钥的动作。拒绝则什么都不留下。
3. **在扩展里启用生物识别** - 设置 -> 安全 -> 「使用生物识别解锁」，然后**完整重启浏览器**；
   浏览器只在启动时读注册项。

之后在扩展里正常锁定与解锁即可。账号页的**退出登录**会删除已存密钥与会话。

解锁时弹窗不出现？[故障排查](./docs/troubleshooting.md) 开头就是三种常见原因，之后才是提取脚本兜底。

## 文档

详细文档为英文：

| | |
|---|---|
| [工作原理](./docs/how-it-works.md) | 协议、密钥存放、安全模型、目录结构 |
| [故障排查](./docs/troubleshooting.md) | 弹窗不出现、提取脚本、已知限制、卸载 |
| [从源码构建](./docs/build.md) | 构建、打包、发版 |
| [安全策略](./SECURITY.md) | 如何报告漏洞、哪些在范围内 |
| [第三方来源](./THIRD_PARTY_NOTICES.md) | 许可证与移植来源 |
| [参与贡献](./CONTRIBUTING.md) | 如何报告、构建与提交改动 |

## 关于连接名

host 注册在 `com.8bit.bitwarden` 名下，因为这个名字硬编码在扩展里。接管这一项注册，就是
「不用 fork 也不用改扩展」的全部代价。已经存在的注册会显示成「可能是官方客户端」，只有你确认后
才覆盖。

## 免责声明

与 Bitwarden 及 8bit Solutions LLC 无任何关联。Bitwarden 是 Bitwarden Inc. 的商标；这里出现
`com.8bit.bitwarden` 这个连接名，只是因为浏览器扩展要求必须叫这个名字。录入的密钥等同于保险库的
加密密钥，请勿在任何地方明文留存。使用风险自负。以 [MIT 许可证](./LICENSE) 发布。
