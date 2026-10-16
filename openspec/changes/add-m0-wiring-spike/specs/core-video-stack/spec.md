## ADDED Requirements

### Requirement: Core video components
The default M0 stack SHALL install Jellyfin, Radarr, Sonarr and Prowlarr, each bound to the LAN or localhost only.

#### Scenario: Stack comes up
- **WHEN** `macka apply` runs on a fresh Debian 12 VM with Docker Engine
- **THEN** all four containers run and each app's API responds healthy

### Requirement: Indexer manager sync
Prowlarr SHALL be connected to Radarr and Sonarr as applications with full sync, using the pre-seeded API keys.

#### Scenario: Apps registered in Prowlarr
- **WHEN** wiring completes
- **THEN** Prowlarr's API lists Radarr and Sonarr as applications and each connection test passes

### Requirement: Root folders
Radarr SHALL have the movies folder and Sonarr the TV folder under the shared data root as root folders.

#### Scenario: Root folders present
- **WHEN** wiring completes
- **THEN** Radarr's API lists the movies root folder and Sonarr's API lists the TV root folder

### Requirement: Media server libraries
Jellyfin SHALL complete first-run setup with a generated admin password and have a Movies library and a Shows library pointing at the same folders as Radarr and Sonarr.

#### Scenario: Libraries present
- **WHEN** wiring completes
- **THEN** Jellyfin's API lists both libraries with the expected paths, and the generated admin password signs in

#### Scenario: No default credentials
- **WHEN** wiring completes
- **THEN** no app accepts a default or empty admin password

### Requirement: Library refresh on import
Radarr and Sonarr SHALL notify Jellyfin to refresh its library when they import media.

#### Scenario: Notification configured
- **WHEN** wiring completes
- **THEN** Radarr and Sonarr each list a Jellyfin notification whose connection test passes

### Requirement: M0 gate
A fresh Debian VM SHALL reach a fully wired stack, and a second run SHALL change nothing.

#### Scenario: Gate test
- **WHEN** CI provisions a clean Debian 12 VM, runs `macka apply`, then runs `macka apply` again
- **THEN** the first run reports every link in this spec as connected, and the second run reports all steps `already-satisfied` with no container recreated
