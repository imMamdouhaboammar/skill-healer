# Agentic Skills Healer UX Contract

## Scope and authority

- Audience: local macOS developer or agent operator
- Data source: `pkg/scanner.ScanPool` joined with `pkg/telemetry.Load`
- Locale: English, local browser time for last-seen labels, Gregorian dates
- Target: WCAG 2.2 AA
- UI design: `DESIGN.md`; runtime tokens: `pkg/dashboard/static/styles.css`
- Server: localhost-only static UI, `GET /api/pool`, `GET /api/state`, `GET /api/preview` and guarded scoped mutation endpoints

## Canonical owners

| Capability | Owner | Behavior |
|---|---|---|
| Global scrollbar | `styles.css` | Tokenized baseline for every scroll surface |
| Search/filter/sort/pagination | `app.js` state + URL | Filters are restorable, page clamps after changes |
| Table rows and detail action | `app.js` rendering functions | Consistent metadata and real buttons |
| Details dialog | `index.html` `<dialog>` + `app.js` | Escape, close buttons, focus restoration |
| Loading/error status | `app.js` scan function | Keep last snapshot, show retry, ignore stale responses |
| Persistent preferences | `Store` in `store.go` | Atomic 0600 JSON under `~/.ash/db/dashboard.json`, shared by CLI and UI |
| Agent discovery | `ash quick --json` | Read favorites, commands and collections as structured JSON, without executing stored commands |
| Mutations | `api.go` | Same-origin local session header, strict JSON payload, limited methods and paths |
| Workspaces and references | `REFERENCE-NOTES.md` | Declares the borrowed concept and scope exclusions |

## Navigation and dataset behavior

- Views: Overview, Inventory, Hot tools, Quick access, Collections, Usage analytics, Cleanup, Sources, Activity, Archived, Settings
- View, query, type, sort, order, and page live in URL search params, and browser Back restores them
- Inventory and Cleanup share a selectable page size of 10, 25, 50, 100 or 250 rows, persisted in the browser and reflected in the URL; preview table is bounded to six rows
- Type headers in Inventory and Cleanup toggle grouping by Skill, MCP, CLI and Agent, with alphabetical order inside each group
- Metadata can be saved, cleared, hidden and restored; command shortcuts and saved collections support create, edit, remove
- Cmd K opens a native search modal across pages, local tools and saved commands
- On a narrow screen, the data grid scrolls horizontally without removing columns
- Page titles follow `{View} | Agentic Skills Healer`
- Search clears immediately, respects IME composition, and shows no-match feedback

## Tracking and cleanup

- `call_count` represents calls recorded by the current CLI telemetry implementation
- A zero count means "no recorded calls"; no inactivity claim is inferred from missing integrations
- Active-in-30-days requires a recorded `last_seen` date within 30 days
- Cleanup is a real source-aware removal operation with a fingerprinted preflight plan and exact-name confirmation
- Eligible user-owned files and directories are moved out of their installed location into `~/.ash/vault/trash`; MCP configuration entries are removed by key and saved privately for recovery
- System/package-managed binaries and virtual agents have no automatic removal adapter and are blocked with a reason
- Multi-select spans all inventory/cleanup pages without an arbitrary count limit; Select page and Clear selection are available, and destructive batches require typed count confirmation
- Bulk removal requires a per-source plan and a typed `REMOVE N TOOLS` confirmation; unsupported or stale entries block the entire preflight
- Bulk updates support Pin, Unpin, Hide and Unhide, and Trash supports bulk Restore and permanent deletion requiring `DELETE N RECOVERY COPIES`
- Bulk execution is source-by-source. The result reports successful and failed entries, and earlier successes remain recoverable if a later one fails
- Standby is restricted to symlink aliases directly under `~/.gemini/config/skills`; canonical content remains in place
- Restoring a standby alias refuses destination conflicts, and no file tree is deleted by standby operations
- Clearing preferences deletes only user-authored metadata; hiding removes an item from standard lists, not from disk
- Custom quick commands copy to the clipboard and are never executed by the dashboard

## Resilience and accessibility

- Initial loading keeps stable panel geometry; failed refresh retains any successful snapshot and provides Retry
- A new refresh aborts the previous one so stale results cannot overwrite newer data
- Tool names, paths and domains render as text nodes, not executable HTML
- Buttons, links, table headers, and dialog use native semantic controls with visible focus
- API responses are uncached and served from loopback with restrictive response headers, a session token and Origin validation for writes
- Motion respects `prefers-reduced-motion`

## Verification

- Go unit tests verify scan delivery, store persistence, session guarding, restricted file movements and source preview
- JS syntax check and live API request verify frontend loading compatibility
- Browser review targets wide desktop and narrow mobile views, search/no-results, navigation, sorting, dialog, error and reduced-motion states
