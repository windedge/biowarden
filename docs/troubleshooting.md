# Troubleshooting, limitations and uninstall

## First: the unlock prompt never appears

Three causes cover almost every report, in this order:

1. **The browser was not restarted after the manifest was written.** Browsers read the
   `NativeMessagingHosts` registry values only at startup. A fresh install or a repair takes effect
   on the next launch, and "restart the browser" means quitting every window, not reloading the
   extension.
2. **The official Bitwarden Desktop app took the registration back.** It rewrites those keys for
   every browser each time it starts. The **Browsers** page shows a red "overwritten by another
   program" state and offers one-click recovery; the durable fix is not to run the official client
   alongside.
3. **Biometrics are switched off in the extension**, or the extension lost the `nativeMessaging`
   permission when it reloaded. Settings -> Security -> "Unlock with biometrics" turns both on.

If the prompt does appear but unlock fails, check the session banner on the Account page: a
rate-limited or unreachable server is reported there and retried on its own, and the key stays
usable for unlocking meanwhile.

## The extraction script: a manual fallback

The manual path exists for the cases the login flow cannot cover: an account protected by WebAuthn
or Duo, a self-hosted server whose endpoints Biowarden cannot reach, or a broken login you want to
diagnose.

On the **Account** page, expand **Advanced**. **Copy extraction script** copies a snippet that runs
in the extension's background console (Chrome: `chrome://extensions` -> the Bitwarden service
worker; Firefox: `about:debugging`) while the extension is unlocked. It prints `{userId, keyB64}`.

Bitwarden 2026.8.x keeps the key in session storage (wrapped as `{__json__}`) or, under a "never
lock" policy, in local `<userId>_user_auto`. The script handles both, and says why it found nothing
otherwise (the extension is locked, or the "never lock" policy is off, or the layout changed).
Paste both values back on the same page, save, and approve the Windows Hello prompt.

## Known limitations

- **The official Bitwarden desktop client conflicts with Biowarden.** It unconditionally rewrites
  the `NativeMessagingHosts` registry keys for every browser each time it starts, taking the
  registration back. The **Browsers** page shows a red "overwritten by another program" state and
  offers one-click recovery, but the real fix is not to run the official desktop app alongside.
- **A manifest change needs a full browser restart.** Browsers read the registry values only at
  startup, so a fresh install or a repair takes effect on the next browser launch.
- **Chromium reads its own registry key.** A Chrome install on the Browsers page writes both the
  `Google\Chrome` and the `Chromium` key; Biowarden versions before 2026.1 only wrote the former.
- **Only the zero-configuration first launch auto-registers.** Adding a browser you install later
  is a manual step on the Browsers page; later launches never register anything on their own.
- **WebAuthn and Duo second factors are not supported.** Biowarden has no embedded web vault and
  does not register the `bitwarden://` protocol, so neither connector's callback can reach it.
  Accounts that require them need the extraction script fallback above.
- **The extraction script depends on the extension's internal storage layout**
  (`chrome.storage.session`'s `user_<id>_crypto_userKey`). A major Bitwarden extension update can
  change it; the script reports the reason instead of failing silently.
- **Browser coverage.** There are registration points for Chrome, Chromium, Edge and Firefox. Edge
  and Firefox still await real-browser acceptance testing. Brave, Vivaldi, Safari and other
  browsers are not supported.
- **Two host processes per browser is normal.** Host processes stay alive per connection, and the
  extension polling once a second while locked is expected.
- **An unlock request blocks that connection while the Hello prompt is up**, exactly as the
  official desktop client does.

## Uninstall

- **Browsers** page: click **Uninstall** for every configured browser (removes the manifest and the
  registry value)
- **Account** page: delete each stored account key (Hello consent required), or use **Log out**
- Quit Biowarden (tray menu -> Exit) and delete the program and its installer
