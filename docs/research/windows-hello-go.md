> Research notes, kept for reference. Not user documentation; details may have moved since.

# Windows Hello 纯 Go（no cgo）实现调研

结论先行：**自写 ~300 行纯 syscall 实现是正确路径**，没有可直接复用的现成 Go 库；所有 API 序列、IID、等待模式都已从一手来源核实，风险从「未知」降为「照清单施工」。

## 1. 现成 Go 库盘点

| 库 | 结论 |
|---|---|
| go-ctap/winhello | **语义不符**。绑定的是 webauthn.dll 的 C API（WebAuthn make-credential/get-assertion，CTAP 语义），不是 UserConsentVerifier（纯同意提示）。其价值在 hiddenwindow 包（纯 syscall 隐藏窗口 + 消息泵样板，529 行）与 `cgo-free implementation` 的先例。https://github.com/go-ctap/winhello |
| julian-bruyers/winhello-go | **排除**。靠自带 native C++ DLL 桥接，违背 no-cgo/单 exe 约束。https://github.com/julian-bruyers/winhello-go |
| yashikota/bio | **排除**。FIDO2/WebAuthn 语义 + macOS 依赖 purego，与 UserConsentVerifier 无关。https://github.com/yashikota/bio |
| goldwarden | 官方 issue #4 确认 Windows Hello 至今 `Missing Golang Binding`，无后续实现可抄。https://github.com/quexten/goldwarden/issues/4 |

## 2. 要调的 API 序列（全部一手核实）

```
combase.dll:
  WindowsCreateString(utf16, len, *HSTRING)          // HSTRING 创建
  WindowsDeleteString(HSTRING)
  RoInitialize(RO_INIT_MULTITHREADED)                 // 每线程一次，配对 RoUninitialize
  RoGetActivationFactory(hstring(RuntimeClass_Name), IID, **factory)
  RoUninitialize

user32.dll:  CreateWindowExW / RegisterClassExW / GetMessageW / DispatchMessageW ...  // 隐藏窗口宿主

kernel32.dll: CreateEventW / SetEvent / WaitForSingleObject / CloseHandle            // 等异步完成
```

1. 在调用线程 `RoInitialize(RO_INIT_MULTITHREADED)`（keyguard windows.rs:91 同款；S_FALSE/RPC_E_CHANGED_MODE 皆算成功，keyguard 用 Rc 守卫配对释放）
2. `RoGetActivationFactory(L"Windows.Security.Credentials.UI.UserConsentVerifier", &IID_IUserConsentVerifierInterop, &interop)`
   - **IID_IUserConsentVerifierInterop = `39E050C3-4E74-441A-8DC0-B81104DF949C`**（Windows SDK 10.0.26100 `um/UserConsentVerifierInterop.h`；vtable 槽位：QI=0, AddRef=1, Release=2, GetIids=3, GetRuntimeClassName=4, GetTrustLevel=5, **RequestVerificationForWindowAsync=6**；NTDDI >= WIN10_RS3，即 Win10 1709+）
   - 签名：`(HWND appWindow, HSTRING message, REFIID riid, void** asyncOperation)`
3. `interop->RequestVerificationForWindowAsync(hwnd, msg, IID_IAsyncOperation<UserConsentVerificationResult>, &op)`
   - **泛型实例 IID = `FD596FFD-2318-558F-9DBE-D21DF43764A5`**（SDK `winrt/windows.security.credentials.ui.h:269` `IAsyncOperation<UserConsentVerificationResult>` 特化 uuid）
   - **Completed handler IID = `0CFFC6C9-4C2B-5CD4-B38C-7B8DF3FF5AFB`**（同文件 :294 `AsyncOperationCompletedHandler<UserConsentVerificationResult>` 特化）
