## 1. Repository bootstrap

- [x] 1.1 `git init`; Go module; CC-7 layout (`cmd/hli`, `internal/{wiring,manifests,platform,backup,health}`, `web/`, `manifests/`, `images/`, `docs/`, `test/`)
- [x] 1.2 Root `CLAUDE.md` with conventions, build/test commands and the never-guess rule (CC-1)
- [x] 1.3 `docs/COMPATIBILITY.md` table: component, image tag, digest, API endpoints used, verified date, verified how
- [x] 1.4 CI skeleton: lint, unit tests, test-name check that every spec requirement ID has a test (CC-2)

## 2. Verify upstream facts (before writing wiring code)

- [x] 2.1 Pin Jellyfin, Radarr, Sonarr, Prowlarr images by tag and digest; record in COMPATIBILITY.md
- [x] 2.2 Confirm arr apps honour a pre-written `ApiKey` in config before first boot (design Q2)
- [x] 2.3 Confirm Jellyfin first-run setup and API key creation over its API (design Q3); if not possible, add to doc section 14 (CC-6)
- [x] 2.4 Record every endpoint and schema/template route the steps will call

## 3. stack-state

- [x] 3.1 `stack.yml` schema and validator; test `TestT2_InvalidStateRejected`
- [x] 3.2 Compose and `.env` generator with generated-file header; test `TestT2_DeterministicGeneration`
- [x] 3.3 Pinned-image enforcement from COMPATIBILITY data; test `TestT2_UnpinnedImageRefused`
- [x] 3.4 Shared data root mounts; test `TestS1_SameDataPathAllContainers`
- [x] 3.5 API key generation, owner-only storage, config pre-seed; tests `TestT5_KeyKnownBeforeFirstBoot`, `TestT5_SecretsNotLogged`
- [x] 3.6 Project-name collision guard; test `TestD11_ProjectNameCollisionRefused`

## 4. wiring-engine

- [x] 4.1 `Step` interface, dependency ordering, outcome recording; test `TestT4_ApplyOnlyWhenMissing`
- [x] 4.2 Verify-after-apply and failure reporting; test `TestP3_ApplyWithoutVerifyFails`
- [x] 4.3 Drift detection fails loud without overwriting; test `TestP4_DriftNotOverwritten`
- [x] 4.4 Readiness waits with timeout and blocked-step reporting
- [x] 4.5 Schema/template-driven payload helper; test `TestT6_PayloadFromSchema`

## 5. core-video-stack steps

- [x] 5.1 Radarr and Sonarr root folders
- [x] 5.2 Prowlarr → Radarr, Prowlarr → Sonarr application sync
- [x] 5.3 Jellyfin first-run, generated admin password, Movies and Shows libraries
- [x] 5.4 Radarr and Sonarr → Jellyfin library refresh notification
- [x] 5.5 Final per-link report output

## 6. CLI

- [x] 6.1 `hli apply`: validate → generate → `docker compose up -d` → wire → report
- [x] 6.2 `hli status`: run every `check` read-only and print link states

## 7. Gate (M0)

- [x] 7.1 Contract tests against real pinned containers (CC-3)
- [ ] 7.2 CI job: clean Debian 12 VM, `hli apply` twice; assert all links connected, then all `already-satisfied`, no container recreated (`TestP4_SecondRunNoop`)
- [x] 7.3 Record gate result and any new open questions in the requirements doc
