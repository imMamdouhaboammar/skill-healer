(() => {
  "use strict";

  const VIEW_COPY = {
    overview: ["Your agentic workspace, at a glance", "All the tools on this Mac, organized in one place"],
    inventory: ["Installed inventory", "Find, inspect, and compare every discovered tool"],
    hot: ["Hot tools", "The tools you reach for most, with actions one click away"],
    quick: ["Quick access", "Pinned tools and your own ready-to-copy terminal commands"],
    collections: ["Collections", "Saved groups and tags across every kind of tool"],
    usage: ["Usage analytics", "Recorded invocations across your local workspace"],
    cleanup: ["Cleanup & removal", "Remove installed tools from supported local sources"],
    sources: ["Sources", "Where your installed tools were discovered"],
    activity: ["Recent activity", "The latest recorded use for each instrumented tool"],
    archived: ["Trash, standby & hidden", "Restore removed tools or permanently delete recovery copies"],
    settings: ["Settings", "Local connection and tracking information"],
  };
  const TYPES = ["all", "skill", "mcp", "cli", "agent"];
  const TYPE_ORDER = { skill: 0, mcp: 1, cli: 2, agent: 3 };
  const TYPE_LABELS = { all: "All", skill: "Skills", mcp: "MCPs", cli: "CLIs", agent: "Agents" };
  const TYPE_ICONS = { skill: "stack", mcp: "nodes", cli: "terminal", agent: "agent" };
  const TYPE_KEYS = { skill: "skills", mcp: "mcps", cli: "clis", agent: "agents" };
  const PAGE_SIZES = [10, 25, 50, 100, 250];
  const fmt = new Intl.NumberFormat("en-US");
  const $ = (id) => document.getElementById(id);
  const state = {
    pool: null, items: [], view: "overview", type: "all", query: "",
    sort: "call_count", order: "desc", page: 1, cleanupPage: 1, pageSize: 25, cleanupFilter: "unused", variantOnly: false, sharedNameIDs: new Set(),
    loading: false, request: null, selected: null, lastScan: null,
    session: "", saved: { tools: {}, shortcuts: {}, presets: {}, standby: {}, trash: {} },
    selectedPreset: null, draftPresetIDs: new Set(), activeTag: "", confirming: null,
    cleanupPlan: null, purgeRecord: null,
    selectedTools: new Set(), selectedTrash: new Set(), bulkMode: "", bulkPlans: [], bulkIDs: [],
  };

  const pref = (item) => state.saved.tools[item.id] || {};
  const displayName = (item) => pref(item).alias || item.name;
  const visible = () => state.items.filter((item) => !pref(item).hidden);
  const standbyEligible = (item) => item.type === "skill" && item.is_symlink && item.path.endsWith(`/.gemini/config/skills/${item.name}`);
  const favoriteItems = () => state.items.filter((item) => pref(item).favorite).sort((a, b) => b.call_count - a.call_count);
  const topItems = () => visible().filter((item) => item.verified_calls > 0).sort((a, b) => b.verified_calls - a.verified_calls);

  function toast(message, isError = false) {
    const target = $("toast");
    target.textContent = message;
    target.classList.toggle("toast-error", isError);
    target.hidden = false;
    window.clearTimeout(toast.timeout);
    toast.timeout = window.setTimeout(() => { target.hidden = true; }, 3500);
  }

  async function copyText(value, label = "Copied to clipboard") {
    if (!value) return toast("No path or command available", true);
    try { await navigator.clipboard.writeText(value); toast(label); }
    catch { toast("Clipboard access unavailable", true); }
  }

  async function refreshSaved() {
    const response = await fetch("/api/state", { cache: "no-store" });
    if (!response.ok) throw new Error("Cannot load saved settings");
    const saved = await response.json();
    state.saved = { tools: saved.tools || {}, shortcuts: saved.shortcuts || {}, presets: saved.presets || {}, standby: saved.standby || {}, trash: saved.trash || {} };
  }

  async function mutation(path, method, data, message) {
    if (!state.session) { toast("Scan inventory first to start a local session", true); return false; }
    try {
      const response = await fetch(path, { method, headers: { "Content-Type": "application/json", "X-ASH-Session": state.session }, body: JSON.stringify(data) });
      if (!response.ok) {
        let detail = `Action could not be completed (${response.status})`;
        try { const body = await response.text(); detail = body.trim().slice(0, 300) || detail; } catch { /* Preserve the HTTP fallback */ }
        throw new Error(detail);
      }
      await refreshSaved();
      render();
      if (message) toast(message);
      return true;
    } catch (error) { toast(error.message || "Action failed", true); return false; }
  }

  function el(tag, className = "", text = null) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== null) node.textContent = String(text);
    return node;
  }

  function icon(name) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `#i-${name}`);
    svg.append(use);
    svg.setAttribute("aria-hidden", "true");
    return svg;
  }

  function normalizePool(pool) {
    return Object.entries(TYPE_KEYS).flatMap(([type, key]) =>
      Object.values(pool[key] || {}).map((item) => ({
        id: String(item.id || `${type}:${item.name}`),
        name: String(item.name || "Unnamed"),
        type,
        domain: String(item.domain || "General utility"),
        description: String(item.description || ""),
        classification_confidence: Number(item.classification_confidence) || 0,
        classification_basis: String(item.classification_basis || ""),
        discovery_source: String(item.discovery_source || ""),
        duplicate_of: String(item.duplicate_of || ""),
        usage_attribution: String(item.usage_attribution || ""),
        path: String(item.path || ""),
        scope: String(item.scope || "unknown"),
        status: String(item.status || "unknown"),
        tier: String(item.tier || "DORMANT").toLowerCase(),
        call_count: Math.max(0, Number(item.call_count) || 0),
        verified_calls: Math.max(0, Number(item.verified_calls) || 0),
        first_seen: item.first_seen || null,
        last_seen: item.last_seen || null,
        is_symlink: Boolean(item.is_symlink),
        symlink_target: String(item.symlink_target || ""),
      }))
    ).sort((a, b) => a.name.localeCompare(b.name));
  }

  function counts() {
    const items = visible();
    const result = { all: items.length, skill: 0, mcp: 0, cli: 0, agent: 0 };
    items.forEach((item) => { result[item.type] += 1; });
    return result;
  }

  function num(value) { return fmt.format(value); }
  function safeDate(value) { const date = value ? new Date(value) : null; return date && !Number.isNaN(date.valueOf()) ? date : null; }
  function observedAgo(value) {
    const date = safeDate(value);
    if (!date) return "Not recorded";
    const days = Math.max(0, Math.floor((Date.now() - date.getTime()) / 86_400_000));
    if (days === 0) return "Today";
    if (days === 1) return "Yesterday";
    if (days < 30) return `${days} days ago`;
    if (days < 365) return `${Math.floor(days / 30)} months ago`;
    return date.toLocaleDateString("en-US", { month: "short", year: "numeric" });
  }

  function typePill(type) { return el("span", "pill", TYPE_LABELS[type] || type); }
  function scopePill(scope) {
    const label = scope === "canonical" ? "Global" : scope === "installed_bundle" ? "Bundle" : scope === "system" ? "System" : scope === "runtime" ? "Runtime" : scope === "global" ? "Global" : scope;
    const cls = scope === "project" ? "pill pill-blue" : scope === "global" || scope === "canonical" ? "pill pill-green" : "pill";
    return el("span", cls, label);
  }
  function tierPill(tier) { return el("span", `tier ${tier}`, tier[0]?.toUpperCase() + tier.slice(1)); }
  function toolSymbol(item) { const holder = el("span", "tool-symbol"); holder.append(icon(TYPE_ICONS[item.type])); return holder; }
  function toolCell(item) {
    const wrap = el("span", "tool-cell");
    wrap.append(toolSymbol(item), el("span", "tool-name", displayName(item)));
    if (pref(item).favorite) { const mark = el("span", "favorite-mark", "★"); mark.title = "Pinned"; wrap.append(mark); }
    if (state.sharedNameIDs.has(item.id)) { const mark = el("span", "pill pill-orange", "Shared name"); mark.title = "Multiple installations have this name. Their contents may differ"; wrap.append(mark); }
    return wrap;
  }
  function addCell(row, child, cls = "") { const td = el("td", cls); td.append(child instanceof Node ? child : document.createTextNode(String(child))); row.append(td); }

  function detailButton(item) {
    const button = el("button", "row-detail");
    button.type = "button";
    button.setAttribute("aria-label", `View details for ${item.name}`);
    button.dataset.detail = item.id;
    button.append(icon("chevron"));
    return button;
  }

  function selectToolCheckbox(item) {
    const input = el("input", "row-select");
    input.type = "checkbox";
    input.dataset.selectTool = item.id;
    input.setAttribute("aria-label", `Select ${item.name}`);
    input.checked = state.selectedTools.has(item.id);
    return input;
  }

  function renderRows(targetId, records, mode = "inventory", emptyText = "No matching tools") {
    const target = $(targetId);
    const fragment = document.createDocumentFragment();
    if (!records.length) {
      const row = el("tr");
      const columns = mode === "cleanup" ? 7 : mode === "overview" ? 7 : 10;
      const cell = el("td", "table-message", emptyText);
      cell.colSpan = columns;
      row.append(cell); fragment.append(row);
    }
    records.forEach((item) => {
      const row = el("tr");
      if (mode === "inventory" || mode === "cleanup") addCell(row, selectToolCheckbox(item), "select-column");
      addCell(row, toolCell(item));
      addCell(row, typePill(item.type));
      if (mode === "inventory") addCell(row, pref(item).domain || item.domain);
      if (mode === "cleanup") {
        addCell(row, el("span", "source-title", item.path || "Unknown source"));
        addCell(row, el("span", item.usage_attribution === "ambiguous_name" ? "pill pill-orange" : item.call_count ? "pill pill-green" : "pill pill-orange", item.usage_attribution === "ambiguous_name" ? "Shared-name usage" : item.call_count ? "Recorded use" : "No calls recorded"));
        addCell(row, el("span", "mono-number", num(item.call_count)), "right");
      } else {
        addCell(row, scopePill(item.scope));
        if (mode === "inventory") addCell(row, el("span", "mono-number", num(item.verified_calls)), "right");
        addCell(row, el("span", "mono-number", num(item.call_count)), "right");
        addCell(row, observedAgo(item.last_seen));
        addCell(row, tierPill(item.tier));
      }
      if (mode === "cleanup") {
        const actions = el("div", "cleanup-actions");
        actions.append(actionButton("Remove…", "trash", "cleanup", item.id, "danger"), detailButton(item));
        addCell(row, actions);
      } else addCell(row, detailButton(item));
      fragment.append(row);
    });
    target.replaceChildren(fragment);
  }

  function renderTabs(containerId) {
    const parent = $(containerId);
    const data = counts();
    const buttons = TYPES.map((type) => {
      const button = el("button", "filter-chip");
      button.type = "button";
      button.dataset.type = type;
      button.setAttribute("aria-pressed", String(type === state.type));
      button.append(document.createTextNode(TYPE_LABELS[type]), el("span", "chip-count", num(data[type])));
      return button;
    });
    parent.replaceChildren(...buttons);
  }

  function filtered() {
    const q = state.query.trim().toLocaleLowerCase();
    return visible().filter((item) =>
      (state.type === "all" || state.type === item.type) &&
      (!state.variantOnly || state.sharedNameIDs.has(item.id)) &&
      (!q || [item.name, pref(item).alias || "", ...(pref(item).tags || []), pref(item).domain || item.domain, item.path, item.type, item.scope].some((field) => field.toLocaleLowerCase().includes(q)))
    );
  }

  function sorted(items) {
    const dir = state.order === "asc" ? 1 : -1;
    return [...items].sort((a, b) => {
      if (state.sort === "type") return ((TYPE_ORDER[a.type] ?? 99) - (TYPE_ORDER[b.type] ?? 99)) * dir || a.name.localeCompare(b.name);
      if (state.sort === "call_count") return (a.call_count - b.call_count) * dir || a.name.localeCompare(b.name);
      if (state.sort === "last_seen") return ((safeDate(a.last_seen)?.getTime() || 0) - (safeDate(b.last_seen)?.getTime() || 0)) * dir || a.name.localeCompare(b.name);
      return a.name.localeCompare(b.name) * dir;
    });
  }

  function renderBars(containerId) {
    const totals = Object.fromEntries(TYPES.slice(1).map((type) => [type, 0]));
    state.items.forEach((item) => { totals[item.type] += item.verified_calls; });
    const max = Math.max(1, ...Object.values(totals));
    const fragment = document.createDocumentFragment();
    TYPES.slice(1).forEach((type) => {
      const row = el("div", "bar-row");
      const track = el("div", "bar-track");
      const bar = el("span", "bar-fill");
      bar.style.width = `${totals[type] / max * 100}%`;
      track.append(bar);
      row.append(el("span", "bar-name", TYPE_LABELS[type]), track, el("span", "bar-value", num(totals[type])));
      fragment.append(row);
    });
    $(containerId).replaceChildren(fragment);
  }

  function observedItems() {
    return state.items.filter((item) => item.verified_calls > 0 && safeDate(item.last_seen)).sort((a, b) => safeDate(b.last_seen) - safeDate(a.last_seen));
  }

  function renderActivity(targetId, limit) {
    const records = observedItems().slice(0, limit);
    const fragment = document.createDocumentFragment();
    if (!records.length) fragment.append(el("li", "empty-message", "No calls recorded yet"));
    records.forEach((item) => {
      const li = el("li");
      const symbol = el("span", "activity-icon"); symbol.append(icon("clock"));
      const text = el("span", "activity-title");
      text.append(el("strong", "", item.name), document.createTextNode(` · ${TYPE_LABELS[item.type]} · ${num(item.verified_calls)} verified calls`));
      li.append(symbol, text, el("span", "activity-time", observedAgo(item.last_seen)));
      fragment.append(li);
    });
    $(targetId).replaceChildren(fragment);
  }

  function cleanupCandidates() { return visible().filter((item) => item.call_count === 0 && item.usage_attribution !== "ambiguous_name").sort((a, b) => a.name.localeCompare(b.name)); }
  function cleanupRecords() {
    if (state.cleanupFilter === "variants") return visible().filter((item) => state.sharedNameIDs.has(item.id)).sort((a, b) => a.name.localeCompare(b.name));
    return state.cleanupFilter === "all" ? [...visible()].sort((a, b) => a.name.localeCompare(b.name)) : cleanupCandidates();
  }

  function updateToolSelectionUI() {
    const count = state.selectedTools.size;
    for (const view of ["inventory", "cleanup"]) {
      $(`${view}-selected-count`).textContent = `${num(count)} selected across pages`;
      $(`${view}-bulk-toolbar`).hidden = count === 0;
      const rows = [...$(`${view}-table`).querySelectorAll("[data-select-tool]")];
      const selected = rows.filter((input) => state.selectedTools.has(input.dataset.selectTool)).length;
      const toggle = $(`${view}-select-page`);
      toggle.checked = rows.length > 0 && selected === rows.length;
      toggle.indeterminate = selected > 0 && selected < rows.length;
    }
  }

  function updateTrashSelectionUI() {
    const selected = state.selectedTrash.size;
    $("trash-selected-count").textContent = `${num(selected)} selected`;
    $("trash-bulk-restore").disabled = !selected;
    $("trash-bulk-purge").disabled = !selected;
    const rows = [...$("trash-list").querySelectorAll("[data-select-trash]")];
    const count = rows.filter((input) => state.selectedTrash.has(input.dataset.selectTrash)).length;
    $("trash-select-all").checked = !!rows.length && count === rows.length;
    $("trash-select-all").indeterminate = count > 0 && count < rows.length;
    $("trash-select-all").disabled = !rows.length;
  }

  function changeSelection(selection, id, checked) {
    if (checked) selection.add(id); else selection.delete(id);
    return true;
  }

  function selectPage(view, checked) {
    const rows = [...$(`${view}-table`).querySelectorAll("[data-select-tool]")];
    rows.forEach((input) => {
      changeSelection(state.selectedTools, input.dataset.selectTool, checked);
      input.checked = state.selectedTools.has(input.dataset.selectTool);
    });
    updateToolSelectionUI();
  }

  function selectTrashAll(checked) {
    const rows = [...$("trash-list").querySelectorAll("[data-select-trash]")];
    rows.forEach((input) => {
      changeSelection(state.selectedTrash, input.dataset.selectTrash, checked);
      input.checked = state.selectedTrash.has(input.dataset.selectTrash);
    });
    updateTrashSelectionUI();
  }

  function renderCleanupPreview() {
    const items = cleanupCandidates().slice(0, 3);
    const fragment = document.createDocumentFragment();
    if (!items.length) fragment.append(el("p", "empty-message", "Every discovered tool has recorded activity"));
    items.forEach((item) => {
      const row = el("div", "cleanup-item");
      row.append(toolSymbol(item), el("span", "cleanup-name", item.name), el("span", "cleanup-reason", "No recorded calls"));
      fragment.append(row);
    });
    $("cleanup-preview").replaceChildren(fragment);
  }

  function actionButton(label, iconName, action, id, kind = "outline") {
    const button = el("button", `button button-mini button-${kind}`);
    button.type = "button";
    button.dataset[action] = id;
    button.append(icon(iconName), document.createTextNode(label));
    return button;
  }

  function toolCard(item, compact = false) {
    const card = el("article", `tool-card${compact ? " tool-card-compact" : ""}`);
    const heading = el("div", "tool-card-heading");
    const titles = el("div", "tool-card-titles");
    titles.append(el("strong", "tool-card-name", displayName(item)), el("span", "tool-card-kind", `${TYPE_LABELS[item.type]} · ${pref(item).domain || item.domain}`));
    const favorite = el("button", `star-toggle${pref(item).favorite ? " is-favorite" : ""}`);
    favorite.type = "button"; favorite.title = pref(item).favorite ? "Unpin tool" : "Pin tool";
    favorite.setAttribute("aria-label", `${pref(item).favorite ? "Unpin" : "Pin"} ${displayName(item)}`);
    favorite.setAttribute("aria-pressed", String(Boolean(pref(item).favorite)));
    favorite.dataset.pin = item.id; favorite.append(icon("star"));
    heading.append(toolSymbol(item), titles, favorite);
    const detail = el("div", "tool-card-stats");
    detail.append(el("span", "mono-number", `${num(item.call_count)} calls`), el("span", "tier-text", observedAgo(item.last_seen)));
    const actions = el("div", "tool-card-actions");
    actions.append(actionButton("Details", "chevron", "detail", item.id));
    if (item.path) {
      actions.append(actionButton("Copy path", "copy", "copytool", item.id));
      actions.append(actionButton("Finder", "open", "reveal", item.id));
    }
    card.append(heading, detail, actions);
    return card;
  }

  function showEmpty(target, heading, detail) {
    const message = el("div", "empty-state");
    message.append(el("strong", "", heading), el("p", "", detail));
    target.replaceChildren(message);
  }

  function renderHot() {
    const top = topItems();
    if (state.view === "overview") {
      const strip = $("overview-hot");
      const picks = [...favoriteItems().filter((item) => !pref(item).hidden), ...top.filter((item) => !pref(item).favorite)].slice(0, 5);
      if (!picks.length) showEmpty(strip, "No recorded activity yet", "Pin a tool to keep it here");
      else strip.replaceChildren(...picks.map((item) => toolCard(item, true)));
    } else if (state.view === "hot") {
      const grid = $("hot-grid");
      if (!top.length) showEmpty(grid, "No calls recorded yet", "Your most used tools will show up here as telemetry arrives");
      else grid.replaceChildren(...top.slice(0, 32).map((item) => toolCard(item)));
    }
  }

  function sortedValues(object) { return Object.values(object || {}).sort((a, b) => new Date(b.updated || b.moved_at) - new Date(a.updated || a.moved_at)); }

  function renderQuick() {
    const favorites = favoriteItems();
    $("favorite-count").textContent = `${favorites.length} PINNED`;
    if (!favorites.length) showEmpty($("favorite-grid"), "No favorite tools", "Open any tool and choose Pin to Quick access");
    else $("favorite-grid").replaceChildren(...favorites.map((item) => toolCard(item, true)));
    const shortcuts = sortedValues(state.saved.shortcuts);
    if (!shortcuts.length) showEmpty($("shortcut-list"), "No saved commands", "Save commands you use often and copy them with one click");
    else $("shortcut-list").replaceChildren(...shortcuts.map((shortcut) => {
      const row = el("div", "shortcut-row");
      const detail = el("div", "shortcut-detail");
      detail.append(el("strong", "", shortcut.label), el("code", "", shortcut.command));
      const actions = el("div", "shortcut-actions");
      actions.append(actionButton("Copy", "copy", "copycommand", shortcut.id, "soft"), actionButton("Edit", "edit", "editcommand", shortcut.id), actionButton("Remove", "trash", "deletecommand", shortcut.id));
      row.append(detail, actions); return row;
    }));
  }

  function renderCollections() {
    const presets = sortedValues(state.saved.presets);
    const container = $("preset-list");
    if (!presets.length) showEmpty(container, "No collections yet", "Group related tools into named collections for direct access");
    else container.replaceChildren(...presets.map((preset) => {
      const card = el("article", "preset-card");
      const head = el("div", "preset-head");
      const symbol = el("span", "preset-symbol"); symbol.append(icon("stack"));
      const titles = el("div"); titles.append(el("h3", "", preset.name), el("span", "", `${preset.tool_ids.length} saved tools`));
      head.append(symbol, titles);
      const tools = el("div", "preset-members");
      const members = preset.tool_ids.map((id) => state.items.find((item) => item.id === id)).filter(Boolean);
      members.slice(0, 5).forEach((item) => tools.append(el("span", "member-pill", displayName(item))));
      if (members.length > 5) tools.append(el("span", "member-pill muted", `+${members.length - 5}`));
      const actions = el("div", "tool-card-actions");
      actions.append(actionButton("Browse", "open", "browsepreset", preset.id, "soft"), actionButton("Edit", "edit", "editpreset", preset.id), actionButton("Remove", "trash", "deletepreset", preset.id));
      card.append(head, tools, actions); return card;
    }));
    const allTags = new Map();
    state.items.forEach((item) => (pref(item).tags || []).forEach((tag) => {
      const key = tag.toLowerCase();
      if (!allTags.has(key)) allTags.set(key, { tag, count: 0 });
      allTags.get(key).count++;
    }));
    const tags = [...allTags.values()].sort((a, b) => b.count - a.count);
    const tagWrap = $("tags-list");
    if (!tags.length) showEmpty(tagWrap, "No tags created", "Add tags from a tool’s details");
    else tagWrap.replaceChildren(...tags.map(({ tag, count }) => {
      const button = el("button", "filter-chip", `${tag} · ${count}`);
      button.type = "button"; button.dataset.tag = tag; button.setAttribute("aria-pressed", String(state.activeTag.toLowerCase() === tag.toLowerCase()));
      return button;
    }));
    const tagged = state.activeTag ? state.items.filter((item) => (pref(item).tags || []).some((tag) => tag.toLowerCase() === state.activeTag.toLowerCase())) : [];
    $("tag-results").replaceChildren(...tagged.slice(0, 24).map((item) => toolCard(item, true)));
    const selectedPreset = state.saved.presets[state.selectedPreset];
    const browser = $("preset-browser");
    if (selectedPreset) {
      $("preset-browser-title").textContent = selectedPreset.name;
      const matched = selectedPreset.tool_ids.map((id) => state.items.find((item) => item.id === id)).filter(Boolean);
      if (!matched.length) showEmpty($("preset-browser-list"), "No matching installed tools", "The collection retains references to tools that are no longer discovered");
      else $("preset-browser-list").replaceChildren(...matched.map((item) => toolCard(item, true)));
      browser.hidden = false;
    } else browser.hidden = true;
  }

  function renderArchived() {
    const removed = Object.values(state.saved.trash || {}).sort((a, b) => new Date(b.removed_at) - new Date(a.removed_at));
    $("trash-count").textContent = `${num(removed.length)} ITEMS`;
    if (!removed.length) showEmpty($("trash-list"), "Trash is empty", "Removed tools appear here until you restore or permanently delete them");
    else $("trash-list").replaceChildren(...removed.map((record) => {
      const row = el("div", "archive-row trash-row");
      const checkbox = el("input", "row-select trash-select");
      checkbox.type = "checkbox"; checkbox.dataset.selectTrash = record.id;
      checkbox.checked = state.selectedTrash.has(record.id);
      checkbox.setAttribute("aria-label", `Select removed ${record.name}`);
      const middle = el("div", "trash-record-info");
      middle.append(el("strong", "", record.name), el("span", "source-meta", `${TYPE_LABELS[record.type] || record.type} · Removed ${observedAgo(record.removed_at)}`), el("code", "", record.source_path));
      const actions = el("div", "trash-actions");
      actions.append(actionButton("Restore", "restore", "cleanuprestore", record.id, "soft"), actionButton("Delete forever", "trash", "cleanuppurge", record.id, "danger"));
      row.append(checkbox, middle, actions);
      return row;
    }));
    updateTrashSelectionUI();
    const hidden = state.items.filter((item) => pref(item).hidden);
    $("hidden-count").textContent = `${hidden.length} HIDDEN`;
    if (!hidden.length) showEmpty($("hidden-list"), "No hidden items", "Tools you hide from standard inventory appear here");
    else $("hidden-list").replaceChildren(...hidden.map((item) => {
      const card = toolCard(item, true);
      card.append(actionButton("Unhide", "restore", "unhide", item.id, "soft"));
      return card;
    }));
    const records = sortedValues(state.saved.standby);
    if (!records.length) showEmpty($("standby-list"), "Standby is empty", "Eligible aliases can be moved here from tool details");
    else $("standby-list").replaceChildren(...records.map((record) => {
      const row = el("div", "archive-row");
      const middle = el("div"); middle.append(el("strong", "", record.name), el("code", "", record.original_path));
      row.append(middle, actionButton("Restore alias", "restore", "restore", record.id, "soft"));
      return row;
    }));
  }

  function renderOverview() {
    const all = state.items;
    const used = all.filter((item) => item.verified_calls > 0);
    const active = all.filter((item) => item.verified_calls > 0 && safeDate(item.last_seen) && Date.now() - safeDate(item.last_seen).getTime() <= 30 * 86_400_000);
    $("metric-total").textContent = num(all.length);
    $("metric-calls").textContent = num(all.reduce((sum, item) => sum + item.verified_calls, 0));
    $("metric-active").textContent = num(active.length);
    $("metric-candidates").textContent = num(all.length - used.length);
    $("metric-active-note").textContent = all.length ? `${Math.round(active.length / all.length * 100)}% of discovered tools` : "Observed activity";
    $("nav-cleanup-count").textContent = num(cleanupCandidates().length);
    $("inventory-caption").textContent = `${num(all.length)} discovered tools`;
    renderTabs("overview-types");
    renderRows("overview-table", sorted(filtered()).slice(0, 6), "overview", "No tools match your search");
    renderBars("overview-bars");
    const pct = all.length ? Math.round(used.length / all.length * 100) : 0;
    $("coverage-value").textContent = `${pct}% verified`;
    $("coverage-fill").style.width = `${pct}%`;
    renderActivity("recent-list", 5);
    renderCleanupPreview();
    renderHot();
  }

  function updateSortHeaders() {
    document.querySelectorAll("th[data-sort-col]").forEach((header) => {
      const active = header.dataset.sortCol === state.sort;
      if (active) header.setAttribute("aria-sort", state.order === "asc" ? "ascending" : "descending");
      else header.removeAttribute("aria-sort");
      header.querySelector(".sort-indicator").textContent = active ? state.order === "asc" ? "↑" : "↓" : "↕";
    });
  }

  function renderInventory() {
    renderTabs("inventory-types");
    $("inventory-variants").setAttribute("aria-pressed", String(state.variantOnly));
    $("inventory-variants").textContent = `Shared names (${num(state.sharedNameIDs.size)})`;
    const records = sorted(filtered());
    const pages = Math.max(1, Math.ceil(records.length / state.pageSize));
    state.page = Math.min(state.page, pages);
    const from = (state.page - 1) * state.pageSize;
    renderRows("inventory-table", records.slice(from, from + state.pageSize), "inventory", records.length ? "No tools on this page" : "No matching tools. Try clearing your search or filters");
    updateToolSelectionUI();
    $("inventory-range").textContent = records.length ? `Showing ${from + 1}–${Math.min(from + state.pageSize, records.length)} of ${num(records.length)}` : "0 matching tools";
    $("inventory-page-size").value = String(state.pageSize);
    $("page-count").textContent = `Page ${state.page} of ${pages}`;
    $("prev-page").disabled = state.page <= 1;
    $("next-page").disabled = state.page >= pages;
    updateSortHeaders();
  }

  function renderUsage() {
    const totals = visible().reduce((sum, item) => sum + item.verified_calls, 0);
    $("usage-total").textContent = num(totals);
    renderBars("usage-bars");
    const tiers = ["hot", "warm", "cold", "dormant"];
    $("tier-breakdown").replaceChildren(...tiers.map((tier) => {
      const row = el("div", "tier-row");
      row.append(tierPill(tier), el("span", "tier-count", num(state.items.filter((item) => item.tier === tier).length)));
      return row;
    }));
    const top = topItems().slice(0, 10);
    const rows = top.map((item, index) => {
      const row = el("div", "ranked-row");
      row.append(el("span", "rank-index", String(index + 1).padStart(2, "0")), toolSymbol(item), el("span", "tool-name", item.name), el("span", "mono-number", num(item.verified_calls)), detailButton(item));
      return row;
    });
    $("top-tools").replaceChildren(...(rows.length ? rows : [el("p", "empty-message", "No recorded calls to rank yet")]));
  }

  function renderCleanup() {
    const records = state.sort === "type" ? sorted(cleanupRecords()) : cleanupRecords();
    $("cleanup-count").textContent = state.cleanupFilter === "all" ? `${num(records.length)} installed tools · inspect removal before confirming` : state.cleanupFilter === "variants" ? `${num(records.length)} same-name installations · contents may differ, compare sources before removing` : `${num(records.length)} tools without recorded calls and no shared-name attribution ambiguity`;
    document.querySelectorAll("[data-cleanup-filter]").forEach((button) => button.setAttribute("aria-pressed", String(button.dataset.cleanupFilter === state.cleanupFilter)));
    const pages = Math.max(1, Math.ceil(records.length / state.pageSize));
    state.cleanupPage = Math.min(state.cleanupPage, pages);
    const from = (state.cleanupPage - 1) * state.pageSize;
    renderRows("cleanup-table", records.slice(from, from + state.pageSize), "cleanup", "No matching tools found");
    updateSortHeaders();
    updateToolSelectionUI();
    $("cleanup-range").textContent = records.length ? `Showing ${from + 1}–${Math.min(from + state.pageSize, records.length)} of ${num(records.length)}` : "0 candidates";
    $("cleanup-page-size").value = String(state.pageSize);
    $("cleanup-pages").textContent = `Page ${state.cleanupPage} of ${pages}`;
    $("cleanup-prev").disabled = state.cleanupPage <= 1;
    $("cleanup-next").disabled = state.cleanupPage >= pages;
  }

  function renderSources() {
    const coverage = state.pool?.coverage || {};
    const checked = Array.isArray(coverage.scanned) ? coverage.scanned.length : 0;
    const skipped = Array.isArray(coverage.skipped) ? coverage.skipped.length : 0;
    const warnings = Array.isArray(coverage.warnings) ? coverage.warnings : [];
    const duplicateVariants = state.items.filter((item) => item.duplicate_of).length;
    $("scan-coverage").textContent = `${num(checked)} locations checked · ${num(skipped)} unavailable or skipped · ${num(coverage.project_directories || 0)} project directories · ${num(duplicateVariants)} extra variants among ${num(state.sharedNameIDs.size)} shared-name installations`;
    const warningPanel = $("coverage-warnings");
    warningPanel.hidden = warnings.length === 0;
    if (warnings.length) {
      const details = el("div");
      details.append(el("strong", "", `${warnings.length} scan warnings`));
      warnings.forEach((warning) => details.append(el("p", "", String(warning))));
      warningPanel.replaceChildren(icon("info"), details);
    } else warningPanel.replaceChildren();
    const sources = new Map();
    state.items.forEach((item) => {
      const path = item.path;
      if (!path) return;
      const source = item.type === "mcp" && path.endsWith(".json") ? path : path.slice(0, path.lastIndexOf("/")) || path;
      const existing = sources.get(source) || { path: source, count: 0, types: new Set() };
      existing.count += 1; existing.types.add(item.type); sources.set(source, existing);
    });
    const fragment = document.createDocumentFragment();
    if (!sources.size) fragment.append(el("p", "empty-message", "No source locations found"));
    [...sources.values()].sort((a, b) => b.count - a.count).forEach((source) => {
      const row = el("div", "source-row");
      const symbol = el("span", "tool-symbol"); symbol.append(icon("folder"));
      const middle = el("div");
      middle.append(el("div", "source-title", source.path), el("div", "source-meta", [...source.types].map((type) => TYPE_LABELS[type]).join(" · ")));
      row.append(symbol, middle, el("span", "source-count", `${num(source.count)} tools`));
      fragment.append(row);
    });
    $("sources-list").replaceChildren(fragment);
  }

  function render() {
    document.querySelectorAll(".view").forEach((panel) => { panel.hidden = panel.dataset.panel !== state.view; });
    document.querySelectorAll("[data-view].nav-item").forEach((button) => {
      const active = button.dataset.view === state.view;
      button.classList.toggle("active", active);
      if (active) button.setAttribute("aria-current", "page"); else button.removeAttribute("aria-current");
    });
    $("page-title").textContent = VIEW_COPY[state.view][0];
    $("page-subtitle").textContent = VIEW_COPY[state.view][1];
    document.title = `${VIEW_COPY[state.view][0]} | Agentic Skills Healer`;
    $("review-button").hidden = state.view === "cleanup";
    $("overview-search").value = state.query;
    $("inventory-search").value = state.query;
    document.querySelectorAll(".search-clear").forEach((button) => { button.hidden = !state.query; });
    $("nav-cleanup-count").textContent = num(cleanupCandidates().length);
    const views = {
      overview: renderOverview, inventory: renderInventory, usage: renderUsage,
      cleanup: renderCleanup, sources: renderSources, activity: () => renderActivity("activity-list", 100),
      hot: renderHot, quick: renderQuick, collections: renderCollections, archived: renderArchived,
      settings: () => {},
    };
    views[state.view]();
  }

  function readURL() {
    const params = new URLSearchParams(location.search);
    let savedSize = 25;
    try { savedSize = Number(window.localStorage.getItem("ash:rows-per-page")); } catch { /* Private storage may be unavailable */ }
    const requestedSize = Number(params.get("pageSize")) || savedSize;
    state.pageSize = PAGE_SIZES.includes(requestedSize) ? requestedSize : 25;
    const view = params.get("view");
    state.view = Object.hasOwn(VIEW_COPY, view) ? view : "overview";
    state.type = TYPES.includes(params.get("type")) ? params.get("type") : "all";
    state.variantOnly = params.get("variants") === "1";
    state.query = (params.get("q") || "").slice(0, 300);
    state.sort = ["name", "type", "call_count", "last_seen"].includes(params.get("sort")) ? params.get("sort") : "call_count";
    state.order = params.get("order") === "asc" ? "asc" : "desc";
    state.page = Math.max(1, Number.parseInt(params.get("page") || "1", 10) || 1);
    state.cleanupPage = Math.max(1, Number.parseInt(params.get("cleanupPage") || "1", 10) || 1);
  }

  function syncURL(push = false) {
    const url = new URL(location.href);
    const values = { view: state.view, type: state.type, q: state.query, variants: state.variantOnly ? "1" : "", sort: state.sort, order: state.order, page: state.page, cleanupPage: state.cleanupPage, pageSize: state.pageSize };
    Object.entries(values).forEach(([key, value]) => { if (!value || value === "all" || value === "overview" || value === 1 || value === "") url.searchParams.delete(key); else url.searchParams.set(key, String(value)); });
    history[push ? "pushState" : "replaceState"]({}, "", url);
  }

  function goTo(view) {
    if (!Object.hasOwn(VIEW_COPY, view)) return;
    state.view = view;
    state.page = 1;
    syncURL(true);
    render();
    window.scrollTo({ top: 0, behavior: "instant" });
    $("page-title").setAttribute("tabindex", "-1");
    $("page-title").focus({ preventScroll: true });
  }

  function setType(type) { if (!TYPES.includes(type)) return; state.type = type; state.page = 1; syncURL(); render(); }
  function setSearch(query) { state.query = query; state.page = 1; syncURL(); render(); }
  function setPageSize(value) {
    const size = Number(value);
    if (!PAGE_SIZES.includes(size)) return;
    state.pageSize = size;
    state.page = 1;
    state.cleanupPage = 1;
    try { window.localStorage.setItem("ash:rows-per-page", String(size)); } catch { /* URL still retains the choice */ }
    syncURL();
    render();
  }

  async function scan() {
    if (state.request) state.request.abort();
    const controller = new AbortController();
    state.request = controller; state.loading = true;
    $("scan-button").disabled = true; $("scan-button").setAttribute("aria-busy", "true");
    $("scan-button").querySelector("span").textContent = "Scanning…";
    $("scan-status").textContent = "Scanning local inventory…";
    try {
      const response = await fetch("/api/pool", { cache: "no-store", signal: controller.signal });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      state.session = response.headers.get("X-ASH-Session") || "";
      const pool = await response.json();
      if (!pool || typeof pool !== "object" || !["skills", "mcps", "clis", "agents"].every((key) => pool[key] && typeof pool[key] === "object")) throw new Error("Invalid inventory payload");
      state.pool = pool;
      state.items = normalizePool(pool);
      const nameGroups = new Map();
      for (const item of state.items) {
        const key = `${item.type}\u0000${item.name}`;
        if (!nameGroups.has(key)) nameGroups.set(key, []);
        nameGroups.get(key).push(item.id);
      }
      state.sharedNameIDs = new Set([...nameGroups.values()].filter((ids) => ids.length > 1).flat());
      state.selectedTools = new Set([...state.selectedTools].filter((id) => state.items.some((item) => item.id === id)));
      state.lastScan = safeDate(pool.scanned_at) || new Date();
      try { await refreshSaved(); } catch { toast("Tool preferences could not be loaded", true); }
      state.selectedTrash = new Set([...state.selectedTrash].filter((id) => state.saved.trash[id]));
      $("error-banner").hidden = true;
      $("connection-dot").classList.add("green");
      $("connection-state").textContent = "Connected";
      $("scan-status").textContent = `Last scan ${state.lastScan.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" })} · ${num(state.items.length)} tools discovered`;
      render();
    } catch (error) {
      if (error.name === "AbortError") return;
      $("error-banner").hidden = false;
      $("connection-dot").classList.remove("green");
      $("connection-state").textContent = "Unavailable";
      $("scan-status").textContent = state.pool ? "Last successful snapshot retained" : "Local inventory unavailable";
      if (!state.pool) {
        state.items = [];
        render();
      }
    } finally {
      if (state.request === controller) {
        state.request = null; state.loading = false;
        $("scan-button").disabled = false; $("scan-button").removeAttribute("aria-busy");
        $("scan-button").querySelector("span").textContent = "Scan now";
      }
    }
  }

  function showDetails(id) {
    const item = state.items.find((record) => record.id === id);
    if (!item) return;
    state.selected = item;
    $("detail-title").textContent = displayName(item);
    const lines = [
      ["Type", TYPE_LABELS[item.type]], ["Domain", pref(item).domain || item.domain], ["Classifier suggestion", item.domain], ["Scope", item.scope],
      ["Verified calls", num(item.verified_calls)], ["Historical/reported calls", num(item.call_count)], ["Last reported", observedAgo(item.last_seen)],
      ["Discovery source", item.discovery_source || "Unknown"], ["Usage attribution", item.usage_attribution === "ambiguous_name" ? "Ambiguous: multiple installations share this name" : item.usage_attribution || "No host attribution"],
      ["Classification", item.classification_basis ? `${item.classification_basis} · rule score ${Math.round(item.classification_confidence * 100)}%` : "Unclassified"],
      ["Same-name variant", item.duplicate_of || "None"],
      ["Activity tier", item.tier.toUpperCase()], ["Status", item.status],
      ["Source path", item.path || "Not available"],
    ];
    const siblings = state.items.filter((other) => other.id !== item.id && other.type === item.type && other.name === item.name);
    if (siblings.length) lines.push([`Other installations (${siblings.length})`, siblings.map((other) => `${other.scope}: ${other.path}`).join(" | ")]);
    if (item.is_symlink) lines.push(["Symlink target", item.symlink_target || "Target unavailable"]);
    const fragment = document.createDocumentFragment();
    lines.forEach(([name, value]) => {
      const row = el("div", "detail-row");
      row.append(el("span", "detail-key", name), el("span", "detail-val", value));
      fragment.append(row);
    });
    $("detail-content").replaceChildren(fragment);
    $("copy-path").disabled = !item.path;
    $("reveal-tool").disabled = !item.path;
    $("preview-tool").hidden = item.type !== "skill";
    $("standby-tool").hidden = !standbyEligible(item);
    $("remove-tool").dataset.cleanup = item.id;
    $("tool-alias").value = pref(item).alias || "";
    $("tool-domain").value = pref(item).domain || "";
    $("tool-notes").value = pref(item).notes || "";
    $("tool-tags").value = (pref(item).tags || []).join(", ");
    $("tool-favorite").checked = Boolean(pref(item).favorite);
    $("tool-hidden").checked = Boolean(pref(item).hidden);
    $("detail-dialog").showModal();
  }

  async function openCleanup(id) {
    const item = state.items.find((record) => record.id === id);
    if (!item) return;
    state.cleanupPlan = null;
    $("cleanup-dialog-title").textContent = `Remove ${item.name}`;
    $("cleanup-plan-status").textContent = "Verifying the source and removal method…";
    $("cleanup-plan-name").textContent = item.name;
    $("cleanup-plan-path").textContent = item.path || "Unknown";
    $("cleanup-plan-operation").textContent = "Checking…";
    $("cleanup-plan-warning").hidden = true;
    $("cleanup-confirm-label").hidden = true;
    $("cleanup-confirm-input").value = "";
    $("cleanup-execute").disabled = true;
    $("cleanup-dialog").showModal();
    try {
      const response = await fetch(`/api/cleanup/plan?id=${encodeURIComponent(id)}`, { cache: "no-store" });
      if (!response.ok) throw new Error((await response.text()).trim() || `Cannot inspect source (${response.status})`);
      const plan = await response.json();
      if (!$('cleanup-dialog').open) return;
      state.cleanupPlan = plan;
      $("cleanup-plan-name").textContent = plan.name;
      $("cleanup-plan-path").textContent = plan.source_path || "Unavailable";
      const operations = { "quarantine-path": "Move actual file or folder to private Trash", "remove-mcp-entry": "Remove exact MCP server entry from its JSON config" };
      $("cleanup-plan-operation").textContent = plan.allowed ? operations[plan.operation] || plan.operation : "Unavailable for this source";
      $("cleanup-plan-status").textContent = plan.allowed ? "This will change the installed source on your Mac" : "Removal is blocked for this installation method";
      $("cleanup-plan-warning").textContent = plan.allowed ? plan.warning : plan.reason || "This source cannot be safely removed";
      $("cleanup-plan-warning").hidden = false;
      $("cleanup-confirm-label").hidden = !plan.allowed;
      if (plan.allowed) { $("cleanup-confirm-input").placeholder = plan.name; $("cleanup-confirm-input").focus(); }
    } catch (error) {
      if (!$('cleanup-dialog').open) return;
      $("cleanup-plan-status").textContent = "Cannot verify this installation";
      $("cleanup-plan-operation").textContent = "Blocked";
      $("cleanup-plan-warning").textContent = error.message;
      $("cleanup-plan-warning").hidden = false;
    }
  }

  async function executeCleanup() {
    const plan = state.cleanupPlan;
    if (!plan?.allowed || $("cleanup-confirm-input").value !== plan.name) return;
    const button = $("cleanup-execute"); button.disabled = true;
    const ok = await mutation("/api/cleanup/remove", "POST", { id: plan.id, fingerprint: plan.fingerprint, confirmation: plan.name }, "Source removed from Mac. Restore it from Trash if needed");
    if (ok) {
      $("cleanup-dialog").close();
      if ($('detail-dialog').open) $('detail-dialog').close();
      await scan();
    } else button.disabled = false;
  }

  function openPurge(id) {
    const record = state.saved.trash[id];
    if (!record) return;
    state.purgeRecord = record;
    const required = `DELETE ${record.name}`;
    $("purge-title").textContent = `Delete ${record.name} forever`;
    $("purge-description").textContent = `This permanently deletes the saved source and recovery copy for “${record.name}” from the local Trash vault. You cannot restore it afterwards`;
    $("purge-required").textContent = required;
    $("purge-confirm-input").value = "";
    $("purge-confirm-input").placeholder = required;
    $("purge-execute").disabled = true;
    $("purge-dialog").showModal();
    $("purge-confirm-input").focus();
  }

  async function executePurge() {
    const record = state.purgeRecord;
    if (!record || $("purge-confirm-input").value !== `DELETE ${record.name}`) return;
    const button = $("purge-execute"); button.disabled = true;
    const ok = await mutation("/api/cleanup/purge", "POST", { id: record.id, confirmation: `DELETE ${record.name}` }, "Recovery copy deleted permanently");
    if (ok) { $("purge-dialog").close(); await scan(); }
    else button.disabled = false;
  }

  async function restoreDeleted(id) {
    if (await mutation("/api/cleanup/restore", "POST", { id }, "Tool restored to its original source")) await scan();
  }

  async function batchRequest(path, body) {
    if (!state.session) throw new Error("A local session is required. Scan inventory first");
    const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json", "X-ASH-Session": state.session }, body: JSON.stringify(body) });
    if (!response.ok) throw new Error((await response.text()).trim().slice(0, 300) || `Action failed (${response.status})`);
    return response.json();
  }

  function bulkConfirmationPhrase(count, mode) {
    if (mode === "purge") return count === 1 ? "DELETE 1 RECOVERY COPY" : `DELETE ${count} RECOVERY COPIES`;
    return count === 1 ? "REMOVE 1 TOOL" : `REMOVE ${count} TOOLS`;
  }

  function bulkPlanCard(title, details, status, allowed) {
    const row = el("div", "bulk-plan-item");
    row.setAttribute("role", "listitem");
    const header = el("div", "bulk-plan-head");
    header.append(el("strong", "", title), el("span", allowed ? "pill pill-green" : "pill pill-orange", status));
    row.append(header, el("span", "bulk-plan-detail", details));
    return row;
  }

  function resetBulkDialog(mode, ids) {
    state.bulkMode = mode; state.bulkIDs = ids; state.bulkPlans = [];
    $("bulk-title").textContent = mode === "purge" ? `Permanently delete ${ids.length} recovery copies` : `Remove ${ids.length} installed tools`;
    $("bulk-status").textContent = "Verifying selected sources…";
    $("bulk-warning").textContent = "Changes are applied per source. If a later item fails, earlier successful removals remain in Trash";
    $("bulk-plan-list").replaceChildren();
    $("bulk-confirm-label").hidden = true;
    $("bulk-confirm-input").value = "";
    $("bulk-required").hidden = true;
    $("bulk-execute").disabled = true;
    $("bulk-execute").textContent = mode === "purge" ? "Delete recovery copies" : "Remove selected tools";
    $("bulk-dialog").showModal();
  }

  function enableBulkConfirmation(mode) {
    const required = bulkConfirmationPhrase(state.bulkIDs.length, mode);
    $("bulk-required").textContent = required;
    $("bulk-required").hidden = false;
    $("bulk-confirm-label").hidden = false;
    $("bulk-confirm-input").placeholder = required;
    $("bulk-confirm-input").focus();
  }

  async function openBulkRemoval() {
    const ids = [...state.selectedTools];
    if (!ids.length) { toast("Select tools to remove", true); return; }
    resetBulkDialog("remove", ids);
    try {
      const plans = await batchRequest("/api/cleanup/bulk-plan", { ids });
      if (!$('bulk-dialog').open || state.bulkMode !== "remove") return;
      state.bulkPlans = plans;
      $("bulk-plan-list").replaceChildren(...plans.map((plan) => bulkPlanCard(plan.name || plan.id, `${plan.source_path || "No installed source"} · ${plan.allowed ? plan.operation : plan.reason}`, plan.allowed ? "Supported" : "Blocked", plan.allowed)));
      const blocked = plans.filter((plan) => !plan.allowed);
      $("bulk-status").textContent = blocked.length ? `${blocked.length} sources cannot be removed by this adapter` : `${plans.length} sources ready for real removal`;
      $("bulk-warning").textContent = blocked.length ? "Clear the selection and deselect unsupported tools. No files have been changed" : "Removing these tools changes their actual installation. You can restore them from Trash until their recovery copies are permanently deleted";
      if (!blocked.length) enableBulkConfirmation("remove");
    } catch (error) {
      $("bulk-status").textContent = `Preview failed: ${error.message}`;
      toast(error.message, true);
    }
  }

  function openBulkPurge() {
    const ids = [...state.selectedTrash];
    if (!ids.length) { toast("Select recovery copies to delete", true); return; }
    resetBulkDialog("purge", ids);
    $("bulk-plan-list").replaceChildren(...ids.map((id) => {
      const record = state.saved.trash[id];
      return bulkPlanCard(record?.name || id, record?.source_path || "Unavailable", "Permanent", false);
    }));
    $("bulk-status").textContent = "Permanent deletion of the stored recovery copies";
    $("bulk-warning").textContent = "The selected sources are already removed from their active locations. This permanently deletes their recovery copies and cannot be undone";
    enableBulkConfirmation("purge");
  }

  function showBatchResults(mode, result) {
    $("bulk-result-title").textContent = mode === "remove" ? "Removal results" : mode === "purge" ? "Permanent deletion results" : "Restore results";
    $("bulk-result-summary").textContent = `${result.completed} completed · ${result.failed} failed`;
    $("bulk-result-list").replaceChildren(...(result.results || []).map((entry) => bulkPlanCard(entry.name || entry.id, entry.error || (entry.status === "removed" ? "Moved to private Trash" : entry.status), entry.status, entry.status !== "failed")));
    $("bulk-go-trash").hidden = mode === "purge";
    $("bulk-result-dialog").showModal();
  }

  async function executeBulk() {
    const mode = state.bulkMode;
    const ids = [...state.bulkIDs];
    if (!ids.length || $("bulk-confirm-input").value !== bulkConfirmationPhrase(ids.length, mode)) return;
    const button = $("bulk-execute"); button.disabled = true;
    button.textContent = mode === "purge" ? `Deleting ${num(ids.length)} recovery copies…` : `Removing ${num(ids.length)} tools…`;
    try {
      const result = mode === "purge"
        ? await batchRequest("/api/cleanup/bulk-purge", { ids, confirmation: bulkConfirmationPhrase(ids.length, mode) })
        : await batchRequest("/api/cleanup/bulk-remove", { items: state.bulkPlans.map((plan) => ({ id: plan.id, fingerprint: plan.fingerprint })), confirmation: bulkConfirmationPhrase(ids.length, mode) });
      $("bulk-dialog").close();
      const successes = (result.results || []).filter((entry) => entry.status !== "failed");
      if (mode === "purge") successes.forEach((entry) => state.selectedTrash.delete(entry.id));
      else successes.forEach((entry) => state.selectedTools.delete(entry.id));
      await scan();
      showBatchResults(mode, result);
    } catch (error) {
      toast(error.message, true);
      button.disabled = false;
      button.textContent = mode === "purge" ? "Delete recovery copies" : "Remove selected tools";
    }
  }

  async function restoreSelectedTrash() {
    const ids = [...state.selectedTrash];
    if (!ids.length) return;
    $("trash-bulk-restore").disabled = true;
    try {
      const result = await batchRequest("/api/cleanup/bulk-restore", { ids });
      (result.results || []).filter((entry) => entry.status !== "failed").forEach((entry) => state.selectedTrash.delete(entry.id));
      await scan();
      showBatchResults("restore", result);
    } catch (error) { toast(error.message, true); updateTrashSelectionUI(); }
  }

  async function bulkMetadata(action) {
    const ids = [...state.selectedTools];
    if (!ids.length) return;
    try {
      const result = await batchRequest("/api/tools/bulk", { ids, action });
      state.selectedTools.clear();
      await refreshSaved();
      render();
      toast(`${num(result.updated)} tools updated: ${action}`);
    } catch (error) { toast(error.message, true); }
  }

  async function copyPath() {
    if (!state.selected?.path) return;
    await copyText(state.selected.path, "Source path copied");
  }

  function askConfirmation(title, description, action) {
    state.confirming = action;
    $("confirm-title").textContent = title;
    $("confirm-description").textContent = description;
    $("confirm-dialog").showModal();
  }

  function metadataFor(item) {
    return { id: item.id, alias: pref(item).alias || "", notes: pref(item).notes || "", tags: pref(item).tags || [], favorite: Boolean(pref(item).favorite), hidden: Boolean(pref(item).hidden) };
  }

  async function saveTool(event) {
    event.preventDefault();
    if (!state.selected) return;
    const tags = $("tool-tags").value.split(",").map((tag) => tag.trim()).filter(Boolean);
    if (tags.length > 12 || tags.some((tag) => tag.length > 36 || tag.includes("/") || tag.includes("\\")) || new Set(tags.map((tag) => tag.toLowerCase())).size !== tags.length) {
      toast("Use up to 12 unique tags, separated by commas", true); return;
    }
    const item = state.selected;
    const preference = { id: item.id, alias: $("tool-alias").value, domain: $("tool-domain").value, notes: $("tool-notes").value, tags, favorite: $("tool-favorite").checked, hidden: $("tool-hidden").checked };
    const button = $("save-tool"); button.disabled = true;
    const okay = await mutation("/api/tool", "PATCH", preference, "Tool preferences saved");
    button.disabled = false;
    if (okay) $("detail-title").textContent = displayName(item);
  }

  async function pinTool(id) {
    const item = state.items.find((tool) => tool.id === id);
    if (!item) return;
    const data = metadataFor(item); data.favorite = !data.favorite;
    await mutation("/api/tool", "PATCH", data, data.favorite ? "Pinned to Quick access" : "Removed from favorites");
  }

  async function unhideTool(id) {
    const item = state.items.find((tool) => tool.id === id);
    if (!item) return;
    const data = metadataFor(item); data.hidden = false;
    await mutation("/api/tool", "PATCH", data, "Tool restored to inventory");
  }

  async function revealTool(id) {
    await mutation("/api/reveal", "POST", { id }, "Opened source in Finder");
  }

  async function previewTool(id) {
    $("preview-title").textContent = "Loading source…";
    $("preview-content").textContent = "Reading local Markdown…";
    $("preview-dialog").showModal();
    try {
      const response = await fetch(`/api/preview?id=${encodeURIComponent(id)}`, { cache: "no-store" });
      if (!response.ok) throw new Error("No readable SKILL.md or README.md found");
      const data = await response.json();
      $("preview-title").textContent = data.file;
      $("preview-content").textContent = data.content + (data.truncated ? "\n\n[Preview truncated at 32 KB]" : "");
    } catch (error) {
      $("preview-title").textContent = "Source unavailable";
      $("preview-content").textContent = error.message;
    }
  }

  function openShortcutEditor(id = "") {
    const shortcut = state.saved.shortcuts[id];
    $("shortcut-id").value = shortcut?.id || "";
    $("shortcut-label").value = shortcut?.label || "";
    $("shortcut-command").value = shortcut?.command || "";
    $("shortcut-title").textContent = shortcut ? "Edit command" : "Add command";
    $("shortcut-dialog").showModal();
    $("shortcut-label").focus();
  }

  function openPresetEditor(id = "") {
    const preset = state.saved.presets[id];
    state.draftPresetIDs = new Set(preset?.tool_ids || []);
    $("preset-id").value = preset?.id || "";
    $("preset-name").value = preset?.name || "";
    $("preset-search").value = "";
    $("preset-title").textContent = preset ? "Edit collection" : "New collection";
    renderPresetPicker();
    $("preset-dialog").showModal();
    $("preset-name").focus();
  }

  function renderPresetPicker() {
    const query = $("preset-search").value.trim().toLowerCase();
    const matches = state.items.filter((item) => !query || [item.name, displayName(item), item.domain, item.type].some((value) => value.toLowerCase().includes(query))).slice(0, 90);
    $("preset-selected").textContent = `${state.draftPresetIDs.size} tools selected`;
    if (!matches.length) showEmpty($("preset-picker"), "No matching tools", "Try a different search term");
    else $("preset-picker").replaceChildren(...matches.map((item) => {
      const label = el("label", "picker-row");
      const input = el("input"); input.type = "checkbox"; input.checked = state.draftPresetIDs.has(item.id); input.dataset.presetSelect = item.id;
      label.append(input, toolSymbol(item), el("span", "picker-name", displayName(item)), typePill(item.type));
      return label;
    }));
  }

  async function saveShortcut(event) {
    event.preventDefault();
    const id = $("shortcut-id").value;
    const data = { id, label: $("shortcut-label").value.trim(), command: $("shortcut-command").value.trim() };
    if (await mutation("/api/shortcut", id ? "PUT" : "POST", data, "Command saved")) $("shortcut-dialog").close();
  }

  async function savePreset(event) {
    event.preventDefault();
    const id = $("preset-id").value;
    const data = { id, name: $("preset-name").value.trim(), tool_ids: [...state.draftPresetIDs] };
    if (await mutation("/api/preset", id ? "PUT" : "POST", data, "Collection saved")) $("preset-dialog").close();
  }

  function openPalette() {
    $("palette-input").value = "";
    renderPalette();
    $("palette-dialog").showModal();
    $("palette-input").focus();
  }

  function renderPalette() {
    const query = $("palette-input").value.trim().toLowerCase();
    const results = $("palette-results");
    const items = [...state.items].filter((item) => !query || [item.name, displayName(item), item.domain, item.path].some((field) => field.toLowerCase().includes(query)));
    items.sort((a, b) => Number(Boolean(pref(b).favorite)) - Number(Boolean(pref(a).favorite)) || b.call_count - a.call_count);
    const rows = [];
    const pages = Object.entries(VIEW_COPY).filter(([key, copy]) => !query || [key, ...copy].some((field) => field.toLowerCase().includes(query))).slice(0, 5);
    pages.forEach(([key, copy]) => {
      const button = el("button", "palette-result"); button.type = "button"; button.dataset.paletteView = key;
      button.append(icon("arrow"), el("span", "", copy[0]), el("small", "", "Page")); rows.push(button);
    });
    sortedValues(state.saved.shortcuts).filter((shortcut) => !query || `${shortcut.label} ${shortcut.command}`.toLowerCase().includes(query)).slice(0, 4).forEach((shortcut) => {
      const button = el("button", "palette-result"); button.type = "button"; button.dataset.copycommand = shortcut.id;
      button.append(icon("terminal"), el("span", "", shortcut.label), el("small", "", "Copy command")); rows.push(button);
    });
    items.slice(0, 12).forEach((item) => {
      const button = el("button", "palette-result"); button.type = "button"; button.dataset.paletteTool = item.id;
      button.append(icon(TYPE_ICONS[item.type]), el("span", "", displayName(item)), el("small", "", TYPE_LABELS[item.type])); rows.push(button);
    });
    if (!rows.length) showEmpty(results, "No results", "Search tools, pages or commands");
    else results.replaceChildren(...rows);
  }

  function download(name, text, mime) {
    const url = URL.createObjectURL(new Blob([text], { type: mime }));
    const anchor = el("a"); anchor.href = url; anchor.download = name;
    document.body.append(anchor); anchor.click(); anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    toast("Download prepared locally");
  }

  function exportJSON() {
    const payload = { exported_at: new Date().toISOString(), inventory: state.items, preferences: state.saved, note: "Recorded usage only; absence of calls does not prove non-use" };
    download("ash-workspace-export.json", JSON.stringify(payload, null, 2), "application/json");
  }

  function exportCSV() {
    const escapeCSV = (value) => {
      let cell = String(value ?? "");
      if (/^[\t\r\n\s]*[=+\-@]/.test(cell)) cell = `'${cell}`;
      return `"${cell.replaceAll('"', '""')}"`;
    };
    const rows = [["ID", "Name", "Alias", "Type", "Domain", "Scope", "Recorded calls", "Last observed", "Path", "Tags", "Favorite", "Hidden"]];
    state.items.forEach((item) => rows.push([item.id, item.name, pref(item).alias || "", item.type, item.domain, item.scope, item.call_count, item.last_seen || "", item.path, (pref(item).tags || []).join("; "), Boolean(pref(item).favorite), Boolean(pref(item).hidden)]));
    download("ash-inventory.csv", rows.map((row) => row.map(escapeCSV).join(",")).join("\r\n"), "text/csv;charset=utf-8");
  }

  function setPage(key, direction) { state[key] = Math.max(1, state[key] + direction); syncURL(); render(); }

  document.addEventListener("click", (event) => {
    const target = event.target.closest("button");
    if (!target) return;
    if (target.dataset.closeDialog) $(target.dataset.closeDialog).close();
    else if (target.dataset.bulkRemove !== undefined) void openBulkRemoval();
    else if (target.dataset.bulkMeta) void bulkMetadata(target.dataset.bulkMeta);
    else if (target.dataset.clearSelection) {
      if (target.dataset.clearSelection === "trash") { state.selectedTrash.clear(); renderArchived(); }
      else { state.selectedTools.clear(); render(); }
    }
    else if (target.dataset.cleanup) void openCleanup(target.dataset.cleanup);
    else if (target.dataset.cleanupFilter) { state.cleanupFilter = target.dataset.cleanupFilter; state.cleanupPage = 1; renderCleanup(); }
    else if (target.id === "inventory-variants") {
      state.variantOnly = !state.variantOnly;
      if (state.variantOnly) { state.sort = "name"; state.order = "asc"; }
      state.page = 1; syncURL(); renderInventory();
    }
    else if (target.dataset.cleanuprestore) void restoreDeleted(target.dataset.cleanuprestore);
    else if (target.dataset.cleanuppurge) openPurge(target.dataset.cleanuppurge);
    else if (target.dataset.paletteView) { $("palette-dialog").close(); goTo(target.dataset.paletteView); }
    else if (target.dataset.paletteTool) { $("palette-dialog").close(); showDetails(target.dataset.paletteTool); }
    else if (target.dataset.pin) void pinTool(target.dataset.pin);
    else if (target.dataset.copytool) { const item = state.items.find((item) => item.id === target.dataset.copytool); if (item) void copyText(item.path, "Source path copied"); }
    else if (target.dataset.reveal) void revealTool(target.dataset.reveal);
    else if (target.dataset.unhide) void unhideTool(target.dataset.unhide);
    else if (target.dataset.restore) {
      const record = state.saved.standby[target.dataset.restore];
      if (record) askConfirmation("Restore skill alias", `Restore ${record.name} to ${record.original_path}? Existing files will never be overwritten`, async () => { if (await mutation("/api/restore", "POST", { id: record.id }, "Alias restored")) await scan(); });
    }
    else if (target.dataset.copycommand) { const shortcut = state.saved.shortcuts[target.dataset.copycommand]; if (shortcut) void copyText(shortcut.command, "Command copied. Review before running"); if ($("palette-dialog").open) $("palette-dialog").close(); }
    else if (target.dataset.editcommand) openShortcutEditor(target.dataset.editcommand);
    else if (target.dataset.deletecommand) {
      const shortcut = state.saved.shortcuts[target.dataset.deletecommand];
      if (shortcut) askConfirmation("Remove saved command", `Remove “${shortcut.label}” from Quick access? This deletes only the saved command, not any installed tool`, () => mutation("/api/shortcut", "DELETE", { id: shortcut.id }, "Saved command removed"));
    }
    else if (target.dataset.editpreset) openPresetEditor(target.dataset.editpreset);
    else if (target.dataset.browsepreset) { state.selectedPreset = target.dataset.browsepreset; renderCollections(); $("preset-browser").scrollIntoView({ behavior: "smooth", block: "start" }); }
    else if (target.dataset.deletepreset) {
      const preset = state.saved.presets[target.dataset.deletepreset];
      if (preset) askConfirmation("Remove collection", `Delete the collection “${preset.name}”? All installed tools stay untouched`, async () => { if (await mutation("/api/preset", "DELETE", { id: preset.id }, "Collection removed")) { state.selectedPreset = null; render(); } });
    }
    else if (target.dataset.tag) { state.activeTag = state.activeTag.toLowerCase() === target.dataset.tag.toLowerCase() ? "" : target.dataset.tag; renderCollections(); }
    else if (target.dataset.view) goTo(target.dataset.view);
    else if (target.dataset.type) setType(target.dataset.type);
    else if (target.dataset.detail) showDetails(target.dataset.detail);
    else if (target.dataset.clear) { $(target.dataset.clear).value = ""; setSearch(""); $(target.dataset.clear).focus(); }
    else if (target.dataset.sort) {
      if (state.sort === target.dataset.sort) state.order = state.order === "asc" ? "desc" : "asc";
      else { state.sort = target.dataset.sort; state.order = ["name", "type"].includes(target.dataset.sort) ? "asc" : "desc"; }
      state.page = 1; state.cleanupPage = 1; syncURL(); render();
    }
  });

  document.addEventListener("change", (event) => {
    const target = event.target;
    if (target.dataset?.selectTool) {
      const ok = changeSelection(state.selectedTools, target.dataset.selectTool, target.checked);
      if (!ok) target.checked = false;
      updateToolSelectionUI();
    } else if (target.dataset?.selectTrash) {
      const ok = changeSelection(state.selectedTrash, target.dataset.selectTrash, target.checked);
      if (!ok) target.checked = false;
      updateTrashSelectionUI();
    }
  });

  ["overview-search", "inventory-search"].forEach((id) => {
    const input = $(id);
    let composing = false;
    input.addEventListener("compositionstart", () => { composing = true; });
    input.addEventListener("compositionend", () => { composing = false; setSearch(input.value); });
    input.addEventListener("input", () => { if (!composing) setSearch(input.value); });
  });
  $("scan-button").addEventListener("click", scan);
  $("retry-button").addEventListener("click", scan);
  $("settings-refresh").addEventListener("click", scan);
  $("review-button").addEventListener("click", () => goTo("cleanup"));
  $("prev-page").addEventListener("click", () => setPage("page", -1));
  $("next-page").addEventListener("click", () => setPage("page", 1));
  $("cleanup-prev").addEventListener("click", () => setPage("cleanupPage", -1));
  $("cleanup-next").addEventListener("click", () => setPage("cleanupPage", 1));
  $("inventory-page-size").addEventListener("change", (event) => setPageSize(event.target.value));
  $("cleanup-page-size").addEventListener("change", (event) => setPageSize(event.target.value));
  $("inventory-select-page").addEventListener("change", (event) => selectPage("inventory", event.target.checked));
  $("cleanup-select-page").addEventListener("change", (event) => selectPage("cleanup", event.target.checked));
  $("trash-select-all").addEventListener("change", (event) => selectTrashAll(event.target.checked));
  $("trash-bulk-restore").addEventListener("click", restoreSelectedTrash);
  $("trash-bulk-purge").addEventListener("click", openBulkPurge);
  $("bulk-confirm-input").addEventListener("input", () => {
    $("bulk-execute").disabled = $("bulk-confirm-input").value !== bulkConfirmationPhrase(state.bulkIDs.length, state.bulkMode) || (state.bulkMode === "remove" && state.bulkPlans.some((plan) => !plan.allowed));
  });
  $("bulk-execute").addEventListener("click", executeBulk);
  $("bulk-go-trash").addEventListener("click", () => { $("bulk-result-dialog").close(); goTo("archived"); });
  $("cleanup-confirm-input").addEventListener("input", () => { $("cleanup-execute").disabled = !state.cleanupPlan?.allowed || $("cleanup-confirm-input").value !== state.cleanupPlan.name; });
  $("cleanup-execute").addEventListener("click", executeCleanup);
  $("purge-confirm-input").addEventListener("input", () => { $("purge-execute").disabled = !state.purgeRecord || $("purge-confirm-input").value !== `DELETE ${state.purgeRecord.name}`; });
  $("purge-execute").addEventListener("click", executePurge);
  $("close-dialog").addEventListener("click", () => $("detail-dialog").close());
  $("done-dialog").addEventListener("click", () => $("detail-dialog").close());
  $("copy-path").addEventListener("click", copyPath);
  $("tool-form").addEventListener("submit", saveTool);
  $("reset-tool").addEventListener("click", () => {
    if (!state.selected) return;
    const item = state.selected;
    askConfirmation("Clear tool metadata", `Clear your alias, tags, notes, pin and hide status for “${displayName(item)}”? The installed tool will remain on disk`, async () => {
      if (await mutation("/api/tool", "DELETE", { id: item.id }, "Tool metadata cleared")) { $("detail-dialog").close(); }
    });
  });
  $("reveal-tool").addEventListener("click", () => { if (state.selected) void revealTool(state.selected.id); });
  $("preview-tool").addEventListener("click", () => { if (state.selected) void previewTool(state.selected.id); });
  $("standby-tool").addEventListener("click", () => {
    if (!state.selected || !standbyEligible(state.selected)) return;
    const item = state.selected;
    askConfirmation("Move alias to standby", `Move the Gemini skill symlink “${item.name}” out of its active folder? The original skill content stays in place and the alias can be restored from Archived`, async () => {
      if (await mutation("/api/standby", "POST", { id: item.id }, "Alias moved to standby")) { $("detail-dialog").close(); await scan(); }
    });
  });
  $("add-shortcut").addEventListener("click", () => openShortcutEditor());
  $("shortcut-form").addEventListener("submit", saveShortcut);
  $("add-preset").addEventListener("click", () => openPresetEditor());
  $("preset-form").addEventListener("submit", savePreset);
  $("preset-search").addEventListener("input", renderPresetPicker);
  $("close-preset-browser").addEventListener("click", () => { state.selectedPreset = null; renderCollections(); });
  $("preset-picker").addEventListener("change", (event) => {
    const id = event.target.dataset?.presetSelect;
    if (!id) return;
    if (event.target.checked) {
      if (state.draftPresetIDs.size >= 200) { event.target.checked = false; toast("Maximum 200 tools per collection", true); return; }
      state.draftPresetIDs.add(id);
    } else state.draftPresetIDs.delete(id);
    $("preset-selected").textContent = `${state.draftPresetIDs.size} tools selected`;
  });
  $("confirm-accept").addEventListener("click", () => {
    const action = state.confirming; state.confirming = null;
    $("confirm-dialog").close();
    if (action) void action();
  });
  $("palette-input").addEventListener("input", renderPalette);
  $("palette-input").addEventListener("keydown", (event) => {
    if (event.key !== "ArrowDown" && event.key !== "Enter") return;
    const first = $("palette-results").querySelector("button");
    if (first) { event.preventDefault(); first.focus(); if (event.key === "Enter") first.click(); }
  });
  $("palette-results").addEventListener("keydown", (event) => {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    const buttons = [...$("palette-results").querySelectorAll("button")];
    const current = buttons.indexOf(document.activeElement);
    const next = current + (event.key === "ArrowDown" ? 1 : -1);
    if (buttons[next]) { event.preventDefault(); buttons[next].focus(); }
    else if (event.key === "ArrowUp" && current === 0) { event.preventDefault(); $("palette-input").focus(); }
  });
  $("command-trigger").addEventListener("click", openPalette);
  document.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") { event.preventDefault(); if (!$('palette-dialog').open) openPalette(); }
  });
  $("export-json").addEventListener("click", exportJSON);
  $("export-csv").addEventListener("click", exportCSV);
  $("detail-dialog").addEventListener("click", (event) => { if (event.target === $("detail-dialog")) $("detail-dialog").close(); });
  window.addEventListener("popstate", () => { readURL(); render(); });
  $("host-label").textContent = location.host;
  $("settings-host").textContent = location.origin;
  readURL(); render(); scan();
})();
