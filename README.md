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

## Attribution & Author

- **Author**: Mamdouh Aboammar
- **Ecosystem**: Antigravity / Get-Fable / Agent-Kernel
- **Version**: 1.0.0
