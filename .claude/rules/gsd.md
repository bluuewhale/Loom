# GSD Workflow Rules

## Core Flow

```
/gsd:new-project → /gsd:plan-phase → /gsd:execute-phase → /gsd:verify-work → repeat
```

## When to Use Which Command

- **`/gsd:new-project`** — one-time only; initializes the project with vision, requirements, and roadmap.
- **`/gsd:new-milestone`** — use when adding a large batch of features (new milestone-level goal).
- **`/gsd:add-phase`** — use when adding a single feature within the current milestone.
- **`/gsd:insert-phase N`** — use when urgent work must be inserted between existing phases.
- **`/gsd:quick`** — one-off tasks that need a plan but don't belong to a phase.
- **`/gsd:fast`** — trivial changes (≤3 files); no planning overhead.

## Best Practices

- **`/clear` before and after each phase** — prevents context pollution; start each step cleanly.
- **Run `/gsd:discuss-phase` before planning** — conveys your vision to the planner; significantly improves plan quality.
- **Check with `/gsd:list-phase-assumptions` before executing** — review Claude's intended approach and correct direction early.
- **Use `--research` for complex domains** — specialized areas (3D, audio, ML, etc.) benefit from the research agent.
- **`.planning/config.json` → `commit_docs: false`** — keep planning files local and out of git.
