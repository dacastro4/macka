# macka

Installer and control panel that gives a non-technical person a complete, wired
home media library. Requirements: "Home Library Installer: Requirements v0.1"
(https://claude.ai/code/artifact/92ed6e4b-7ec4-4094-9841-0850103d0180).
Planned work lives in `openspec/changes/`; accepted specs in `openspec/specs/`.

## The never-guess rule (CC-1)

Never guess an endpoint, image tag, port, config key or payload shape.
Verify it against upstream docs or a running container, record it in
`docs/COMPATIBILITY.md`, and mark anything unverified `TODO(verify)`.

## Conventions

- Every requirement ID (P-, S-, D-, W-, X-, C-, A-, R-, L-, T-, CC-, LC-) has at
  least one test whose name contains it, written without the dash:
  `TestT4_ApplyOnlyWhenMissing` covers T-4. PRs list the IDs they satisfy (CC-2).
- `stack.yml` is the source of truth. Generated files (`docker-compose.yml`,
  `.env`) are never hand-edited (T-2).
- Wiring steps implement `Check`, `Apply`, `Verify` and must be idempotent
  (T-4, P-3, P-4). A link counts as connected only after `Verify` reads it back.
- No app-specific code outside `internal/apps/` wiring steps (T-7).
- Secrets never appear in logs, errors or test output.
- Work in milestone slices; do not build ahead of the current gate (CC-5).
  Ambiguity → add a question to the requirements doc, section 14 (CC-6).
- Go: standard library first; `go vet` and `gofmt` clean.

## Layout

```
cmd/macka/          CLI entry point
internal/stack/     stack.yml schema, Compose/.env generation, secrets
internal/compat/    pinned images and verified upstream facts (mirrors docs/COMPATIBILITY.md)
internal/wiring/    step engine (check/apply/verify, ordering, report)
internal/apps/      per-app API clients and wiring steps
test/gate/          milestone gate scripts (run in a Debian VM)
docs/               COMPATIBILITY.md and other docs
```

## Commands

```bash
make build        # bin/macka for this machine
make build-linux  # bin/macka-linux-<arch> for the Debian VM
make test         # unit tests
make lint         # gofmt + go vet
make reqcheck     # every spec requirement ID has a test (CC-2)
make gate         # M0 gate in a fresh Lima Debian 12 VM (needs limactl)
```

## Dev rig

- Lima VM `hli-deb12` (Debian 12, Docker Engine) is the integration target.
  `limactl shell hli-deb12` opens it; the home directory is mounted read-only.
- `make gate` creates a throwaway VM so every gate run starts clean.
