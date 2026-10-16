## Context

The repository is empty. The requirements doc (v0.1, 2026-10-15) fixes the architecture: one Go manager on the host generates a Compose stack from `stack.yml`, controls it, and wires each app through its own API (section 11). M0 builds only the parts needed to prove the wiring approach on one Debian VM.

## Goals / Non-Goals

**Goals:**
- Prove `stack.yml` → Compose → running containers → verified wiring, end to end.
- Prove idempotency: a second `hli apply` performs zero `apply` actions and leaves every container untouched.
- Establish the repo conventions every later milestone inherits (layout, CLAUDE.md, COMPATIBILITY.md, ID-named tests).

**Non-Goals:**
- Web UI, wizard, REST API, host service registration (T-3), installers, VM images.
- Download clients, VPN, safety rules. M0 wires no download path.
- Manifest schema freeze (M4). M0 uses internal Go definitions shaped like the section 8 sketch.

## Decisions

**D1. Go, single static binary (T-1, open question #4).** Adopt the doc's recommendation. Alternative TypeScript rejected: needs a runtime on three OSes.

**D2. Shell out to `docker compose` rather than the Docker SDK for lifecycle.** Generated Compose is the product's portable artifact (P-6, Eject), so the spike should exercise exactly what users will get. The SDK is used only for read-only inspection if needed.

**D3. Step model.** Each wiring step implements:
```go
type Step interface {
    ID() string                          // e.g. "prowlarr->radarr:app-sync"
    Check(ctx) (State, error)            // read target API; Satisfied | Missing | Drifted
    Apply(ctx) error                     // only called when Check != Satisfied
    Verify(ctx) error                    // re-read target API; fail if link absent
}
```
The engine runs steps in dependency order, records per-step outcome (`already-satisfied`, `applied`, `failed`), and reports "link exists" only after `Verify` passes (P-3). `Drifted` in M0 fails loud; drift handling with keep/overwrite is C-3 (M1).

**D4. Pre-seeded keys (T-5).** The manager generates each arr app's API key, stores it in the `.env`/secrets area with owner-only permissions, and writes it into the app's config before first start. Arr apps reading `ApiKey` from `config.xml` is TODO(verify) per image version. Jellyfin has no known pre-seed path: the spike drives its first-run setup over its API, then creates an API key. TODO(verify) both against running containers and record results in `docs/COMPATIBILITY.md`.

**D5. Payloads from upstream schema (T-6).** Prowlarr app-sync and the arr "Connect" notifications are built from the apps' own schema endpoints (fetch the template, fill fields) rather than hand-written JSON. Exact endpoint paths: TODO(verify).

**D6. Data layout follows S-1 now.** Even though hardlinks are an M1 requirement, M0 mounts one shared data root at the same container path in every app (e.g. `/data/media/{movies,tv}`), so M1 does not have to move paths.

**D7. Docker project name.** Derived from `stack.yml` (default `hli`) and written into the Compose file. A second stack with the same name on the same host is refused (D-11 groundwork).

**D8. Links wired in M0.**
| From | To | Link |
| --- | --- | --- |
| Prowlarr | Radarr | Application sync |
| Prowlarr | Sonarr | Application sync |
| Radarr | `/data/media/movies` | Root folder |
| Sonarr | `/data/media/tv` | Root folder |
| Jellyfin | `/data/media/movies`, `/data/media/tv` | Movies and Shows libraries |
| Radarr, Sonarr | Jellyfin | Library refresh notification |
| Jellyfin | admin user | Startup completed, generated password |

## Risks / Trade-offs

- Upstream API shape differs from assumptions → every endpoint goes through COMPATIBILITY.md and contract tests against real pinned containers (CC-3).
- Jellyfin first-run over API may be brittle → isolate in its own step with a clear failure message; if unworkable, record it as an open question (CC-6) rather than guessing.
- Shelling out to `docker compose` couples to CLI output → parse only `--format json` output.
- Spike code becomes production code → acceptable; keep the step interface and generator behind packages that M1 extends, not rewrites.

## Migration Plan

None. New repository.

## Open Questions

- Q1 (doc #1): final project name. Using `hli` as binary and default Compose project name until decided.
- Q2 (resolved 2026-10-15): yes. Radarr, Sonarr and Prowlarr use a key written to `config.xml` before first start (`docs/COMPATIBILITY.md`).
- Q3 (resolved 2026-10-15): yes. The startup wizard completes over its API, then a login token creates the HLI key via `POST /Auth/Keys`.

## Findings during implementation

- Containers must run as the data owner's uid/gid; the Lima VM user is 502, not 1000. `stack.ResolveUser` defaults to the stack directory owner.
- Servarr reads mask API keys, so `Check` compares every field except the key, and `Verify` reads the record back and calls the app's `/test` endpoint, which covers the key.
- `Verify` also runs for already-satisfied links, so "connected" is always read back, never assumed (P-3).
