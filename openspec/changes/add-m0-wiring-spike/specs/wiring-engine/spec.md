## ADDED Requirements

### Requirement: Check, apply, verify steps (T-4)
Each wiring step SHALL implement `check` (read the target app's API), `apply`, and `verify` (read the target API again). `apply` SHALL run only when `check` reports the link missing.

#### Scenario: Missing link applied and verified
- **WHEN** `check` finds a link absent
- **THEN** the engine runs `apply`, then `verify`, and records the step as `applied` only if `verify` reads the link back

#### Scenario: Present link skipped
- **WHEN** `check` finds the link already present and matching
- **THEN** the engine skips `apply` and records `already-satisfied`

### Requirement: Verified reporting (P-3)
The engine SHALL report a link as existing only after `verify` reads it back from the target API. A sent request alone SHALL NOT count as success.

#### Scenario: Apply succeeds but link absent
- **WHEN** `apply` returns success but `verify` cannot find the link
- **THEN** the step fails with the step ID, target app and what was expected

#### Scenario: Final report
- **WHEN** `hli apply` finishes
- **THEN** it prints one line per link, e.g. `Prowlarr to Radarr: connected`, and exits non-zero if any step failed

### Requirement: Idempotent runs (P-4)
Running the full wiring twice SHALL change nothing on the second run.

#### Scenario: Second run is a no-op
- **WHEN** `hli apply` runs on a stack it already wired
- **THEN** every step reports `already-satisfied`, no `apply` runs, and no container is recreated or restarted

#### Scenario: Drift detected
- **WHEN** `check` finds a link present but with different settings than desired
- **THEN** the step fails with a drift message and does not overwrite the setting (keep/overwrite UI is C-3, M1)

### Requirement: Payloads from upstream schema (T-6)
Steps SHALL build request payloads from the target app's own API schema or templates where the app offers them. Endpoint paths SHALL come from `docs/COMPATIBILITY.md`.

#### Scenario: No hard-coded payload where schema exists
- **WHEN** a step creates a Prowlarr application or an arr notification
- **THEN** it fetches the app's schema/template first and fills only the required fields

### Requirement: Readiness before wiring
The engine SHALL wait for each app's API to respond healthy, with a bounded timeout, before running steps that target it.

#### Scenario: App never becomes ready
- **WHEN** an app's API does not respond within the timeout
- **THEN** dependent steps are skipped, reported as blocked, and the command exits non-zero
