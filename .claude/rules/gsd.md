# GSD Workflow Rules

## Core Flow

```
/gsd:new-project → /gsd:plan-phase → /gsd:execute-phase → /gsd:verify-work → repeat
```

`/gsd:new-project` is run **once**. All subsequent work uses the commands below.

## Command Selection

| Situation | Command |
|---|---|
| New direction or v2-level scope | `/gsd:new-milestone` |
| Feature within current milestone | `/gsd:add-phase` |
| Urgent work between existing phases | `/gsd:insert-phase N` |
| One-off task, no phase needed | `/gsd:quick` |
| Trivial change (≤3 files) | `/gsd:fast` |

**Rule of thumb:** In scope when you ran `/gsd:new-project`? → `add-phase`. New direction or significant expansion? → `new-milestone`.

### Adding a feature (in-scope)

```
/gsd:add-phase "기능명"
/gsd:plan-phase N
/gsd:execute-phase N
/gsd:verify-work N
```

### Starting a new milestone (out-of-scope)

```
/gsd:complete-milestone 1.0.0
/gsd:new-milestone "v2.0 기능"
/gsd:plan-phase 1
/gsd:execute-phase 1
```

---

## Parallel Work with Git Worktrees

Global state files (`STATE.md`, `ROADMAP.md`, `REQUIREMENTS.md`) are updated on every phase execution. `.gitattributes` is already configured so these files always keep main's version on merge — each worktree's `phases/` directories are fully merged in. No upfront coordination needed.

```bash
# Create worktrees and start immediately
git worktree add ../loom-auth feat/auth
git worktree add ../loom-payment feat/payment

# Run GSD freely and independently in each worktree
# PR → merge to main: global files auto-resolved, phases merged in
# After merge: briefly update STATE.md on main to reflect completed work
```

Phase directory names are feature-based (e.g. `01-auth-system/`, `01-payment/`), so two worktrees creating Phase 1 for different features will never produce the same path — no conflicts.

---

## Best Practices

- **`/clear` before and after each phase** — prevents context pollution.
- **`/gsd:discuss-phase` before planning** — conveys your vision; significantly improves plan quality.
- **`/gsd:list-phase-assumptions` before executing** — verify Claude's intended approach before it starts.
- **`--research` for complex domains** — 3D, audio, ML, etc. benefit from the research agent.
- **`.planning/` files should be committed to git** — planning artifacts are part of project history.
