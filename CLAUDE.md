# task-plus

Go CLI tool (`tp` / `task-plus`) that standardises dev workflows across repos. Subcommand architecture with interactive release workflow as the centrepiece.

## Architecture

- `cmd/task-plus/` and `cmd/tp/` — identical entry points (tp is a shorter alias)
- `internal/cli/` — command routing and flag parsing
- `internal/workflow/` — release workflow orchestration (gather → ask → execute)
- `internal/config/` — `task-plus.yml` loader
- `internal/changelog/` — keepachangelog parser/updater
- `internal/version/` — semver parsing and comparison
- `internal/deploy/` — pluggable doc deployment (GitHub Pages, statichost.eu)
- `internal/md2html/` — markdown → Bulma-styled HTML converter
- `internal/worktree/` — git worktree management for isolated Claude tasks
- `internal/forge/` — forge detection and API (GitHub, Codeberg/Forgejo)
- `internal/check/` — project config validation
- `internal/pages/` — docs server and deployment orchestration

## Key commands

`release`, `check`, `pages`, `wt`, `claude`, `md2html`, `md_update`, `readme`, `repos`, `self`

## Build & test

```
task check    # fmt + vet + test
task build    # builds to bin/task-plus
```

## Config

Project config lives in `task-plus.yml` at the project root. The tool is also its own user — this repo has a `task-plus.yml` configuring its own release and docs deployment.

## Conventions

- Version set by goreleaser ldflags; falls back to `debug.ReadBuildInfo()` for `go install`
- Release workflow guards against Taskfile.yml containing a conflicting `release:` task
- Dual-remote setup: origin (Codeberg) + github (GitHub mirror)
- Docs built with `tp md2html`, deployed to statichost.eu
