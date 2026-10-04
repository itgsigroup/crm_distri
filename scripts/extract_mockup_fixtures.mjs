#!/usr/bin/env node
// Extracts the sample-data constants from reference/arc-crm-mockup.html into
// tests/fixtures/*.json. The constants are plain JS literals, so each one is
// sliced out of the <script> block and evaluated in an isolated vm context.
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const html = fs.readFileSync(path.join(root, 'reference/arc-crm-mockup.html'), 'utf8');
const script = html.slice(html.lastIndexOf('<script>') + 8, html.lastIndexOf('</script>'));

// Returns the literal source that starts right after `const NAME=`.
function literal(name) {
  const re = new RegExp(`(?:const|let) ${name}\\s*=\\s*`);
  const m = re.exec(script);
  if (!m) throw new Error(`constant ${name} not found`);
  let i = m.index + m[0].length;
  const open = script[i];
  const close = open === '[' ? ']' : open === '{' ? '}' : null;
  if (!close) throw new Error(`constant ${name} is not an array/object literal`);
  let depth = 0, str = null;
  for (let j = i; j < script.length; j++) {
    const c = script[j];
    if (str) {
      if (c === '\\') { j++; continue; }
      if (c === str) str = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') { str = c; continue; }
    if (c === open) depth++;
    else if (c === close) { depth--; if (depth === 0) return script.slice(i, j + 1); }
  }
  throw new Error(`unterminated literal for ${name}`);
}

// Object.assign(NAME,{...}) blocks extend some constants (ACTIONS, CANNED).
function assigned(name) {
  const out = [];
  const re = new RegExp(`Object\\.assign\\(${name},\\s*`, 'g');
  let m;
  while ((m = re.exec(script))) {
    const start = m.index + m[0].length;
    let depth = 0, str = null;
    for (let j = start; j < script.length; j++) {
      const c = script[j];
      if (str) { if (c === '\\') { j++; continue; } if (c === str) str = null; continue; }
      if (c === "'" || c === '"' || c === '`') { str = c; continue; }
      if (c === '{') depth++;
      else if (c === '}') { depth--; if (depth === 0) { out.push(script.slice(start, j + 1)); break; } }
    }
  }
  return out;
}

const evalLit = src => vm.runInNewContext(`(${src})`, {});

const names = ['DEALS', 'WON', 'LEADS', 'ACTIONS', 'L2C', 'INSTALLED', 'SALES', 'CONTACTS', 'MONTHS', 'EDGES_M',
  'INTERNAL', 'SUSPECT', 'CHATS', 'INBOUND', 'CANNED', 'CTX_Q', 'HIST', 'TITLES', 'OWNER_INI'];
const data = {};
for (const n of names) data[n] = evalLit(literal(n));
for (const n of ['ACTIONS', 'CANNED']) for (const src of assigned(n)) Object.assign(data[n], evalLit(src));

const outDir = path.join(root, 'tests/fixtures');
fs.mkdirSync(outDir, { recursive: true });
const files = {
  deals: data.DEALS, won: data.WON, leads: data.LEADS, actions: data.ACTIONS, l2c: data.L2C,
  installed: data.INSTALLED, sales: data.SALES, contacts: data.CONTACTS,
  edges_m: { months: data.MONTHS, edges: data.EDGES_M }, internal: data.INTERNAL, suspects: data.SUSPECT,
  chats: data.CHATS, inbound: data.INBOUND, ask_canned: data.CANNED, ask_suggestions: data.CTX_Q,
  wa_history: data.HIST, screens: data.TITLES, owner_initials: data.OWNER_INI,
};
for (const [file, value] of Object.entries(files)) {
  fs.writeFileSync(path.join(outDir, `${file}.json`), JSON.stringify(value, null, 2) + '\n');
}
console.log(JSON.stringify({
  deals: data.DEALS.length, edges: data.EDGES_M.length, chats: data.CHATS.length,
  inbound: data.INBOUND.length, actions: Object.keys(data.ACTIONS).length, out: path.relative(root, outDir),
}));
