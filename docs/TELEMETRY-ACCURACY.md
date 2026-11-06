# Recorded calls: accuracy and coverage

The dashboard now separates two counts

- **Verified calls** are observations received from a supported host hook or a managed CLI PATH shim, starting when it was installed
- **History** is the total held in the original local telemetry database, including older imported data and manual `ash track` reports whose provenance cannot be independently established

Historical call counts are retained without resetting or rewriting the old records

## Active sources

| Source | Captures | Limitations |
| --- | --- | --- |
| Claude Code `PostToolUse` and `PostToolUseFailure` | Completed successful or failed `Skill` calls and MCP server tool calls | Does not include other LLM hosts or calls blocked before execution |
| Claude Code `SubagentStart` | Actual launches of Claude subagents | Does not include subagents launched by other tools |
| Managed CLI PATH shims | CLI invocations made through the shim in a shell that inherited the modified PATH | Direct absolute-path execution bypasses the shim |

Each event updates the database inside an inter-process advisory lock and commits the new JSON through an atomic rename

## Setup

```sh
go build -o bin/ash ./cmd/ash
./bin/ash instrument claude install
./bin/ash instrument cli install claude git python3
```

Open a new terminal for the CLI PATH shims, or pass the updated PATH explicitly to non-interactive tools

```sh
./bin/ash instrument claude status
./bin/ash instrument cli status
./bin/ash stats
```

The installer keeps unrelated Claude hooks unchanged and makes the initial recovery copies at `~/.claude/settings.json.ash-backup` and `~/.zshrc.ash-backup`

## Removal decisions

A verified count of zero means no matching event has reached the recorder yet

This can happen because the tool was never invoked, the calling app has no installed hook, the app is running with an old PATH, or the invocation used an absolute executable path

Always inspect installed location, scope, and dependencies before cleanup, especially for bulk selections

## Extending coverage

More hosts need their own official integrations and test fixtures

The `ash hook claude` receiver reads the official JSON event on standard input and exits without blocking the host if telemetry storage fails

Manual `ash track` remains supported for integrations without a native hook, and its increments are shown under **History**, not under verified usage
