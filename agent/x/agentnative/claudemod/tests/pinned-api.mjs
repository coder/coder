import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { runInNewContext } from 'node:vm';

/**
 * Load selected native queue, receipt, entry-gate, and turn-dispatch functions.
 * This never executes Claude, creates a session, or contacts a model. The hash
 * gates evaluation of exact 2.1.309 functions, not arbitrary binary text. Host
 * services and the plugin dispatcher are stubbed, but native settlement and
 * attribution run as-is. Set CLAUDE_CONTRACT_BINARY when running the mod tests.
 */
export async function loadPinnedAPI(path) {
  const binary = await readFile(path);
  assert.equal(createHash('sha256').update(binary).digest('hex'),
    '4d704a8aecbe038affa3000f498ae17c1b4f753add8facb8764120affc99b658',
    'contract extraction requires the pinned Linux Claude 2.1.309 binary');
  const source = binary.toString('utf8');
  function extract(start, end) {
    const from = source.indexOf(start);
    assert.ok(from >= 0, start);
    assert.equal(source.indexOf(start, from + 1), -1, 'ambiguous source boundary: ' + start);
    const to = source.indexOf(end, from + start.length);
    assert.ok(to > from, end);
    const code = source.slice(from, to);
    assert.ok(!code.includes('\0'), 'extraction crossed a bundled module');
    return code;
  }
  const queueCode = [
    extract('function p$t(){let e=new Map;', 'var Qfi='),
    extract('var GRo=(e,n)=>new Promise', 'var wpe='),
    extract('function nxo({text:e,attachments:n,asUser:r}', 'function jX('),
  ].join('\n');
  const receiptCode = [
    extract('function X2e(){let e=[];', 'var nGr='),
    extract('var Gbo=(e)=>({name:"core"', 'var Kbo='),
    extract('function Zfi(e,n=(r)=>Promise.resolve(r))', 'var WRo='),
    extract('async function qtn(e,n)', 'function Ytn('),
    extract('function Ytn(e){let n=Promise.withResolvers()', 'var xbt='),
    extract('function bae(e,n){switch(e.kind)', 'function SCr('),
    extract('function xPs({origin:e,bridgeOrigin:n', 'function wCr('),
    extract('function wNt(e,n,r){let s=OJ(n);', 'var B2n='),
    extract('var K=(e,o)=>o?{drop:o.resultText', 'import{randomUUID'),
    extract('function Y(e){let o=e.origin;', 'var q=(e)=>e.aborted;'),
    extract('var fe=({submission:e,attachments:o,origin:r', 'function ufi('),
    extract('function ufi(e){let o=!1;', 'function ce('),
    extract('function ce(e,o){let r=ta("prompt.submit");', 'function ge('),
    extract('async function p0n({submission:e,origin:o', 'export{f0n,'),
  ].join('\n');
  const turnCode = [
    extract('function nr(){let e=new Map;return{get:', 'function ys(e,s,r)'),
    extract('function eJr(e){let n=Promise.resolve()', 'function Uci(e,n)'),
  ].join('\n');
  const noop = () => {};
  return {
    async submit(input, onRoot = async () => {}) {
      const queued = [];
      const listeners = new Map();
      const channel = name => ({ subscribe: fn => listeners.set(name, fn) });
      const queue = runInNewContext(queueCode + '\n({ nxo, epi });', {
        Promise, Map, He: Error, Am: noop, qC: noop, t: noop, _: String,
        re: value => value, kpe: 100, $Ue: value => value.startsWith('/'),
        txo: randomUUID, Tpe: noop, vpe: noop, bP: () => [], et: () => undefined,
        BUe: 'next', _k: entry => queued.push(entry), ms: value => value,
        Kbo: { watching: false }, rWe: channel('discarded'), dsn: channel('consumed'),
      });
      const result = queue.nxo(input, 'workspace-bridge');
      if (!queued.length) await result;
      assert.equal(queued.length, 1);
      const entry = queued[0];
      const receipt = queue.epi(entry.uuid);
      const native = runInNewContext(receiptCode + '\n({ p0n, ufi });', {
        Promise, Map, Symbol, He: Error, rP: 'managed', t: noop,
        OJ: name => { assert.equal(name, 'workspace-bridge'); return name; },
        sJ: { get: () => Promise.resolve() }, Hfo: () => () => ({}),
        po: () => false, hwr: () => false, N: () => false, ta: () => true,
        yAr: 'prompt not submitted', re: 'prompt did not go through prompt.submit',
        lo: () => ({ pass: async () => ({}), commit: () => ({}), sessionTitle: noop }),
        aMo: () => [], qmi: (attachments, model, run) => run(attachments),
        XQn: value => value,
        Pft: ({ origin, core }) => ({ prompt: { submit: event => {
          // The original caller is excluded from reentrant hook dispatch. No
          // synthetic invocation of the mod's own prompt.submit hook occurs.
          assert.deepEqual(Array.from(origin), ['workspace-bridge']);
          return core['prompt.submit'].run(event);
        } } }),
      });
      let settled = false;
      result.then(() => { settled = true; });
      const turns = runInNewContext(turnCode + '\n({ dJr, or, Fci, tJr });', {
        Promise, Map, Set, Symbol, He: Error, Te: value => value,
        bs: randomUUID, t: noop, d: String, xpi: noop,
        my: content => { assert.equal(typeof content, 'string'); return content; },
        og: origin => origin?.kind === 'human',
        Wae: () => ({ turn: { start: event => {
          assert.equal(settled, false, 'native dispatch queues the root before settling the receipt');
          return onRoot(JSON.parse(JSON.stringify(event)));
        } } }),
      });
      const entries = turns.dJr();
      const turnEvents = turns.Fci(noop);
      const messages = [{ type: 'user', uuid: entry.uuid, origin: entry.origin,
        message: { content: entry.value } }];
      const signal = { aborted: false };
      const outcome = await native.p0n({
        submission: { text: entry.value }, origin: entry.origin,
        context: { abortController: { signal }, options: {} },
        result: { messages, shouldQuery: true }, queued: native.ufi(receipt),
        enter: () => turns.or(entries, messages),
      });
      assert.equal(settled, false, 'receipt must wait for the input to enter its turn');
      // Native input processing applies the attributed text to the user message.
      // tJr queues turn.start, then Bci resolves the session-entry gate used by
      // p0n's settleWhen. Neither receipt shape nor dispatch ordering is invented.
      messages[0].message.content = outcome.proceeds.text;
      turns.tJr({ turnEvents, newMessages: messages, signal, abort: noop });
      const accepted = await result;
      await turnEvents.settled();
      entries[Symbol.dispose]();
      return JSON.parse(JSON.stringify({ receipt: accepted, rootText: outcome.proceeds.text }));
    },
  };
}
