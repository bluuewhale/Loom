# Claude Code Instructions

## Git

- Do **not** add `Co-Authored-By: Claude` or any Claude Code co-author trailer to commit messages.

## Workflow

- For any non-trivial task, use the **GSD (Get-Shit-Done) workflow** via the `gsd:*` skills.
  - Simple tasks can be done directly via `/gsd:fast`.
  - When in doubt, use GSD.
- See `.claude/rules/gsd.md` for detailed GSD command usage and best practices.

## Code / Design / Plan Reviews

- All reviews **must be delegated to a subagent** with an isolated context.
  - Use the `code-review:code-review` skill or dispatch a `superpowers:code-reviewer` subagent.
  - Never review in the main conversation context — keep review logic separate to avoid bias and context pollution.
- **After completing any code, design, or plan, a subagent review is mandatory before considering the task done.**
  - Do not mark a task complete or move to the next step until the subagent review has been received and addressed.
