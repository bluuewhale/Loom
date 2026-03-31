# Claude Code Instructions

## Git

- Do **not** add `Co-Authored-By: Claude` or any Claude Code co-author trailer to commit messages.

## Merge Conflict Resolution

Planning files (`ROADMAP.md`, `STATE.md`, `REQUIREMENTS.md`) contain critical project history. **Never blindly overwrite one side.** When resolving conflicts in these files:

- **ROADMAP.md**: Preserve phase entries from both sides. If the same phase number was edited differently, merge the intent — keep the more descriptive goal/plan text.
- **STATE.md**:
  - Frontmatter (`stopped_at`, `last_updated`): use the most precise timestamp; use the most advanced `stopped_at` (higher plan number = more progress).
  - `progress` counters: take the higher values (union of completed work).
  - `Current Position`: take the more advanced phase/plan.
  - Performance log table: **union** — include all rows from both sides.
  - `Decisions`: **union** — keep all entries; drop exact duplicates only.
  - `Session Continuity`: combine the most precise timestamp with the most advanced stopped-at description.
- **REQUIREMENTS.md**: Preserve all requirement entries from both sides; resolve wording conflicts by keeping the more specific/complete text.

When in doubt: **more information is better than less**. A planning file that contains both sides' additions is always preferable to one that silently lost work.

## Workflow

- For any non-trivial task, use the **GSD (Get-Shit-Done) workflow** via the `gsd:*` skills.
  - Simple tasks can be done directly via `/gsd:fast`.
  - When in doubt, use GSD.
- See `.claude/rules/gsd.md` for detailed GSD command usage and best practices.

## Gstack

- Use the `/browse` skill from gstack for all web browsing.
- **Never** use `mcp__claude-in-chrome__*` tools directly.
- Available gstack skills:
  `/office-hours`, `/plan-ceo-review`, `/plan-eng-review`, `/plan-design-review`,
  `/design-consultation`, `/design-shotgun`, `/review`, `/ship`, `/land-and-deploy`,
  `/canary`, `/benchmark`, `/browse`, `/connect-chrome`, `/qa`, `/qa-only`,
  `/design-review`, `/setup-browser-cookies`, `/setup-deploy`, `/retro`,
  `/investigate`, `/document-release`, `/codex`, `/cso`, `/autoplan`,
  `/careful`, `/freeze`, `/guard`, `/unfreeze`, `/gstack-upgrade`

## Code / Design / Plan Reviews

- All reviews **must be delegated to a subagent** with an isolated context.
  - Use the `code-review:code-review` skill or dispatch a `superpowers:code-reviewer` subagent.
  - Never review in the main conversation context — keep review logic separate to avoid bias and context pollution.
- **After completing any code, design, or plan, a subagent review is mandatory before considering the task done.**
  - Do not mark a task complete or move to the next step until the subagent review has been received and addressed.
