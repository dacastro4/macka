## Why

HLI's whole value rests on one bet: a manager can bring up a Docker stack and wire every app through its own API, verifiably and repeatably, with no user copying API keys. Milestone M0 proves that bet on the smallest useful stack before any wizard, UI, platform or safety work is built on top of it (requirements doc, section 12; CC-5).

## What Changes

- New Go repository skeleton following CC-7 (`cmd/hli`, `internal/{wiring,manifests,platform,backup,health}`, `web/`, `manifests/`, `images/`, `docs/`, `test/`), with a root `CLAUDE.md` (CC-1) and `docs/COMPATIBILITY.md`.
- A `stack.yml` state file as the single source of truth, from which the manager generates a plain Docker Compose project and `.env` file (T-2). Generated files are never hand-edited.
- API keys for the arr apps are pre-seeded into their config before first start, so wiring is deterministic (T-5).
- A wiring engine: an ordered list of idempotent steps, each with `check` (read the target API), `apply`, and `verify` (read back again) (T-4, P-3, P-4). Payloads are built from each app's own API schema where offered; no guessed endpoints (T-6).
- The core video stack, wired end to end on one Debian VM: Jellyfin, Radarr, Sonarr, Prowlarr.
- A minimal `hli` CLI (`apply`, `status`) to drive the spike. No web UI, no wizard, no installer packaging in M0.
- A gate test: a fresh Debian VM reaches a wired stack, and a second run changes nothing.

Satisfies: P-3, P-4, T-2, T-4, T-5, T-6 (milestone M0).

Out of scope (later milestones): download clients and VPN, safety rules X-*, wizard W-*, UI C-*, books S-3..S-5, remote access R-*, other platforms D-7..D-10, add-on catalog A-*, backups and updates L-*.

## Capabilities

### New Capabilities

- `stack-state`: `stack.yml` schema, Compose and `.env` generation, Docker project naming, pre-seeded secrets.
- `wiring-engine`: idempotent check/apply/verify steps, verified-link reporting, re-run safety.
- `core-video-stack`: the Jellyfin, Radarr, Sonarr and Prowlarr components and the links between them.

### Modified Capabilities

None. This is the first change in the repository.

## Impact

- New code: entire repository (empty today).
- Runtime dependencies: Docker Engine with the Compose plugin on Debian 12+.
- Upstream dependencies: Jellyfin, Radarr, Sonarr, Prowlarr container images, pinned by tag and digest once verified (record in `docs/COMPATIBILITY.md`).
- Open questions that block M0 (doc section 14): #1 project name (placeholder `hli` used until decided), #4 manager stack (this proposal adopts the recommendation: Go).
