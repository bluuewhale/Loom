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

## Adding Features After new-project

`/gsd:new-project` is run **once**. All subsequent feature additions follow one of two paths:

### Path A: Feature fits within the current milestone scope → `add-phase`

Use this when the feature was already implied or expected in the current v1 plan.

```
/gsd:add-phase "기능명"
/gsd:plan-phase N
/gsd:execute-phase N
/gsd:verify-work N
```

Use `/gsd:insert-phase N "기능명"` instead if the feature must be done *before* an already-planned phase.

### Path B: Feature goes beyond the current milestone scope → `new-milestone`

Use this when the feature represents a new direction, a v2 expansion, or was out-of-scope in the original plan.

```
/gsd:complete-milestone 1.0.0     # close current milestone, create git tag
/clear
/gsd:new-milestone "v2.0 기능"    # define new requirements + roadmap
/clear
/gsd:plan-phase 1
/gsd:execute-phase 1
```

### Decision Guide

| Situation | Command |
|---|---|
| Small addition, same goal as v1 | `/gsd:add-phase` |
| Urgent fix needed mid-milestone | `/gsd:insert-phase N` |
| New direction or v2-level scope | `/gsd:new-milestone` |
| One-off task, no phase needed | `/gsd:quick` |
| Trivial change (≤3 files) | `/gsd:fast` |

**Rule of thumb:** If the feature was in scope when you ran `/gsd:new-project`, use `add-phase`. If it changes the product's direction or adds a significant new capability beyond what was originally planned, start a new milestone.

---

## Best Practices

- **`/clear` before and after each phase** — prevents context pollution; start each step cleanly.
- **Run `/gsd:discuss-phase` before planning** — conveys your vision to the planner; significantly improves plan quality.
- **Check with `/gsd:list-phase-assumptions` before executing** — review Claude's intended approach and correct direction early.
- **Use `--research` for complex domains** — specialized areas (3D, audio, ML, etc.) benefit from the research agent.
- **`.planning/` files should be committed to git** — planning artifacts are part of the project history.
