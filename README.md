# macka

Free, open-source installer and control panel that gives a non-technical person
a complete home digital library (movies, TV, music, ebooks, audiobooks) on their
own PC or in a VM, without opening a terminal.

**Status: Milestone 0 (spike).** The `macka` manager generates a pinned Docker
Compose stack from `stack.yml` and wires Jellyfin, Radarr, Sonarr and Prowlarr
through their own APIs. Running it twice changes nothing. Not for end users yet.

## Try the M0 spike (Linux with Docker)

```bash
make build
cp examples/stack.yml /path/to/stack/
./bin/macka apply -f /path/to/stack/stack.yml
./bin/macka status -f /path/to/stack/stack.yml
```

## Develop

See [CLAUDE.md](CLAUDE.md) for conventions and commands, and
[docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for every verified upstream fact.
Planned work lives in [openspec/](openspec/).

## Legal

macka ships no media, indexers or trackers. See [DISCLAIMER.md](DISCLAIMER.md).
Licensed under the [Apache License 2.0](LICENSE).
