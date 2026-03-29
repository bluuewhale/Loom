# Claude Code Instructions

## Git

- Do **not** add `Co-Authored-By: Claude` or any Claude Code co-author trailer to commit messages.

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
