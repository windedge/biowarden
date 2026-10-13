# v1 验收清单（工单 #14）

对着地图终点的验收：三浏览器 × 两通道 × 全流程，真实扩展、真实 Windows Hello。
逐项打勾并记现象；任何一步失败，记下 host.log 尾部与扩展后台 console 的报错。

前置：`go tool mygo build` 产出的 `build/windows-amd64/biowarden.exe`（或安装 NSIS 安装器后用安装版）。

## 0. 基础冒烟（不需浏览器）

- [ ] 双击 biowarden.exe → 托盘出现蓝环图标，窗口打开四页
- [ ] 关闭窗口 → 窗口隐藏、托盘仍在；托盘菜单 Exit → 进程真正退出（任务管理器确认）
- [ ] 再次启动两个实例 → 第二个自动置前第一个的窗口后退出（单实例锁）
- [ ] 设置页：自启动开关切换后，`reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v dev.windedge.biowarden` 出现/消失
- [ ] 设置页：Windows Hello 显示「可用」
- [ ] `printf '' | biowarden.exe` → 输出 `{"command":"connected",...}` 后退出（host 模式判据）

## 1. 密钥录入（每浏览器共用一次）

- [ ] Bitwarden 扩展处于已解锁状态
- [ ] Chrome/Edge：`chrome://extensions` → 开发者模式 → Bitwarden → 「服务工作进程」打开控制台
- [ ] Firefox：`about:debugging#/runtime/this-firefox` → Bitwarden → 「检查」
- [ ] 密钥页「复制提取脚本」→ 粘贴到控制台回车 → 输出 `{userId, keyB64}`
- [ ] 粘贴两个值 → 「保存并验证」→ 弹出 Windows Hello → 刷脸/指纹通过 → 账户进列表，notice 绿色
- [ ] 「测试解锁」→ 再弹一次 Hello → 通过
- [ ] 列表行显示 userId（截断+悬停全文）与创建时间

## 2. 浏览器注册（每浏览器一遍）

- [ ] 浏览器页：Chrome/Edge/Firefox 三行状态与真实注册表一致
  - 本机预期：Chrome=已安装（biowarden）【若之前装过官方则可能为 foreign】、Edge=已存在（可能官方配置，指向 scoop 的 Bitwarden）、Firefox=未安装
- [ ] Edge 行点「覆盖安装」→ 出现二次确认 → 「确认覆盖」→ 状态变已安装（biowarden）
  - [ ] 确认 `HKCU\Software\Microsoft\Edge\NativeMessagingHosts\com.8bit.bitwarden` 指向 `%LOCALAPPDATA%\biowarden\manifests\edge.json`
- [ ] Firefox 行「安装」→ 状态变已安装
- [ ] 卸载某浏览器 → 状态回未安装；对 foreign 行（若有）卸载被拒、提示「可能属于官方」

## 3. 解锁验收（核心：三浏览器 × 两通道）

扩展设置 → 安全 → 启用「解锁时使用生物识别」（每个浏览器都要启用一次）。
锁定扩展（自动锁定或手动锁定），然后：

| 浏览器 | 旧命令通道（默认） | SDK IPC 通道（BiometricsSDKIPC 开关放量时） |
|---|---|---|
| Chrome | 锁定后点扩展图标 → 弹 Windows Hello → 通过后扩展解锁 | 同左（若该浏览器分到开关） |
| Edge | 同上 | 同上 |
| Firefox | 同上 | 同上 |

- [ ] 每格：Hello **通过** → 扩展解锁成功，能进密码库
- [ ] 每格：Hello **取消**（Esc/关窗）→ 扩展保持锁定，弹窗不转圈（转圈 = 协议应答形状错，立即停记现象）
- [ ] 拒绝后立刻重试 → 仍能再弹 Hello
- [ ] 三浏览器各自 host.log（`%LOCALAPPDATA%\biowarden\host.log`）有完整记录且**搜不到 keyB64 明文**（脱敏断言）
- [ ] 录入的账户在 GUI 删除（需 Hello 确认）→ 扩展再解锁应失败（密钥已删）

## 4. 通道确认

- [ ] 在扩展后台 console 观察走的是哪条通道（sdkipc 信封 vs 旧命令），记录下来
- [ ] 若走旧命令：确认 `setupEncryption` 后的解锁正常；若走 SDK IPC：确认 Noise 握手日志无错误

## 5. 冲突与恢复

- [ ] 装有官方 Bitwarden 桌面客户端的浏览器：覆盖前后官方「Unlock with biometrics」行为对比（覆盖后官方通道失效属预期，README 已声明）
- [ ] 移动 exe 位置 → 浏览器页「修复」→ manifest 路径重写、解锁恢复
- [ ] `BIOWARDEN_LOG=debug biowarden.exe`（host 由浏览器拉起时如何注入：设用户环境变量后重启浏览器）→ 日志级别变化

## 退出标准

全部勾项通过 → 结单 #14，开始 #15（README 定稿）。
任何「转圈」/「解锁失败但 Hello 已通过」→ 记录浏览器+扩展版本+host.log，回协议工单排查。
