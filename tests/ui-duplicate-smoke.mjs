// Local macOS smoke check: read-only duplicate grouping and guarded cleanup
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';

const port = process.argv[2] || '4317';
const chromePort = '9337';
const chrome = spawn(process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
  '--headless=new', '--no-first-run', '--disable-gpu', `--remote-debugging-port=${chromePort}`,
  '--user-data-dir=/tmp/ash-healer-duplicate-smoke', 'about:blank',
], { stdio: 'ignore' });
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
let ws;
let nextId = 0;
const pending = new Map();
async function command(method, params = {}) {
  const id = ++nextId;
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });
}
async function evaluate(expression) {
  const result = await command('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
  if (result.exceptionDetails) throw Error(result.exceptionDetails.text);
  return result.result.value;
}
try {
  let target;
  for (let attempt = 0; attempt < 90; attempt++) {
    try {
      const response = await fetch(`http://127.0.0.1:${chromePort}/json/list`);
      if (response.ok) target = (await response.json()).find((entry) => entry.type === 'page');
      if (target) break;
    } catch { /* Browser still starting */ }
    await pause(150);
  }
  assert.ok(target, 'Chrome debugging target');
  ws = new WebSocket(target.webSocketDebuggerUrl);
  ws.addEventListener('message', (event) => {
    const data = JSON.parse(event.data);
    if (!pending.has(data.id)) return;
    const { resolve, reject } = pending.get(data.id);
    pending.delete(data.id);
    data.error ? reject(new Error(data.error.message)) : resolve(data.result);
  });
  await new Promise((resolve, reject) => {
    ws.addEventListener('open', resolve, { once: true });
    ws.addEventListener('error', reject, { once: true });
  });
  await command('Page.enable');
  await command('Runtime.enable');
  await command('Page.navigate', { url: `http://127.0.0.1:${port}/` });
  let loaded = false;
  for (let n = 0; n < 120; n++) {
    loaded = await evaluate("Number(document.querySelector('#metric-total')?.textContent.replaceAll(',', '') || 0) > 0");
    if (loaded) break;
    await pause(125);
  }
  assert.ok(loaded, 'Inventory loaded before evaluating filters');
  await evaluate("document.querySelector('[data-view=inventory]').click()");
  await evaluate("document.querySelector('#inventory-variants').click()");
  const inventory = await evaluate(`(() => ({
    pressed: document.querySelector('#inventory-variants').getAttribute('aria-pressed'),
    range: document.querySelector('#inventory-range').textContent,
    rows: [...document.querySelectorAll('#inventory-table tr')].length,
    allShared: [...document.querySelectorAll('#inventory-table tr')].every((row) => row.textContent.includes('Shared name')),
    url: location.search,
  }))()`);
  console.log('Inventory variant filter:', inventory);
  assert.equal(inventory.pressed, 'true');
  assert.ok(inventory.rows > 0 && inventory.allShared, 'Every filtered row shares a name');
  assert.ok(inventory.url.includes('variants=1'), 'Filter persisted to URL');
  await evaluate("document.querySelector('#inventory-table [data-detail]').click()");
  const details = await evaluate(`(() => ({
    modal: document.querySelector('#detail-dialog').open,
    siblings: document.querySelector('#detail-content').textContent.includes('Other installations'),
    category: document.querySelector('#tool-domain option[value="Security, Compliance & Guardrails"]')?.value,
  }))()`);
  assert.ok(details.modal && details.siblings, 'Other installations listed in details');
  assert.equal(details.category, 'Security, Compliance & Guardrails');
  await evaluate("document.querySelector('#close-dialog').click()");
  await evaluate("document.querySelector('[data-view=cleanup]').click()");
  await evaluate("document.querySelector('[data-cleanup-filter=variants]').click()");
  const cleanup = await evaluate(`(() => ({
    pressed: document.querySelector('[data-cleanup-filter=variants]').getAttribute('aria-pressed'),
    caption: document.querySelector('#cleanup-count').textContent,
    rows: document.querySelectorAll('#cleanup-table [data-select-tool]').length,
  }))()`);
  assert.equal(cleanup.pressed, 'true');
  assert.ok(cleanup.rows > 0 && cleanup.caption.includes('same-name installations'));
  await evaluate("document.querySelector('#cleanup-select-page').click()");
  await evaluate("document.querySelector('#cleanup-bulk-toolbar [data-bulk-remove]').click()");
  for (let n = 0; n < 100; n++) {
    if (await evaluate("document.querySelector('#bulk-plan-list').children.length > 0")) break;
    await pause(125);
  }
  const guard = await evaluate(`(() => ({
    open: document.querySelector('#bulk-dialog').open,
    disabled: document.querySelector('#bulk-execute').disabled,
    reviewed: document.querySelector('#bulk-plan-list').children.length,
  }))()`);
  assert.ok(guard.open && guard.disabled && guard.reviewed === cleanup.rows, 'Removal requires explicit confirmation');
  await evaluate("document.querySelector('#bulk-dialog').close()");
  await evaluate("document.querySelector('[data-view=sources]').click()");
  const coverage = await evaluate(`(() => ({
    summary: document.querySelector('#scan-coverage').textContent,
    warningsVisible: !document.querySelector('#coverage-warnings').hidden,
  }))()`);
  assert.ok(coverage.summary.includes('locations checked') && coverage.summary.includes('shared-name installations'), 'Coverage and duplicate counts exposed');
  console.log(JSON.stringify({ inventory, details, cleanup, guard, coverage }, null, 2));
} finally {
  ws?.close();
  chrome.kill('SIGTERM');
}
