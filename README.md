# task-plus

Go CLI tool that standardizes common development workflows across repositories. Subcommand architecture — start with `release`, more commands to come.

<!-- auto:version -->Latest: v0.1.46<!-- /auto:version -->

Try https://h3-task-plus.statichost.page/ for documentation.

## Install

```bash
go install github.com/drummonds/task-plus/cmd/task-plus@latest
```

A shorter alias `tp` is also available:

```bash
go install github.com/drummonds/task-plus/cmd/tp@latest
```

Both binaries are identical — `tp` is just shorter to type.

## Commands

| Command | Description |
|---------|-------------|
| [`check`](https://h3-task-plus.statichost.page/check.html) | Validate task-plus.yml and Taskfile.yml configuration |
| [`release`](https://h3-task-plus.statichost.page/release-workflow.html) | Interactive release workflow |
| `release:version-update` | Scaffold a Taskfile task to update version strings |
| `repos` | Manage git remotes for release |
| [`pages`](https://h3-task-plus.statichost.page/pages.html) | Serve, deploy, configure, and migrate documentation |
| `md2html` | Convert markdown files to Bulma-styled HTML |
| `readme` | Update auto-marker sections in README.md |
| [`wt`](https://h3-task-plus.statichost.page/worktrees.html) | Manage git worktrees for isolated Claude tasks |
| [`claude`](https://h3-task-plus.statichost.page/worktrees.html#the-claude-command) | Run claude with --dangerously-skip-permissions (requires worktree + sandbox) |
| `self` | Manage task-plus itself |

### Global Flags

- `--init` — create a default `task-plus.yml` config file (statichost.eu pre-configured)
- `-a` — list available commands
- `--version` — print version

### `tp check`

Validates project configuration and prints a report. Checks:

- `task-plus.yml` — parses YAML, validates type/forge/changelog_format/deploy targets, flags unknown fields
- `Taskfile.yml` — checks standard tasks (fmt, vet, test, check), detects name conflicts and inversions
- Cross-repo — validates `-docs` sibling relationship, checks for stale `docs/` or misplaced config
- In a `-docs` repo, warns about `.md` files missing the `DOC-` prefix

```bash
tp check
tp check --dir /path/to/project
```

### `tp release`

Interactive release workflow. Replaces duplicated `task release` Taskfile patterns.

```bash
tp release
tp release --dry-run
tp release --yes --dir /path/to/project
```

Flags:
- `--dry-run` — show what would happen without making changes
- `--yes` — auto-confirm all prompts
- `--dir <path>` — project directory (default: `.`)

**Taskfile guard:** If `Taskfile.yml` contains a `release:` task, tp refuses to run (to avoid conflict).

#### Release Workflow

1. Run checks (e.g. `task check`)
2. Show git status
3. Git add / commit (if dirty)
4. Detect version (latest tag + patch bump)
5. Update CHANGELOG.md
6. Git tag
7. WASM build (if configured)
8. Git push (branch + tags)
9. Goreleaser (if binary project)
10. Cleanup old GitHub releases
11. Local install
12. Deploy documentation (if configured)
13. Run `post:release` Taskfile task (if present)

### `tp release:version-update`

Scaffolds a sample Taskfile task for updating version strings during release. The release workflow calls this task (if it exists) with `VERSION=vX.Y.Z` after the version is confirmed.

```bash
tp release:version-update --init
```

### `tp repos`

Manage which git remotes are pushed to during release.

```bash
tp repos              # show configured and available remotes
tp repos info         # same as above
tp repos add <name>   # add a git remote to the release push list
tp repos remove <name> # remove a remote from the release push list
```

### `tp pages`

Serve, deploy, and manage documentation. When run from a main project repo with a `-docs` sibling, automatically delegates to the docs repo.

```bash
tp pages                    # build and serve docs/ over HTTP
tp pages --port 3000        # custom port
tp pages deploy             # deploy to configured targets
tp pages deploy --dry-run   # show what would happen
tp pages config             # show configured deploy targets
tp pages migrate            # create a -docs sibling repo from docs/
tp pages migrate clean      # remove docs/ and pages config from main repo after migration
```

Flags (serve mode):
- `--port <n>` — HTTP port (default: `8080`)
- `--dir <path>` — project directory (default: `.`)

### `tp md2html`

Converts markdown files to Bulma-styled HTML pages with breadcrumb navigation.

```bash
tp md2html                                    # convert docs/internal/*.md in place
tp md2html --src docs/api --dst docs/api      # custom directories
tp md2html --file README.md --dst docs/       # single file
tp md2html --dst docs --index --project myapp # generate index.html from all .html in --dst
```

Flags:
- `--src <dir>` — source markdown directory (default: `docs/internal`)
- `--dst <dir>` — destination HTML directory (default: `docs/internal`)
- `--label <text>` — breadcrumb label (default: `Internal Docs`)
- `--project <name>` — project name (auto-detected from go.mod)
- `--file <path>` — single file to convert (overrides `--src`)
- `--index` — generate an `index.html` listing all `.html` files in `--dst`
- `--subtitle <text>` — subtitle for the index page (default: `Documentation`)

When `--index` is set, the generated `index.html` includes:
- **Pages table** — all `.html` files found in `--dst` (not just files converted in this run)
- **Links table** — auto-discovered from git remotes (parent repo via `parent_repo` in `task-plus.yml`)
- **Intro content** — rendered from `_index.md` if present in `--dst` or `--src`

Typical docs repo Taskfile pattern — run `--index` last so it picks up all pages:

```yaml
docs:build:
  cmds:
    - task-plus md2html --src docs --dst docs           # convert docs/*.md
    - task-plus md2html --file DOC-README.md --dst docs  # individual files
    - task-plus md2html --file ../myapp/README.md --dst docs
    - task-plus md2html --dst docs --index --project myapp  # index last
```

### `tp wt`

Manage git worktrees for running Claude tasks in isolation. Each worktree gets its own branch (`task/<name>`), sandbox settings, and VS Code configuration.

```bash
tp wt start --task=my-feature      # create worktree, open as VS Code workspace folder
tp wt agent --task=my-feature --spec="implement login"  # register agent + run claude
tp wt review --task=my-feature     # diff task branch against main
tp wt merge --task=my-feature      # merge branch and remove worktree
tp wt clean --task=my-feature      # merge, remove worktree, delete branch, close VS Code folder
tp wt list                         # list active worktrees
tp wt dashboard                    # agent dashboard (web UI; --term for terminal)
tp wt --init                       # print Taskfile snippets for wt: tasks
```

Task names "doc" and "docs" are reserved (they clash with the `-docs` repo convention).

### `tp claude`

Runs `claude --dangerously-skip-permissions` with safety guards. Requires:
1. Running inside a git worktree (not the main repo)
2. `.claude/settings.json` with sandbox enabled

```bash
tp claude
tp claude "implement the search feature"
```

### `tp self`

Manage the task-plus installation.

```bash
tp self update    # update to latest version via go install
```

## Config

Optional `task-plus.yml` in project root (generate with `task-plus --init`):

```yaml
type: library           # or "binary" (auto-detected from .goreleaser.yaml)
check: [task check]     # commands to run first
changelog_format: keepachangelog  # or "simple"
wasm: []                # optional WASM build commands
goreleaser_config: .goreleaser.yaml
install: true           # auto-run "go install" (skip prompt; omit to be asked)
cleanup:
  keep_patches: 2       # per minor version
  keep_minors: 5
pages_build: [task docs:build]  # commands to build docs before serving/deploying
pages_deploy:                   # deploy docs during release (multiple targets supported)
  - type: github                # push docs/ to gh-pages branch
  - type: statichost
    site: myproject             # site name on statichost.eu
```

All fields optional — sensible defaults are auto-detected.

### Documentation Deployment

Configure `pages_deploy` in `task-plus.yml` to deploy documentation as part of the release workflow. Multiple targets can be active simultaneously.

**Supported providers:**

| Type | Description | Requirements |
|------|-------------|--------------|
| `github` | Pushes `docs/` to `gh-pages` branch via `git subtree push` | Git remote configured |
| `statichost` | Uploads `docs/` to [statichost.eu](https://www.statichost.eu/) | `site` field required; uses `shcli` (auto-downloaded if missing) |

If `pages_build` commands are configured, they run before deployment.

Example `task-plus.yml` for deploying to both GitHub Pages and statichost.eu:

```yaml
pages_build: [task docs:build]
pages_deploy:
  - type: github
  - type: statichost
    site: my-docs
```

## Links

<!-- auto:links -->
| | |
|---|---|
| Documentation | https://h3-task-plus.statichost.page/ |
| Source (Codeberg) | https://codeberg.org/hum3/task-plus |
| Mirror (GitHub) | https://github.com/drummonds/task-plus |
| Docs repo | https://codeberg.org/hum3/task-plus-docs |
<!-- /auto:links -->
