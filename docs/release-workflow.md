# Release Workflow

task-plus automates the release process for Go projects. The workflow runs through five phases: Precheck, Gather, Ask, Check, and Execute.

## Flowchart

```mermaid
flowchart TD
    A[Start] --> B[Precheck]
    B --> C[Gather State]
    C --> D{Git Dirty?}
    D -->|Yes| E[Add + Commit]
    D -->|No| F[Ask Version]
    E --> F
    F --> G[Run Checks]
    G --> H[Update CHANGELOG]
    H --> I[Git Tag]
    I --> J{WASM?}
    J -->|Yes| K[Build WASM + Re-tag]
    J -->|No| L[Git Push]
    K --> L
    L --> M{Binary?}
    M -->|Yes| N[Goreleaser]
    M -->|No| O[Cleanup]
    N --> O
    O --> P{Fork?}
    P -->|Yes| Q[Skip Install]
    P -->|No| R[Install]
    Q --> S[Done]
    R --> S
```

## Phases

### 1. Precheck
Runs any configured precheck commands (e.g. `task precheck`). Fails fast before user interaction.

### 2. Gather
Read-only state probing:
- Git status (dirty/clean)
- Existing tags and retracted versions
- Suggested next version (auto-incremented, skipping retracted)
- Fork detection (go.mod vs git remote)
- Goreleaser config detection
- Forge detection (GitHub/Gitea) and release cleanup candidates

### 3. Ask
Interactive prompts:
- Git add/commit if dirty
- Version confirmation (suggested or custom)
- Release comment
- Push, goreleaser, cleanup, install confirmations

### 4. Check
Runs configured check commands (default: `task check` which runs fmt, vet, test).

### 5. Execute
All mutations in order:
1. Git add + commit (if dirty)
2. Tag existence check
3. Version update task (if configured)
4. CHANGELOG update + auto-commit
5. Git tag
6. WASM build + re-tag (if configured)
7. Git push
8. Goreleaser (binary projects)
9. Release cleanup (old releases)
10. Local install (`go install`)
