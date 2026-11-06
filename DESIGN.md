---
version: alpha
name: Agentic Skills Healer
description: A quiet, dense macOS workspace console for inspecting and managing installed agentic tools
colors:
  primary: "#bf684e"
  primary-hover: "#a94e37"
  background: "#f7f7f5"
  surface: "#ffffff"
  sidebar: "#f5f5f2"
  text: "#242421"
  text-secondary: "#64645f"
  border: "#e7e6e2"
  success: "#2e9457"
  warning: "#b67c28"
typography:
  body:
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif'
  data:
    fontFamily: 'ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, monospace'
rounded:
  DEFAULT: "8px"
  control: "8px"
  panel: "11px"
  dialog: "15px"
spacing:
  panel: "20px"
  column-gap: "17px"
  sidebar-width: "252px"
components:
  button: {}
  table: {}
  search: {}
  dialog: {}
---

# Agentic Skills Healer Design

## Overview

The visual reference is a native macOS inspection utility: precise controls, restrained borders, compact data, and no decorative dashboard effects

- Audience: a developer or agent operator managing installed skills, MCP servers, CLIs and agents on one Mac
- Register: product/operate, English UI for the initial local build
- Signature: an inventory-first table with direct source visibility and honest telemetry coverage
- Scope: inventory, local preferences, saved collections, command shortcuts, guarded alias standby, source-aware removal, multi-selection, bulk actions and private Trash recovery
- Avoid: gradients, neon, illustration tiles, decorative charts, misleading usage claims and unchecked deletion
- Token ownership: this document records intent; `pkg/dashboard/static/styles.css` owns runtime tokens and `controls.css` extends components without redefining tokens

## Colors

Neutral surfaces and hairline separation carry density. Terracotta is reserved for selected navigation, recorded-use bars and primary actions. Green means a verified positive state; amber identifies review and uncertainty. No color is the sole status indicator

## Typography

macOS system sans supports rapid reading and familiar control geometry. Mono is limited to paths, counts, timestamps and endpoint information, with tabular numerals for comparison. Headings are compact and slightly tightened

## Layout

Desktop uses a 252px persistent sidebar, summary strip, two-column insight region, and a responsive inventory table. At 860px the sidebar becomes horizontally scrollable navigation. Tables retain their columns through visible horizontal overflow. The table viewport owns scrolling; the page and other sections retain natural height

## Elevation & Depth

Static panels use borders and background contrast without a drop shadow. Modal dialogs use a directional soft shadow to distinguish focus. All loading states retain the existing surface geometry

## Shapes

Controls use 8px corners, panels 11px, the detail dialog 15px. Pills and small tags signal metadata only. Icons use the same thin, rounded SVG stroke grammar

## Components

### Foundational visual states

Controls have visible hover, active, disabled and keyboard-focus treatments. Error feedback persists until a successful refresh. Empty states sit inside the affected content surface and suggest the next relevant investigation

### Buttons and actions

The principal action is a fresh scan. Cards expose pin, copy, inspect and Finder reveal. Inventory and cleanup tables include selection checkboxes and page-level selection. Bulk Pin/Hide controls are reversible; physical removal opens a source-by-source review and typed batch confirmation. Trash supports individual and bulk Restore, as well as permanent deletion with a separate confirmation. Busy buttons retain dimensions

### Navigation and data display

Navigation has stable labels and active markers. Tables use semantic headings, sortable inventory columns, bounded pagination, and row-level details. Short counts use mono numerals; paths allow wrapping in details

### Forms and overlays

Search uses an app-owned clear control and IME-safe local filtering. Details, command editing, collections, source preview, Cmd K search and confirmation use native dialogs. Form fields are labeled, metadata persists locally and the close controls preserve Escape behavior

### Iconography

Consistent inline SVG stroke icons at 16–20px, always paired with text in primary navigation and actions

### Motion

Only state feedback transitions at approximately 150ms and a short data-bar interpolation. Reduced-motion preference disables them

### Content and data visualization

Copy names observed facts precisely: recorded invocations, not all invocations; zero recorded calls, not proven non-use. Charts show actual counts and have numeric labels alongside bar lengths

## Do's and Don'ts

- Do keep file sources visible in details and source lists
- Do retain usable data after a failed refresh
- Do keep original scanner names in stored inventory while optional display aliases affect presentation only
- Do make saved quick commands copy-only, never one-click executable
- Do offer one-click Finder reveal only for paths validated against the current scanner inventory
- Do treat collection presets as saved groups, not an implicit multi-agent deployment
- Don't present incomplete telemetry as definitive usage history
- Don't put deletion behind a single button while dependency evidence is unavailable
