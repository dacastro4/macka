# Compatibility record

Every image, endpoint, config key and payload shape macka depends on, and how it
was verified (CC-1). Anything not in this file is unverified: mark it
`TODO(verify)` in code. `internal/compat` mirrors the pinned images below; keep
them in sync.

Verified 2026-10-15 on Debian 12 (arm64, Lima VM), Docker Engine 29.9.1,
Compose v5.6.0, by starting each pinned image and calling it with curl.

## Pinned images

Digests are multi-arch index digests (linux/amd64 + linux/arm64 confirmed with
`docker buildx imagetools inspect`).

| Component | Image | Tag | Digest | App version |
| --- | --- | --- | --- | --- |
| Jellyfin | `jellyfin/jellyfin` | `12.2.20261005-225228` | `sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037` | 12.2.0 |
| Radarr | `lscr.io/linuxserver/radarr` | `6.4.4.10685-ls321` | `sha256:ea6822c956d26db648b237919f41340318f32e5b7dcc4ba702cf256a0f5f3c12` | 6.4.4.10685 |
| Sonarr | `lscr.io/linuxserver/sonarr` | `4.0.20.3014-ls329` | `sha256:44b06045a98bc1f237b9ffefc4b28dab64cb29305e4a2a94875ae93b5224e4cc` | 4.0.20.3014 |
| Prowlarr | `lscr.io/linuxserver/prowlarr` | `2.6.5.5623-ls163` | `sha256:f9151e5bc1025c6d0a630d503210cdcb6bb55a7cc098562609d96a408d838902` | 2.6.5.5623 |

## Container runtime

| Fact | Detail | Verified how |
| --- | --- | --- |
| Ports | Jellyfin 8096, Radarr 7878, Sonarr 8989, Prowlarr 9696 (HTTP, in-container) | Containers answered on these ports |
| Config path | All four use `/config` | Containers started with `/config` bind mounts |
| User (linuxserver images) | `PUID`, `PGID` env vars | Started with VM user's uid/gid; files owned correctly |
| User (Jellyfin official) | Compose `user: "<uid>:<gid>"` | With a uid that does not own `/config`, Jellyfin crashes: `Access to the path '/config/log' is denied` |
| UID must match data owner | The VM user was uid 502, not 1000; macka must detect the owning uid/gid, never assume 1000 | Observed failure above |
| Cache path | Jellyfin `/cache` | Mounted, started cleanly |

## Radarr, Sonarr, Prowlarr (Servarr)

API base: Radarr `/api/v3`, Sonarr `/api/v3`, Prowlarr `/api/v1`. Auth header
`X-Api-Key`. No OpenAPI document is served at runtime (`/api/v3/openapi.json`
→ 404), so payloads come from each app's `/schema` endpoints (T-6).

