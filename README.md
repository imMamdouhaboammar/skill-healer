# AGentic Skills Healer (`ash`)

A high-performance, native macOS agentic CLI system for local pool management, invocation telemetry, lossless deduplication, and on-demand dynamic loading of agent skills, MCP servers, CLIs, and autonomous agents.

## Features

- **Blazing Fast Native Go Engine**: Microsecond startup (<5ms) on macOS Apple Silicon (`darwin/arm64`). Zero external dependencies.
- **Lifetime Lite Counter**: Tracks invocations across all CLIs, MCPs, Skills, and Agents with backfilled historical telemetry (over 334,000 lifetime calls indexed from 500+ past sessions).
- **Activity Tiering**: Classifies every entity as `HOT` (frequently used), `WARM` (occasionally used), or `DORMANT` (never used).
- **Lossless Healing & Deduplication**: Unifies duplicate physical directories across `~/.gemini/config/skills` and `~/.agents/skills` using canonical symlinks with zero data loss.
- **Dynamic On-Demand Loading**: Cuts agent context noise by >90% by parking dormant skills in the Standby Vault while keeping Hot/Essential skills active. Any agent can mount any standby skill in <25ms with `ash load <skill>`.
- **System Doctor**: Audits link integrity, broken symlinks, and directory permissions.

## Architecture & System Structure

```
~/.ash/
├── bin/                 # Local binaries
├── db/
│   └── telemetry.json   # High-speed JSON telemetry store with call counts & tiers
├── shims/               # Passthrough shims for CLI invocation counting
└── vault/
    ├── snapshots/       # Cryptographic SHA-256 archives of previous states
    └── standby/         # Parked standby skills (mounted on-demand)
```

## CLI Usage

```bash
# Full inventory scan across all 4 pillars
ash pool

# View lifetime telemetry and activity tiers
ash stats

# View top most-invoked entities across CLIs, MCPs, and Skills
ash top

# List dormant skills candidates for standby
ash dormant

# Check ecosystem link integrity and broken references
ash doctor

# Run deduplication to convert physical duplicates to canonical symlinks
ash dedupe [--dry-run]

# Activate quiet mode to reduce agent prompt noise
ash quiet [--dry-run] [--min-calls <n>]

# Dynamically mount any standby skill on demand (in <25ms)
ash load <skill-name>

# Unmount a skill back to standby
ash unload <skill-name>

# Manually track an invocation
ash track <cli|mcp|skill|agent> <name>
```

## Local dashboard & agent quick access

Start the local web dashboard and open `http://127.0.0.1:4317` on this Mac

```bash
ash dashboard --port 4317
```

The dashboard includes Inventory, Hot tools, Quick access, Collections, Cleanup, Sources, Activity, Archived and Settings. Press `⌘K` for the command palette

- Pin tools; add display aliases, tags and notes; hide them from normal lists
- Save custom terminal commands, edit them, and copy them with one click. They are never executed automatically
- Save collections that reference any discovered skill, MCP, CLI or agent
- Preview discovered skill Markdown, reveal source paths in Finder and export inventory CSV or JSON
- Clear saved metadata without changing installed tools
- Physically remove eligible user-owned Skills, MCP bundles, and local CLI/Agent entries by moving their installed files into a private Trash vault
- Unregister configured MCP servers by rewriting only their named entry in the relevant JSON configuration, with a local recovery backup
- Review the exact source and operation before removal, enter the tool name to confirm, and restore it from Archived when needed
- Select any number of tools across pages for bulk removal; review every source and type the batch count confirmation. Bulk Pin, Unpin, Hide and Unhide are also available
- Select any number of removed tools in Trash for bulk Restore or permanent deletion with a separate typed confirmation
- Unsupported package-managed CLIs, system binaries and virtual agents are blocked until a source-specific uninstall adapter is available. User-level macOS file permissions still apply
- Move only eligible Gemini skill symlink aliases into standby, with confirmation and restore

### Real cleanup from the CLI

The local dashboard and CLI share the same on-disk Trash and recovery data. Every destructive CLI action requires an explicit confirmation and, for installed sources, the fingerprint returned by a fresh plan

```bash
# Inspect exact removal action, eligibility and source fingerprint
ash cleanup plan --id 'skill:my-skill'

# Move the installed source to ~/.ash/vault/trash
ash cleanup remove --id 'skill:my-skill' --fingerprint '<fingerprint-from-plan>' --confirm 'my-skill'

# List removed entries and their Trash record IDs
ash cleanup trash

# Recover an entry at its original source, refusing to overwrite existing files
ash cleanup restore --id '<trash-record-id>'

# Permanently delete the recovery copy, only after typed confirmation
ash cleanup purge --id '<trash-record-id>' --confirm 'DELETE my-skill'
```

Bulk operations have no tool-count cap. The bulk API requires a verified plan for every entry before removal, and reports partial results if a later source fails. Large confirmed operations may take longer to complete

Saved preferences are kept in `~/.ash/db/dashboard.json` with user-only file permissions

Agents can read the same saved preferences without opening the browser

```bash
ash quick
ash quick --json
```

For reference analysis and deferred capabilities, see [REFERENCE-NOTES.md](REFERENCE-NOTES.md)

## Attribution & Author

- **Author**: Mamdouh Aboammar
- **Ecosystem**: Antigravity / Get-Fable / Agent-Kernel
- **Version**: 1.0.0
