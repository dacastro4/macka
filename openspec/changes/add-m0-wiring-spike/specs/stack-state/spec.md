## ADDED Requirements

### Requirement: State file is the source of truth (T-2)
The manager SHALL treat `stack.yml` as the only source of truth for the stack. The Compose file and `.env` file SHALL be generated from it and SHALL carry a header stating they are generated and must not be edited.

#### Scenario: Generate from state
- **WHEN** the user runs `macka apply` with a valid `stack.yml`
- **THEN** the manager writes `docker-compose.yml` and `.env` derived only from `stack.yml` and pinned image references

#### Scenario: Deterministic generation
- **WHEN** generation runs twice on the same `stack.yml`
- **THEN** the generated files are byte-identical

#### Scenario: Invalid state rejected
- **WHEN** `stack.yml` fails schema validation
- **THEN** the manager exits non-zero, names the invalid field, and changes no files or containers

### Requirement: Pinned images
Every service in the generated Compose file SHALL reference its image by version tag and digest taken from `docs/COMPATIBILITY.md`.

#### Scenario: Unpinned image refused
- **WHEN** a component has no recorded tag and digest
- **THEN** generation fails with a message pointing to `docs/COMPATIBILITY.md`

### Requirement: Shared data root (S-1 groundwork)
All media-facing services SHALL mount one shared data root at the same container path.

#### Scenario: Same path in every container
- **WHEN** the Compose file is generated
- **THEN** Jellyfin, Radarr and Sonarr mount the data root at the identical container path

### Requirement: Pre-seeded API keys (T-5)
The manager SHALL generate a unique API key per app before first start, store it with owner-only file permissions, and write it into the app's configuration so the app starts with that key.

#### Scenario: Key known before first boot
- **WHEN** Radarr, Sonarr or Prowlarr starts for the first time
- **THEN** its API accepts the key the manager generated

#### Scenario: Secrets not logged
- **WHEN** any `macka` command runs
- **THEN** no API key or password appears in stdout, stderr or log files

### Requirement: Docker project isolation (D-11 groundwork)
The generated Compose project SHALL use the project name from `stack.yml`, and the manager SHALL refuse to apply when a different stack directory already owns that project name on the host.

#### Scenario: Name collision refused
- **WHEN** a second stack directory applies with a project name already in use
- **THEN** the manager exits non-zero without creating or recreating any container