| Fact | Detail | Verified how |
| --- | --- | --- |
| Pre-seeded key (T-5) | Writing `<Config><ApiKey>KEY</ApiKey></Config>` to `/config/config.xml` before first start makes the app use KEY; the app fills in other defaults | All three: `GET /system/status` 200 with KEY, 401 without |
| Liveness | `GET /ping` → 200, no auth | curl |
| Status | `GET {base}/system/status` → `appName`, `version` | curl |
| Root folders | `GET {base}/rootfolder`; `POST {base}/rootfolder` `{"path":"/data/media/movies"}` → 201 with `id`, `accessible:true` | Radarr |
| App schema | `GET /api/v1/applications/schema` → templates; `Radarr`/`Sonarr` entries have fields `prowlarrUrl`, `baseUrl`, `apiKey`, `syncCategories`, default `syncLevel: fullSync` | Prowlarr |
| Add app | `POST /api/v1/applications` with the schema template, `name` set, fields filled → 201. Runs a connection test first: wrong key → 400 `API Key is invalid` | Prowlarr → Radarr |
| Notification schema | `GET {base}/notification/schema`; Jellyfin entry `implementation: MediaBrowser`, `implementationName: Emby / Jellyfin`, fields `host`, `port` (8096), `useSsl`, `urlBase`, `apiKey`, `notify`, `updateLibrary` (true), `mapFrom`, `mapTo` | Radarr, Sonarr |
| Add notification | `POST {base}/notification` with template, `name`, `onDownload`/`onUpgrade`/`onRename`/`on*Delete` true, `host`, `apiKey` → 201 (connection tested) | Radarr → Jellyfin |
| Masked secrets | Reads return `apiKey` as `********`. Drift checks cannot compare keys | Prowlarr, Radarr reads |
| Verify an existing link | `GET {resource}/{id}` then `POST {resource}/test` with that body (masked key accepted, server uses stored key) → 200 | Prowlarr app, Radarr notification |
| Unique names | `POST …/test` or create with an existing `name` and no `id` → 400 `Should be unique` | Prowlarr, Radarr |
| Test all apps | `POST /api/v1/applications/testall` → `[{id,isValid,validationFailures}]` | Prowlarr |
| Service URLs | Inside the Compose network, apps reach each other by service name (`http://radarr:7878`, `http://prowlarr:9696`, host `jellyfin`) | Prowlarr→Radarr and Radarr→Jellyfin tests passed |

Sonarr: root folder `/data/media/tv` (201), Prowlarr→Sonarr app (201) and
Sonarr→Jellyfin notification (201) verified with the same shapes as Radarr.
Event flags differ per app: enable `onDownload`, `onUpgrade`, `onRename` and
every supported `on*Delete*` flag (`supportsOn<Event>` true). Radarr:
`onMovieDelete`, `onMovieFileDelete`, `onMovieFileDeleteForUpgrade`. Sonarr:
`onSeriesDelete`, `onEpisodeFileDelete`, `onEpisodeFileDeleteForUpgrade`.

## Jellyfin

OpenAPI served at `GET /api-docs/openapi.json` (200). Auth header
`Authorization: MediaBrowser Client="macka", Device="macka", DeviceId="<id>", Version="<v>"`,
plus `, Token="<token>"` once authenticated. An API key works as
`Authorization: MediaBrowser Token="<key>"`.

| Fact | Detail | Verified how |
| --- | --- | --- |
| Public info | `GET /System/Info/Public` → `Version`, `StartupWizardCompleted` (no auth) | curl |
| First-run state | Fresh server: `StartupWizardCompleted:false`; `GET /Startup/User` → `{"Name":"MyJellyfinUser"}` | curl |
| Startup config | `POST /Startup/Configuration` `StartupConfigurationDto{ServerName,UICulture,MetadataCountryCode,PreferredMetadataLanguage}` → 204 | curl |
| Admin user | `POST /Startup/User` `StartupUserDto{Name,Password}` → 204 | curl, then login |
| Remote access | `POST /Startup/RemoteAccess` `StartupRemoteAccessDto{EnableRemoteAccess}` → 204; send `false` (R-1) | curl |
| Complete | `POST /Startup/Complete` → 204; public info then shows `StartupWizardCompleted:true` | curl |
| Login | `POST /Users/AuthenticateByName` `{Username,Pw}` → `AccessToken` (32 chars); wrong/empty password → 401 | curl |
| API key | `POST /Auth/Keys?app=macka` (elevated) → 204, no body; `GET /Auth/Keys` → `Items[]` of `AuthenticationInfo{AccessToken,AppName,…}`; find by `AppName` | curl |
| Libraries | `POST /Library/VirtualFolders?name=Movies&collectionType=movies&paths=/data/media/movies&refreshLibrary=false` body `{"LibraryOptions":{}}` → 204; `GET /Library/VirtualFolders` → `[{Name,CollectionType,Locations}]` | curl, with API key |
| Collection types | `movies`, `tvshows`, `music`, `musicvideos`, `homevideos`, `boxsets`, `books`, `mixed` | OpenAPI `CollectionTypeOptions` |
