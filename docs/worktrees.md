# Worktrees for Claude Development

task-plus manages git worktrees so you can run multiple Claude agents in parallel, each in its own isolated branch.

## Quick Start

```bash
# One-off: generate Taskfile snippets
task-plus wt --init

# Create a worktree and open VS Code in it
task-plus wt start --task=add-login

# In the VS Code terminal, run claude interactively (with safety checks)
task-plus claude

# Or run a headless agent for the dashboard
task-plus wt agent --task=add-login --spec="implement the login page"

# Monitor all running agents
task-plus wt dashboard          # web UI (default)
task-plus wt dashboard --term   # terminal table
```

## Typical Workflow

1. **`task-plus wt start --task=add-login`** — creates a worktree and opens VS Code in it
2. **In the VS Code terminal: `task-plus claude`** — verifies worktree sandbox is active (restricts access to ~/.ssh, ~/.aws, etc.), then runs `claude --dangerously-skip-permissions`
3. Work interactively with Claude in the sandboxed worktree
4. **`task-plus wt review --task=add-login`** — review the diff
5. **`task-plus wt merge --task=add-login`** — merge and clean up

For headless/automated use, `wt agent` registers with the dashboard instead.

## How It Works

```mermaid
flowchart TD
    A[wt start] --> B{Worktree exists?}
    B -->|No| C[git worktree add]
    B -->|Yes| D[Resume]
    C --> E[Write .claude/settings.json]
    D --> F{settings.json exists?}
    F -->|No| E
    F -->|Yes| G[Open VS Code]
    E --> G
```

```mermaid
flowchart TD
    A[wt agent] --> B{Worktree exists?}
    B -->|No| Z[Error: run wt start first]
    B -->|Yes| C[Start status server]
    C --> D[Register in agents.json]
    D --> E{--spec provided?}
    E -->|Yes| F[Run claude]
    E -->|No| G[Wait for Ctrl+C]
    F --> H[Deregister + shutdown]
    G --> H
```

## Commands

| Command | Description |
|---------|-------------|
| `wt start --task=NAME` | Create or resume a worktree, open VS Code |
| `wt agent --task=NAME [--spec="PROMPT"]` | Run Claude agent in worktree (registers with dashboard) |
| `wt review --task=NAME` | Show diff between main and the task branch |
| `wt merge --task=NAME` | Merge task branch into current branch, remove worktree |
| `wt clean --task=NAME` | Merge branch, close VS Code folder, remove from recent list, remove worktree, delete branch |
| `wt list` | List all git worktrees |
| `wt dashboard [--term]` | Live dashboard of running agents |
| `wt --init` | Print Taskfile.yml snippets for all wt commands |
| `claude` | Run `claude --dangerously-skip-permissions` (requires worktree + sandbox) |

## The `claude` Command

`task-plus claude` is a safety wrapper that:

1. Checks you're in a git worktree (not the main checkout)
2. Checks `.claude/settings.json` exists with sandbox enabled
3. Runs `claude --dangerously-skip-permissions`

Any extra arguments are passed through to claude.

## Agent Lifecycle

Each `wt agent` registers itself in `~/.config/task-plus/agents.json` with:
- HTTP port (random, serves `/status`)
- PID, worktree path, branch, project name, start time

On exit (normal or Ctrl+C), the agent:
1. Shuts down the HTTP status server
2. Deregisters from agents.json

Stale entries (dead PIDs) are cleaned automatically on every `wt agent`.

## Resume Support

If you run `wt start --task=add-login` and the worktree directory already exists, it resumes instead of failing with "branch already exists". This handles:
- Machine reboots
- Interrupted sessions
- Re-running a task after reviewing changes

## Dashboard

The dashboard polls each registered agent's `/status` endpoint every 2 seconds.

**Web mode** (default, port 8091): Bulma-styled table with auto-refresh, powered by lofigui.

**Terminal mode** (`--term`): ANSI table that clears and redraws. Ctrl+C exits.

Both modes show: task key, branch, status (Running/Idle/Offline), last commit subject, uptime, and port.

## Directory Layout

Worktrees are placed alongside the main repo:

```
~/projects/
  my-app/              # main repo
  my-app-add-login/    # worktree for task "add-login"
  my-app-fix-bug/      # worktree for task "fix-bug"
```

Each worktree gets a `.claude/settings.json` with sandbox config (denies `~/.ssh` and `~/.aws` reads). Sandbox stub files are excluded via `.git/info/exclude`.

## Taskfile Integration

Run `task-plus wt --init` to get copy-paste Taskfile snippets. The `task wt:*` commands are Taskfile wrappers around `task-plus wt *`.

```bash
task wt:start TASK=my-feature
task wt:agent TASK=my-feature SPEC="implement the login page"
task wt:review TASK=my-feature
task wt:merge TASK=my-feature
task wt:clean TASK=my-feature
task wt:list
task wt:dashboard
```
