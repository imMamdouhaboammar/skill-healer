# Discovery coverage and identity contracts

## Sources

The scanner reads configured global roots and selected project roots without executing installed tools

Default project roots are `~/Documents`, `~/Projects`, `~/Developer` and `~/Code`

Set `ASH_PROJECT_ROOTS` to a colon-separated list to use an explicit root set, or add `~/.ash/projects.json` with a `roots` array

Project traversal stops at depth 5, at 6,000 directories per root, or after five seconds per root, whichever comes first

The `coverage` field in `/api/pool` exposes visited roots, skipped roots, warnings and discovered project directories

## Supported inventory sources

| Type | Sources |
| --- | --- |
| Skills | Claude, Codex, Gemini, Cursor, canonical `.agents/skills`, their project-local counterparts, and nested manifest folders |
| Agents | Claude, Codex TOML, Gemini, Cursor and canonical agent definition folders, with runtime-observed agents |
| MCP | Claude `.mcp.json`, `~/.claude.json` including project registrations, Gemini, Cursor, VS Code JSON, Codex TOML and Gemini Antigravity |
| CLI | Executable files in user-managed PATH directories, with a smaller explicit list for operating-system directories, plus project `node_modules/.bin`, `.venv/bin` and `bin` |

## Identity and usage

Each discovered installation has its own ID derived from source path and scope when a name is repeated

Name-only host telemetry cannot reliably identify which same-name installation ran

The first occurrence retains the existing aggregate once, other variants remain visible, and every affected entry is marked `usage_attribution=ambiguous_name`

Ambiguous entries are excluded from the automatic no-calls cleanup list

This ambiguity flag applies even when no call was recorded for any copy

The Inventory and Cleanup views have a **Shared names** filter that groups these installations by name and shows their individual source paths for review

The shared-name badge is a review hint, not proof that the installation contents are byte-identical or safe to remove

The cleanup preflight shows the exact source path and warns about shared-name attribution

The separate `ash dedupe` command now previews by default and requires `--apply` for a real change

It only links two skill folders after comparing their complete regular-file trees, retains each original under `~/.ash/vault/dedupe/`, and leaves different-content copies and old backup folders untouched

`ash quiet` also previews by default and uses `--apply` to move eligible symlinks; `load` and `unload` reject conflicting existing destinations instead of overwriting standby data

The dedupe recovery archive is independent of the dashboard Trash list, so restore its archived original manually after inspecting the current link and target

Claude project MCP registrations embedded inside `~/.claude.json` cannot currently be safely removed through the top-level JSON adapter, so deletion is blocked for those entries

Codex TOML MCP deletion and VS Code `servers` deletion are also blocked until format-specific write adapters exist

## Classification

Classification uses a deterministic weighted vocabulary against the installed definition's name and short description, returning a domain, basis and rule-strength score

General results mean available metadata was insufficient for a specific match

The rule-strength score is an explainable heuristic, **not** a calibrated probability and **not** an LLM interpretation

CLI description discovery does not execute arbitrary `--help` scripts, avoiding side effects from untrusted executables

You can manually assign one of the canonical domains in the tool details panel; the preference is persisted locally and takes precedence in dashboard presentation and search

## Official references

- [Claude Skills](https://code.claude.com/docs/en/skills)
- [Claude subagents](https://code.claude.com/docs/en/sub-agents)
- [Claude MCP configuration](https://code.claude.com/docs/en/mcp)
- [Codex Skills](https://learn.chatgpt.com/docs/build-skills)
- [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)
- [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
- [Gemini CLI skills](https://geminicli.com/docs/cli/skills/)
- [Gemini configuration](https://geminicli.com/docs/reference/configuration/)
- [Gemini subagents](https://geminicli.com/docs/core/subagents/)
- [Cursor skills](https://cursor.com/help/customization/skills)
- [Cursor subagents](https://cursor.com/docs/subagents)
- [Cursor MCP](https://cursor.com/guides/coding-agent-mcp)
- [VS Code MCP](https://code.visualstudio.com/docs/agent-customization/mcp-servers)

Cloud-connected, session-defined, and plugin-managed tools may not have a discoverable local artifact, and should be surfaced as coverage gaps until their relevant host adapter is available
