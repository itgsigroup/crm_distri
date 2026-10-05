// Extracts the sample data (fiktif) embedded in reference/distri-arc-orbit-v2-mockup.html into
// tools/seedgen/mockup.json, together with the values the mockup's own formulas produce.
// The Go seed generator (tools/seedgen) reads this file; the seed regression test compares against "expected".
// Usage: node tools/seedgen/extract_mockup.mjs
import fs from 'node:fs';
import vm from 'node:vm';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const html = fs.readFileSync(path.join(root, 'reference/distri-arc-orbit-v2-mockup.html'), 'utf8');
const scripts = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m => m[1]);
let src = scripts[scripts.length - 1];
src = src.slice(0, src.indexOf('/* ---------------- Boot'));
// const/let at top level are not visible on the context object; expose what we need.
src += `\n;globalThis.__out={SALES,KAT,DEALERS,ACTIONS,QUEUE,NEXT,AGENTS,STAGES,RUNS,CONFLICTS,MCP_LOG,AGING,AR,CHATS,EDGES_M,KUAD,
  plan:planItems(),
  expected:DEALERS.map(d=>({id:d.id,cyc:cyc(d),due_in:dueIn(d),activity:denyut(d),status:ring(d),credit:napas(d),score:gravitasi(d),segment:kuadran(d),segment_prev:kuadPrev(d),omzet:omzetBln(d),freq:freq(d)}))};`;
const stub = () => new Proxy(function () {}, { get: (t, k) => (k === Symbol.toPrimitive ? () => '' : stub()), apply: () => stub(), set: () => true });
const ctx = { document: stub(), window: {}, location: { hash: '' }, history: stub(), getComputedStyle: stub(), matchMedia: stub(), MutationObserver: function () { return stub(); }, requestAnimationFrame: () => 0, cancelAnimationFrame: () => 0, devicePixelRatio: 1, setTimeout, clearTimeout, console, Math, Date, JSON };
vm.createContext(ctx);
vm.runInContext(src, ctx);
const out = ctx.__out;
fs.writeFileSync(path.join(root, 'tools/seedgen/mockup.json'), JSON.stringify(out, null, 1));
console.log('dealers', out.DEALERS.length, 'chats', out.CHATS.length, 'actions', Object.keys(out.ACTIONS).length, 'edges', out.EDGES_M.length);
