# Third-Party Notices

biowarden is released under the MIT license (see [LICENSE](./LICENSE)). It references, ports and
depends on the third-party work listed below. This file records the origin of that work so the
required copyright notices can be preserved.

License types and copyright lines were read from the license text shipped with each Go module in
the module cache (`$(go env GOMODCACHE)/<module>@<version>/LICENSE`), not transcribed from memory.

## Ported and referenced implementations

### goldwarden

- Version: `v0.3.6-2-g7727d3a` (commit `7727d3ac291d6f38473d8861a187c56efb449acb`)
- License: MIT
- Upstream: https://github.com/quexten/goldwarden
- Copyright: Copyright (c) 2023 Bernd Schoolmann
- Usage: the login, KDF and encryption implementation is referenced and partly ported from
  goldwarden. Ported code keeps the MIT notice above.

### WardenBio

- Version: not pinned (macOS/Swift reference implementation)
- License: 未能核实 - the checked-out tree carries no LICENSE file
- Upstream: https://github.com/xVanTuring/WardenBio
- Copyright: 未能核实
- Usage: biowarden is a Windows port of WardenBio's behavior; no source code is copied verbatim.

### bw-bio-handler

- Version: not pinned (legacy command protocol reference)
- License: 未能核实 - no local checkout was available to read the license text
- Upstream: https://github.com/quexten/bw-bio-handler
- Copyright: 未能核实
- Usage: reference only for the legacy command protocol; no source code is copied verbatim.

## Direct dependencies (from go.mod)

### github.com/danieljoos/wincred

- Version: v1.2.3
- License: MIT
- Upstream: https://github.com/danieljoos/wincred
- Copyright: Copyright (c) 2014 Daniel Joos
- Evidence: `$(go env GOMODCACHE)/github.com/danieljoos/wincred@v1.2.3/LICENSE`

### github.com/egoist/mygo

- Version: v0.4.8
- License: MIT
- Upstream: https://github.com/egoist/mygo
- Copyright: Copyright (c) 2026 MyGo contributors
- Evidence: `$(go env GOMODCACHE)/github.com/egoist/mygo@v0.4.8/LICENSE`

### golang.org/x/sys

- Version: v0.48.0
- License: BSD-3-Clause
- Upstream: https://cs.opensource.google/go/x/sys
- Copyright: Copyright 2009 The Go Authors.
- Evidence: `$(go env GOMODCACHE)/golang.org/x/sys@v0.48.0/LICENSE`

## Indirect dependencies (from go.mod)

### github.com/ebitengine/purego

- Version: v0.11.1
- License: Apache-2.0
- Upstream: https://github.com/ebitengine/purego
- Copyright: not stated in the LICENSE file (it carries the unmodified Apache-2.0 text, whose
  appendix still shows the `Copyright {yyyy} {name of copyright owner}` placeholder)
- Evidence: `$(go env GOMODCACHE)/github.com/ebitengine/purego@v0.11.1/LICENSE`

### github.com/go-text/typesetting

- Version: v0.3.5
- License: Unlicense OR BSD-3-Clause (the LICENSE file offers both)
- Upstream: https://github.com/go-text/typesetting
- Copyright: Copyright 2021 The go-text authors
- Evidence: `$(go env GOMODCACHE)/github.com/go-text/typesetting@v0.3.5/LICENSE`

### golang.org/x/image

- Version: v0.46.0
- License: BSD-3-Clause
- Upstream: https://cs.opensource.google/go/x/image
- Copyright: Copyright 2009 The Go Authors.
- Evidence: `$(go env GOMODCACHE)/golang.org/x/image@v0.46.0/LICENSE`

## Notes

- `golang.org/x/crypto` is not a dependency of biowarden and is therefore not listed here.
- The full license text for each dependency is reproduced verbatim in that dependency's own module
  directory in the Go module cache, and is shipped with any binary that statically links it.
