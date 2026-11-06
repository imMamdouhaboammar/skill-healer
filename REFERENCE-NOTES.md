# Skills Manager reference analysis

Reference repository: https://github.com/xingkongliang/skills-manager

Clone for research: `/tmp/ash-skills-manager.fQsrp4` on the Mac, commit `d30530c`

Read the README and inspected the React command palette, skill detail panel, preset bar, card actions, sidebar and Rust path-guard/removal logic

## Concepts carried into AGentic Skills Healer

| Reference concept | Implemented locally | Behavior |
|---|---|---|
| Command palette | Yes | Cmd K opens searchable tool, page and saved-command actions |
| Skill preview | Yes | Read a discovered skill's SKILL.md or README.md, capped at 32 KB |
| Per-skill labels and tagging | Yes | Optional alias, notes, tags, pinning and hide flags |
| Presets | Partial | Named collections for quick browsing and CRUD, without deployment |
| Quick per-card actions | Yes | Finder reveal, source path copy, pin/unpin and details |
| Activity and cleanup | Existing | Real recorded calls with incomplete-coverage warning |
| Safe removal guardrails | Partial | Reversible standby limited to known Gemini symlinks |
| Export and backup | Partial | Explicit local JSON / CSV download, no remote sync |
| Agent-facing access | Yes | `ash quick --json` reads favorites, saved commands and collections from the same local state |

## Deliberately deferred

- Cross-agent skill installation and sync, with path overrides and conflict previews
- Git-backed history, restore points and multi-device synchronization
- Git and marketplace installation, archive import, update checking
- Bulk enable/disable, cross-project drift, agent-scoped deployment presets
- Full-source diff and upstream comparison
- Event-level logging of management actions and reliable tracking for invocations outside instrumented adapters

These need a separate capability model, dependency inspection, source provenance and rollback design before exposing mutating controls across all discovered integrations

No source code was copied from the reference clone. The frontend keeps its own visual and interaction design
