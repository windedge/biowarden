# Security Policy

biowarden handles your Bitwarden account key. If you found a way to read that
key without a Windows Hello consent, or to reach it from another process, that
is a security problem and we want to hear about it.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting (Security tab -> Report a
vulnerability) so details stay out of public view until there is a fix.

Please include:

- what an attacker would gain,
- the steps or code to reproduce,
- the biowarden version (`biowarden-setup-<version>-amd64.exe` or the commit).

Please do not open a public issue for anything that exposes key material.

## What is in scope

- reading `Biowarden/<userId>` or `Biowarden/Session/<userId>` from the
  Credential Manager without a Hello consent,
- reaching the native messaging host from an origin that is not an allowed
  extension,
- the login flow leaking the master password, access token or refresh token
  beyond what the design states,
- the host accepting frames it should reject (bad MAC, wrong appId or
  messageId echo, replayed frames).

## What is not a vulnerability

- A local attacker who can already run code as your user can wait for the
  unlock prompt and steal the key after you consent. Biometrics gate *access*
  to the key; they do not encrypt it. The key sits in the OS credential store,
  exactly as it does with the official desktop client.
- The login flow sends your credentials to the Bitwarden server you configured.
  A malicious server sees what the official client would show it.
- SmartScreen warnings on the unsigned download.

## Supported versions

Only the latest release is supported. The installer and `SHA256SUMS.txt` live
on the Releases page; verify your download against the sums file.