4. 等 `IAsyncOperation` 完成（无需消息泵！）：`IAsyncOperation<T>` vtable 槽位：QI=0, AddRef=1, Release=2, GetIids=3, GetRuntimeClassName=4, GetTrustLevel=5, **put_Completed=6, get_Completed=7, GetResults=8**（`IAsyncInfo` 是另一张 vtable 的独立接口，不在这张表上，本实现用不到）
   - windows-rs `join()` 的实现（crates/libs/future/src/async.rs + waiter.rs）：
     ```
     if get_Status() == Started:
         event = CreateEventW()
         put_Completed(handler{ SetEvent(event) })    // handler 是自实现 COM 对象（Invoke(this, op, status)）
         WaitForSingleObject(event, INFINITE)
     return GetResults()
     ```
   - Completed 回调在 WinRT 线程池线程触发，**不需要**窗口消息泵，也不阻塞 host 的 stdin 读循环（goroutine 里等待即可）
   - **put_Completed 会对传入的 handler 做 QI，要求它实现该操作专属的 completed-handler IID**：验证路径 `0CFFC6C9-4C2B-5CD4-B38C-7B8DF3FF5AFB`，可用性路径 `28988174-ACE2-5C15-A0DF-580A26D94294`（SDK 头文件核对）。handler 的 QueryInterface 不认它就会拿到 `E_NOINTERFACE (0x80004002)`，表现为 `put_Completed failed`。实现期实测踩过。
5. 结果映射（`UserConsentVerificationResult`，数值以 SDK `winrt/windows.security.credentials.ui.h` 为准；本笔记初版曾串位，已按实测纠正）：
   - 0 Verified → 成功
   - 1 DeviceNotPresent / 2 NotConfiguredForUser / 3 DisabledByPolicy → Unavailable（Hello 未配置）
   - 4 DeviceBusy → Busy（设备忙，可重试）
   - 5 RetriesExhausted → Locked（多次尝试，Hello 暂时锁定）
   - 6 Canceled → Canceled（用户取消）

## 3. 可直接照抄的本地参照

| 参照 | 用途 |
|---|---|
| `keyguard-app\desktopLibNative\src\src\biometrics\windows.rs`（Rust 814 行） | API 序列、RoInitialize 守卫、结果映射的权威样板 |
| `mygo\internal\windows\com.go`（165 行） | `comCall`/`queryInterface`/`addRef`/`release`/`newHandler`（单方法 COM 回调对象，WebView2 事件在用，成熟模式） |
| `mygo\internal\windows\app.go:273` | CreateWindowExW 隐藏窗口宿主样板 |
| WardenBio `SecretStore.swift` | 验证流程语义（先 Hello 后读钥匙串） |

## 4. 自写实现的形状（预估 ~300 行）

```
internal/hello/hello.go          // 接口: Verifier{Available() bool; Verify(reason string) (Result, error)}
internal/hello/hello_windows.go  // syscall 实现
  - combase: WindowsCreateString/WindowsDeleteString/RoInitialize/RoGetActivationFactory/RoUninitialize
  - IID 常量 ×3（interop / IAsyncOperation 特化 / CompletedHandler 特化）
  - completedHandler COM 对象: 三槽 IUnknown + Invoke（照 mygo newHandler 模式扩到 4 槽）
  - Verify(): hidden hwnd（复用 mygo 窗口或 winhello hiddenwindow 思路）→ interop 调用 → join 模式等待 → GetResults 映射
```

- COM 调用约定：mygo com.go 的 `comCall(obj, i, args...)` 已带 `go:uintptrescapes`（指针在调用期间保持有效），直接照抄此模式
- `native()` 是 mygo 内部函数，biowarden 侧不需要（无 WOW64 场景，指针直接传）
- 隐患清单（PoC 时验证）：a) host 模式无窗口线程，隐藏窗口 + Hello 弹窗的宿主关系（keyguard 传 JVM 窗口 HWND；winhello 传隐藏窗口）；b) RO_INIT_MULTITHREADED 在 GUI 进程里可能与 mygo WebView2 的 STA 冲突，host 进程无此问题；GUI 进程验证密钥时若冲突可退 RO_INIT_SINGLETHREADED 或专用线程
- **每线程 RoInitialize 一次**：Verify 若从多个 goroutine 调用，需固定到专用 OS 线程（runtime.LockOSThread）或用 sync.Once 确保只初始化一次

## 5. 对后续实现的影响

- Hello syscall 实现与 PoC 按此清单施工，预估 ~300 行（原估 200 行偏乐观，含 HSTRING/事件/守卫）
- 无需引入第三方依赖，纯标准库 syscall（实测通过，未用到 golang.org/x/sys）
- 调研发现的 hiddenwindow 消息泵问题不存在：Completed 回调在线程池触发，host 主循环无阻塞风险
- **实现已落地**：`internal/hello/`（合并提交 43996dd 前的 11a6ccd），实测 `Available() = true`、`Verify() = verified`（Windows Security 提示由隐藏窗口 owner 拉起），并在启用 `put_Completed` 之前暴露了上面那条 handler IID 的坑
