import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import test from 'node:test';

const source = await readFile(new URL('../hooks/register.js', import.meta.url), 'utf8');

function nativeSubmission(text, asUser = true) {
  return { text, origin: { kind: 'plugin', name: 'workspace-bridge', ...(asUser ? { asUser: true } : {}) } };
}

function nativeRootText(text, asUser = true) {
  // The pinned API preserves plugin provenance in receipts even with asUser.
  // Only asUser submissions reach the model without an attribution wrapper.
  if (asUser) return text;
  return 'The workspace-bridge plugin sent a message:\n' + text +
    '\n\nThis is how Claude Code surfaces a prompt a plugin submits between ' +
    'turns \u2014 it starts this turn in the user\'s place. Address the message above.';
}

async function fixture() {
  const { register } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64') + '#' + randomUUID());
  const hooks = new Map();
  register((name, filter, handler) => {
    if (typeof filter === 'function') hooks.set(name, filter);
    else hooks.set(name + ':' + filter.command, handler);
  });
  let tick;
  let descriptor = { url: 'http://127.0.0.1:32101', token: 'first-token' };
  let commands = [];
  let ack = true;
  let respond = async (_, reply) => reply;
  let deadline;
  const logs = [];
  const requests = [];
  const submissions = [];
  const accepted = [];
  const aborts = [];
  const api = {
    env: { get: async key => ({ WORKSPACE_BRIDGE_FILE: '/private/descriptor.json', WORKSPACE_BRIDGE_LAUNCH_ID: 'test-launch', WORKSPACE_BRIDGE_MODE: 'automation' })[key] },
    fs: { read: async () => JSON.stringify(descriptor) },
    session: { id: async () => 'native-uuid', version: async () => ({ version: '2.1.309' }) },
    command: { register: async () => {} },
    clock: {
      every: (ms, callback) => { assert.equal(ms, 200); tick = callback; return { cancel() { tick = undefined; } }; },
      after: (ms, callback) => { assert.equal(ms, 5000); deadline = callback; return { cancel() { deadline = undefined; } }; },
      sleep: async ms => { assert.equal(ms, 100); },
    },
    ui: { log: value => logs.push(value) },
    prompt: { read: async () => ({ text: '' }), submit: async input => {
      submissions.push(input);
      // Native suppresses the calling plugin's own prompt.submit hook. Its
      // submission result still carries the native, immutable plugin origin.
      const result = nativeSubmission(input.text, input.asUser === true);
      accepted.push(result);
      return result;
    } },
    turn: { abort: async input => { aborts.push(input); } },
    http: { fetch: async (url, options) => {
      const body = JSON.parse(options.body);
      requests.push({ url, options, body });
      const reply = { ack: ack ? (body.events.at(-1)?.seq ?? 0) : 0, commands };
      commands = [];
      return { ok: true, text: JSON.stringify(await respond(body, reply)) };
    } },
  };
  const emit = async (name, data = {}, next = async value => value) => hooks.get(name)(api, data, next);
  return {
    api, emit, requests, submissions, accepted, aborts, logs,
    rootText(index) { return nativeRootText(accepted[index].text, accepted[index].origin.asUser === true); },
    async start() { await emit('session.start', { cwd: '/workspace' }); },
    async poll(...values) { commands = values; await tick?.(); },
    respond(value) { respond = value; },
    expire() { assert.ok(deadline); deadline(); },
    rotate(value) { descriptor = value; },
    acknowledge(value) { ack = value; },
    events(type) { return requests.flatMap(request => request.body.events).filter(event => event.type === type); },
  };
}

test('submits literal user text without plugin attribution or request markers', async () => {
  const f = await fixture();
  await f.start();
  const text = 'Keep this exact text.\nThe workspace-bridge plugin sent a message: is a quotation.';
  await f.poll({ id: 'literal-request', type: 'prompt', text });
  assert.deepEqual(f.submissions, [{ text, asUser: true }]);
  assert.equal(f.rootText(0), text);
  await f.emit('turn.start', { turnId: 'literal-root', text: f.rootText(0) });
  await f.emit('turn.complete', { turnId: 'literal-root' });
  await f.poll();
  assert.equal(f.events('turn.start').at(-1).data.text, text);
  assert.equal(f.events('turn.complete').at(-1).data.requestId, 'literal-request');
});

test('reloads endpoint and token without changing runtime or replaying a prompt', async () => {
  const f = await fixture();
  f.acknowledge(false);
  await f.start();
  const command = { id: 'request-one', type: 'prompt', text: 'native task' };
  await f.poll(command);
  await f.poll();
  assert.equal(f.submissions.length, 1);
  assert.deepEqual(f.submissions[0], { text: 'native task', asUser: true });
  const identity = f.requests[0].body.runtimeId;
  f.rotate({ url: 'http://127.0.0.1:32102', token: 'second-token' });
  await f.poll(command);
  assert.equal(f.submissions.length, 1);
  assert.equal(f.requests.at(-1).url, 'http://127.0.0.1:32102/v1/runtime');
  assert.equal(f.requests.at(-1).options.headers.Authorization, 'Bearer second-token');
  assert.equal(f.requests.at(-1).body.runtimeId, identity);
  assert.deepEqual(f.requests.at(-1).body.events.slice(0, 1), f.requests[0].body.events);
  f.acknowledge(true);
  await f.poll();
  await f.poll();
  assert.deepEqual(f.requests.at(-1).body.events, []);
});

test('cancels only the matching owned root, never a human or child turn', async () => {
  const f = await fixture();
  await f.start();
  await f.emit('turn.start', { turnId: 'human-root', text: 'human work' });
  await f.poll({ id: 'cancel-human', type: 'cancel', requestId: 'human-root' });
  assert.deepEqual(f.aborts, []);
  await f.emit('turn.complete', { turnId: 'human-root' });
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  await f.emit('turn.start', { turnId: 'owned-root', text: f.rootText(0) });
  await f.emit('turn.start', { turnId: 'child-turn', agentId: 'child', text: '[workspace-bridge:owned-request]' });
  await f.poll({ id: 'cancel-unrelated', type: 'cancel', requestId: 'other-request' });
  assert.deepEqual(f.aborts, []);
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  assert.deepEqual(f.aborts, [{ turnId: 'owned-root' }]);
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  assert.equal(f.aborts.length, 1);
  assert.equal(f.events('turn.start').some(event => event.data.turnId === 'child-turn'), false);
});

for (const isAborted of [false, true]) {
  test('native receipt owns completion and allows identical consecutive prompts, aborted=' + isAborted, async () => {
    const f = await fixture();
    await f.start();
    await f.poll({ id: 'first', type: 'prompt', text: 'same work' });
    await f.poll();
    assert.deepEqual(f.events('overflow'), []);
    assert.equal(f.events('prompt.accepted').at(-1).data.requestId, 'first');
    await f.emit('turn.start', { turnId: 'first-root', text: f.rootText(0) });
    if (isAborted) {
      await f.poll({ id: 'cancel-first', type: 'cancel', requestId: 'first' });
      assert.deepEqual(f.aborts, [{ turnId: 'first-root' }]);
    }
    await f.emit('turn.complete', { turnId: 'first-root', isAborted });
    await f.poll();
    assert.equal(f.events('turn.complete').at(-1).data.requestId, 'first');
    assert.equal(f.requests.at(-1).body.activeTurnId, null);
    if (isAborted) await f.poll({ id: 'resume-auto', type: 'mode', mode: 'automation' });
    await f.poll({ id: 'second', type: 'prompt', text: 'same work' });
    assert.equal(f.submissions.length, 2);
    await f.emit('turn.start', { turnId: 'second-root', text: f.rootText(1) });
    await f.emit('turn.complete', { turnId: 'second-root', isAborted: false });
    await f.poll();
    assert.equal(f.events('turn.start').at(-1).data.requestId, 'second');
    assert.equal(f.events('turn.complete').at(-1).data.requestId, 'second');
    assert.deepEqual(f.events('prompt.rejected'), []);
  });
}

test('mode changes are acknowledged and human takeover does not abort running work', async () => {
  const f = await fixture();
  await f.start();
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  await f.emit('turn.start', { turnId: 'owned-root', text: f.rootText(0) });
  await f.poll({ id: 'human-mode', type: 'mode', mode: 'human' });
  await f.poll();
  assert.deepEqual(f.aborts, []);
  assert.equal(f.requests.at(-1).body.mode, 'human');
  assert.equal(f.events('command.done').some(event => event.data.commandId === 'human-mode'), true);
  await f.poll({ id: 'rejected', type: 'prompt', text: 'do not start' });
  await f.poll();
  assert.equal(f.submissions.length, 1);
  assert.equal(f.events('prompt.rejected').some(event => event.data.requestId === 'rejected'), true);
});

test('session end closes control without aborting native work', async () => {
  const f = await fixture();
  await f.start();
  await f.emit('turn.start', { turnId: 'human-root', text: 'native human work' });
  let delivered = false;
  f.respond(async (_, reply) => {
    if (delivered) return reply;
    delivered = true;
    return { ...reply, commands: [{ id: 'after-end', type: 'prompt', text: 'must not submit' }] };
  });
  await f.emit('session.end', { reason: 'quit' });
  assert.equal(f.requests.at(-1).body.ended, true);
  assert.equal(f.events('session.end').length, 1);
  assert.equal(f.events('prompt.rejected').at(-1).data.requestId, 'after-end');
  assert.deepEqual(f.submissions, []);
  assert.deepEqual(f.aborts, []);
});

for (const origin of [{ kind: 'composer' }, { kind: 'bridge' }, { kind: 'plugin', name: 'other' }, { kind: 'plugin', name: 'workspace-bridge', asUser: true }, undefined]) {
  test('never owns identical text in a competing prompt: ' + JSON.stringify(origin), async () => {
    const f = await fixture();
    await f.start();
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    const text = f.rootText(0);
    await f.emit('prompt.submit', { text, origin });
    await f.emit('turn.start', { turnId: 'human-root', text });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    await f.poll();
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('turn.start').at(-1).data.requestId, undefined);
  });
}

test('a native submission receipt cannot adopt a second root', async () => {
  const f = await fixture();
  await f.start();
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  const text = f.rootText(0);
  await f.emit('turn.start', { turnId: 'owned-root', text });
  await f.emit('prompt.submit', { text, origin: { kind: 'plugin', name: 'workspace-bridge' } });
  await f.emit('turn.start', { turnId: 'other-root', text });
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  await f.poll();
  assert.deepEqual(f.aborts, []);
  assert.equal(f.events('turn.start').at(-1).data.requestId, undefined);
});

for (const origin of [undefined, { kind: 'composer' }, { kind: 'plugin', name: 'other', asUser: true }, { kind: 'plugin', name: 'workspace-bridge' }, { kind: 'plugin', name: 'workspace-bridge', asUser: false }]) {
  test('a submission result without native ownership fails closed: ' + JSON.stringify(origin), async () => {
    const f = await fixture();
    await f.start();
    f.api.prompt.submit = async input => ({ ...nativeSubmission(input.text), origin });
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    await f.emit('turn.start', { turnId: 'unowned', text: '[workspace-bridge:owned-request]\nowned work' });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('overflow').length, 1);
    assert.equal(f.requests.at(-1).body.mode, 'human');
  });
}

for (const text of [undefined, 42, '', 'owned', 'prefix owned work', 'owned work suffix', '[workspace-bridge:owned-request]\nowned work']) {
  test('a receipt without the complete submitted payload fails closed: ' + JSON.stringify(text), async () => {
    const f = await fixture();
    await f.start();
    f.api.prompt.submit = async () => ({ ...nativeSubmission(''), text });
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    await f.poll();
    assert.equal(f.events('overflow').length, 1);
    assert.deepEqual(f.events('prompt.accepted'), []);
    assert.equal(f.requests.at(-1).body.mode, 'human');
    await f.emit('turn.start', { turnId: 'unowned', text: nativeSubmission('[workspace-bridge:owned-request]\nowned work').text });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    assert.deepEqual(f.aborts, []);
  });
}

for (const origin of [{ kind: 'composer' }, { kind: 'bridge' }, { kind: 'plugin', name: 'other', asUser: true }, { kind: 'plugin', name: 'workspace-bridge', asUser: true }]) {
  test('an observed submission fences later reservations until its root completes: ' + JSON.stringify(origin), async () => {
    const f = await fixture();
    await f.start();
    await f.emit('prompt.submit', { text: 'same work', origin });
    await f.poll({ id: 'auto', type: 'mode', mode: 'automation' });
    await f.poll({ id: 'too-early', type: 'prompt', text: 'same work' });
    await f.poll();
    assert.deepEqual(f.submissions, []);
    assert.equal(f.events('prompt.rejected').at(-1).data.requestId, 'too-early');
    await f.emit('turn.start', { turnId: 'competing-root', text: 'same work' });
    await f.poll({ id: 'cancel', type: 'cancel', requestId: 'too-early' });
    await f.emit('turn.complete', { turnId: 'competing-root' });
    await f.poll();
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('turn.start').at(-1).data.requestId, undefined);
    assert.equal(f.events('turn.complete').at(-1).data.requestId, undefined);
    await f.poll({ id: 'after-completion', type: 'prompt', text: 'same work' });
    await f.poll();
    assert.equal(f.submissions.length, 1);
    assert.equal(f.events('prompt.accepted').at(-1).data.requestId, 'after-completion');
  });
}

test('each observed submission needs its own completed root, not the first or an unrelated root', async () => {
  const f = await fixture();
  await f.start();
  const origin = { kind: 'plugin', name: 'other', asUser: true };
  for (const text of ['first work', 'second work']) await f.emit('prompt.submit', { text, origin });
  for (const text of ['unrelated work', 'second work']) {
    await f.emit('turn.start', { turnId: text, text });
    await f.emit('turn.complete', { turnId: text });
    await f.poll({ id: text, type: 'prompt', text: 'first work' });
    assert.deepEqual(f.submissions, []);
  }
  await f.emit('turn.start', { turnId: 'first-root', text: 'first work' });
  await f.emit('turn.complete', { turnId: 'first-root' });
  await f.poll({ id: 'after-all', type: 'prompt', text: 'first work' });
  assert.equal(f.submissions.length, 1);
});

test('owned and child completions do not release an observed submission', async () => {
  const f = await fixture();
  await f.start();
  await f.poll({ id: 'owned', type: 'prompt', text: 'same work' });
  await f.emit('turn.start', { turnId: 'owned-root', text: 'same work' });
  await f.emit('prompt.submit', nativeSubmission('same work'));
  await f.emit('turn.start', { turnId: 'child', agentId: 'child', text: 'same work' });
  await f.emit('turn.complete', { turnId: 'child', agentId: 'child' });
  await f.emit('turn.complete', { turnId: 'owned-root' });
  await f.poll({ id: 'too-early', type: 'prompt', text: 'same work' });
  assert.equal(f.submissions.length, 1);
  await f.emit('turn.start', { turnId: 'competing-root', text: 'same work' });
  await f.emit('turn.complete', { turnId: 'competing-root' });
  await f.poll({ id: 'after-both', type: 'prompt', text: 'same work' });
  assert.equal(f.submissions.length, 2);
});

test('unresolved observed submissions have a bounded tracking budget without dropping native hooks', async () => {
  const f = await fixture();
  await f.start();
  let forwarded = 0;
  for (let i = 0; i < 65; i++) {
    await f.emit('prompt.submit', nativeSubmission('work ' + i), async event => { forwarded++; return event; });
  }
  await f.poll({ id: 'overflowed', type: 'prompt', text: 'work' });
  assert.equal(forwarded, 65);
  assert.equal(f.events('overflow').length, 1);
  assert.deepEqual(f.submissions, []);
});

test('overlapping identical observed submissions stay fenced rather than guessing root ownership', async () => {
  const f = await fixture();
  await f.start();
  const event = nativeSubmission('same work');
  await f.emit('prompt.submit', event);
  await f.emit('prompt.submit', event);
  for (const turnId of ['first-root', 'second-root']) {
    await f.emit('turn.start', { turnId, text: event.text });
    await f.emit('turn.complete', { turnId });
    await f.poll({ id: turnId, type: 'prompt', text: event.text });
    assert.deepEqual(f.submissions, []);
  }
});

test('an observed root completing before its hook returns stays fenced until the unchanged result', async () => {
  const f = await fixture();
  await f.start();
  const entered = deferred();
  const release = deferred();
  const event = { text: 'human work', origin: { kind: 'composer' } };
  const hook = f.emit('prompt.submit', event, async () => { entered.resolve(); await release.promise; return event; });
  await entered.promise;
  await f.emit('turn.start', { turnId: 'human-root', text: event.text });
  await f.emit('turn.complete', { turnId: 'human-root' });
  await f.poll({ id: 'auto', type: 'mode', mode: 'automation' });
  await f.poll({ id: 'before-hook-return', type: 'prompt', text: event.text });
  assert.deepEqual(f.submissions, []);
  release.resolve();
  assert.deepEqual(await hook, event);
  await f.poll({ id: 'after-hook-return', type: 'prompt', text: event.text });
  assert.equal(f.submissions.length, 1);
});

for (const outcome of ['drop', 'throw', 'rewrite']) {
  for (const rootFirst of [false, true]) {
    test('an observed ' + outcome + ' stays fenced, root before hook result=' + rootFirst, async () => {
      const f = await fixture();
      await f.start();
      const entered = deferred();
      const release = deferred();
      const event = nativeSubmission('same work');
      const hook = f.emit('prompt.submit', event, async () => {
        entered.resolve();
        await release.promise;
        if (outcome === 'throw') throw new Error('downstream hook failed');
        return outcome === 'drop' ? { drop: 'downstream hook dropped' } : { ...event, text: 'rewritten' };
      });
      await entered.promise;
      const root = async () => {
        await f.emit('turn.start', { turnId: 'observed-root', text: event.text });
        await f.emit('turn.complete', { turnId: 'observed-root' });
      };
      if (rootFirst) await root();
      release.resolve();
      if (outcome === 'throw') await assert.rejects(hook, /downstream hook failed/);
      else assert.deepEqual(await hook, outcome === 'drop' ? { drop: 'downstream hook dropped' } : { ...event, text: 'rewritten' });
      if (!rootFirst) await root();
      // A later, genuine human turn must not erase the unresolved submission.
      await f.emit('prompt.submit', { text: 'human work', origin: { kind: 'composer' } });
      await f.emit('turn.start', { turnId: 'human-root', text: 'human work' });
      await f.emit('turn.complete', { turnId: 'human-root' });
      await f.poll({ id: 'auto', type: 'mode', mode: 'automation' });
      await f.poll({ id: 'still-fenced', type: 'prompt', text: event.text });
      await f.poll();
      assert.deepEqual(f.submissions, []);
      assert.equal(f.events('prompt.rejected').at(-1).data.requestId, 'still-fenced');
    });
  }
}

test('default plugin attribution cannot be cleared by a literal asUser root', async () => {
  const f = await fixture();
  await f.start();
  await f.emit('prompt.submit', nativeSubmission('same work', false));
  await f.emit('prompt.submit', { text: 'same work', origin: { kind: 'plugin', name: 'other', asUser: true } });
  for (const [turnId, text] of [['literal', 'same work'], ['wrapped', nativeRootText('same work', false)]]) {
    await f.emit('turn.start', { turnId, text });
    await f.emit('turn.complete', { turnId });
    await f.poll({ id: turnId, type: 'prompt', text: 'same work' });
    assert.deepEqual(f.submissions, []);
  }
});

for (const origin of [{ kind: 'plugin', name: 'other' }, { kind: 'plugin', name: 'workspace-bridge', asUser: true }]) {
  test('a competing submission during draft read rejects before dispatch: ' + JSON.stringify(origin), async () => {
    const f = await fixture();
    await f.start();
    const entered = deferred();
    const draft = deferred();
    f.api.prompt.read = async () => { entered.resolve(); return draft.promise; };
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'same work' });
    await entered.promise;
    await f.emit('prompt.submit', { text: 'same work', origin });
    draft.resolve({ text: '' });
    await f.poll();
    await f.poll();
    assert.deepEqual(f.submissions, []);
    assert.equal(f.events('prompt.rejected').at(-1).data.requestId, 'owned-request');
    await f.emit('turn.start', { turnId: 'competing-root', text: 'same work' });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    await f.emit('turn.complete', { turnId: 'competing-root' });
    await f.poll();
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('turn.start').at(-1).data.requestId, undefined);
    assert.equal(f.events('turn.complete').at(-1).data.requestId, undefined);
  });
}

test('a human root started during draft read releases its fence after completion', async () => {
  const f = await fixture();
  await f.start();
  const entered = deferred();
  const draft = deferred();
  f.api.prompt.read = async () => { entered.resolve(); return draft.promise; };
  await f.poll({ id: 'first', type: 'prompt', text: 'same work' });
  await entered.promise;
  await f.emit('prompt.submit', { text: 'same work', origin: { kind: 'composer' } });
  await f.emit('turn.start', { turnId: 'human-root', text: 'same work' });
  draft.resolve({ text: '' });
  await f.poll();
  await f.poll();
  assert.deepEqual(f.submissions, []);
  assert.equal(f.events('prompt.rejected').at(-1).data.requestId, 'first');
  await f.emit('turn.complete', { turnId: 'human-root' });
  await f.poll({ id: 'observe', type: 'mode', mode: 'automation' });
  await f.poll({ id: 'second', type: 'prompt', text: 'same work' });
  await f.poll();
  assert.deepEqual(f.submissions, [{ text: 'same work', asUser: true }]);
  assert.equal(f.events('prompt.accepted').at(-1).data.requestId, 'second');
  assert.deepEqual(f.events('overflow'), []);
});

for (const origin of [{ kind: 'composer' }, { kind: 'plugin', name: 'workspace-bridge', asUser: true }]) {
  test('a delayed receipt cannot rearm ownership after an identical competing submission: ' + JSON.stringify(origin), async () => {
    const f = await fixture();
    await f.start();
    const entered = deferred();
    const release = deferred();
    f.api.prompt.submit = async input => {
      entered.resolve(input);
      await release.promise;
      return nativeSubmission(input.text);
    };
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    const input = await entered.promise;
    await f.emit('prompt.submit', { ...input, origin });
    release.resolve();
    await f.poll();
    await f.emit('turn.start', { turnId: 'competing-root', text: input.text });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('overflow').length, 1);
  });
}

test('a second identical root invalidates an in-flight receipt', async () => {
  const f = await fixture();
  await f.start();
  const entered = deferred();
  const release = deferred();
  f.api.prompt.submit = async input => {
    entered.resolve(input);
    await release.promise;
    return nativeSubmission(input.text);
  };
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  const input = await entered.promise;
  await f.emit('turn.start', { turnId: 'candidate-root', text: nativeRootText(input.text) });
  await f.emit('turn.start', { turnId: 'other-root', text: nativeRootText(input.text) });
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  assert.deepEqual(f.aborts, []);
  release.resolve();
  await f.poll();
  await f.poll();
  assert.equal(f.events('overflow').length, 1);
});

test('a marker alone does not match the native accepted text', async () => {
  const f = await fixture();
  await f.start();
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  await f.emit('turn.start', { turnId: 'other-root', text: '[workspace-bridge:owned-request]' });
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  assert.deepEqual(f.aborts, []);
  assert.equal(f.events('turn.start').at(-1).data.requestId, undefined);
});

for (const completion of [undefined, false, true]) {
  test('holds a root and its events until the native receipt arrives, completion=' + completion, async () => {
    const f = await fixture();
    await f.start();
    const entered = deferred();
    const release = deferred();
    let dispatches = 0;
    f.api.prompt.submit = async input => {
      dispatches++;
      entered.resolve(input);
      await release.promise;
      return nativeSubmission(input.text);
    };
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    const input = await entered.promise;
    await f.emit('turn.start', { turnId: 'owned-root', text: nativeRootText(input.text) });
    const stream = await f.emit('turn.step', { turnId: 'owned-root' }, async function* () {
      yield { kind: 'text', text: 'native output' };
    });
    for await (const chunk of stream) assert.equal(chunk.text, 'native output');
    await f.emit('classic.PermissionRequest', { tool_use_id: 'tool-one' });
    await f.emit('tool.call', { tool: 'Bash', tool_use_id: 'tool-one' });
    if (completion !== undefined) await f.emit('turn.complete', { turnId: 'owned-root', isAborted: completion });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    assert.deepEqual(f.aborts, [], 'a candidate is not cancellation authority');
    assert.deepEqual(f.events('turn.start'), []);
    assert.deepEqual(f.events('tool.start'), []);
    assert.deepEqual(f.events('text'), []);
    const identity = f.requests.at(-1).body.runtimeId;
    f.rotate({ url: 'http://127.0.0.1:32102', token: 'rotated-token' });
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    assert.equal(dispatches, 1, 'rejoining observer must not replay a pending prompt');
    assert.equal(f.requests.at(-1).body.runtimeId, identity);
    release.resolve();
    await f.poll();
    await f.poll();
    assert.deepEqual(f.events('overflow'), []);
    assert.equal(f.events('turn.start').at(-1).data.requestId, 'owned-request');
    assert.equal(f.events('tool.end').at(-1).data.requestId, 'owned-request');
    assert.equal(f.events('text').at(-1).data.requestId, 'owned-request');
    assert.equal(f.events('needs.input').at(-1).data.requestId, 'owned-request');
    assert.deepEqual(f.aborts, completion === undefined ? [{ turnId: 'owned-root' }] : []);
    if (completion === undefined) await f.emit('turn.complete', { turnId: 'owned-root', isAborted: true });
    await f.poll();
    assert.equal(f.events('turn.complete').at(-1).data.requestId, 'owned-request');
    await f.poll({ id: 'auto', type: 'mode', mode: 'automation' });
    await f.poll({ id: 'second', type: 'prompt', text: 'next work' });
    await f.poll();
    assert.equal(f.events('prompt.accepted').at(-1).data.requestId, 'second');
  });
}

for (const takeover of ['mode-command', 'native-command']) {
  test('human takeover leaves an already running candidate alone: ' + takeover, async () => {
    const f = await fixture();
    await f.start();
    const entered = deferred();
    const release = deferred();
    f.api.prompt.submit = async input => {
      entered.resolve(input);
      await release.promise;
      return nativeSubmission(input.text);
    };
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    const input = await entered.promise;
    await f.emit('turn.start', { turnId: 'candidate', text: nativeRootText(input.text) });
    if (takeover === 'mode-command') await f.poll({ id: 'human', type: 'mode', mode: 'human' });
    else await f.emit('command.run:bridge-human');
    release.resolve();
    await f.poll();
    await f.poll();
    assert.deepEqual(f.events('overflow'), []);
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('turn.start').at(-1).data.requestId, 'owned-request');
    assert.equal(f.requests.at(-1).body.mode, 'human');
  });
}

test('an unproven root has a bounded observation hold and late receipts cannot rearm it', async () => {
  const f = await fixture();
  await f.start();
  const entered = deferred();
  const release = deferred();
  f.api.prompt.submit = async input => { entered.resolve(input); await release.promise; return nativeSubmission(input.text); };
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  const input = await entered.promise;
  await f.emit('turn.start', { turnId: 'owned-root', text: nativeRootText(input.text) });
  f.expire();
  await f.poll();
  assert.equal(f.events('overflow').length, 1);
  release.resolve();
  await f.poll({ id: 'cancel', type: 'cancel', requestId: 'owned-request' });
  await f.poll();
  assert.deepEqual(f.events('prompt.accepted'), []);
  assert.deepEqual(f.aborts, []);
});

for (const conflict of ['human', 'plugin', 'receipt-text', 'receipt-origin', 'drop', 'exception', 'end', 'overflow']) {
  test('a candidate root fails closed on ' + conflict, async () => {
    const f = await fixture();
    await f.start();
    const entered = deferred();
    const release = deferred();
    f.api.prompt.submit = async input => {
      entered.resolve(input);
      await release.promise;
      if (conflict === 'exception') throw new Error('native transport lost');
      const result = nativeSubmission(input.text, input.asUser === true);
      if (conflict === 'receipt-text') result.text += ' rewritten';
      if (conflict === 'receipt-origin') delete result.origin;
      if (conflict === 'drop') result.drop = 'native dropped prompt';
      return result;
    };
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    const input = await entered.promise;
    await f.emit('turn.start', { turnId: 'candidate', text: nativeRootText(input.text) });
    if (conflict === 'human' || conflict === 'plugin') {
      await f.emit('prompt.submit', { text: input.text, origin: conflict === 'human' ? { kind: 'composer' } : { kind: 'plugin', name: 'other' } });
    }
    if (conflict === 'end') await f.emit('session.end');
    if (conflict === 'overflow') await f.emit('session.attach', { text: 'x'.repeat(128 * 1024) });
    release.resolve();
    await f.poll({ id: 'cancel', type: 'cancel', requestId: 'owned-request' });
    await f.poll();
    assert.equal(f.events('overflow').length, 1);
    assert.deepEqual(f.events('turn.start'), []);
    assert.deepEqual(f.events('prompt.accepted'), []);
    assert.deepEqual(f.events('prompt.rejected'), []);
    assert.deepEqual(f.aborts, []);
  });
}

test('pinned 2.1.309 queue, receipt and root contract', { skip: !process.env.CLAUDE_CONTRACT_BINARY }, async () => {
  const { loadPinnedAPI } = await import('./pinned-api.mjs');
  const native = await loadPinnedAPI(process.env.CLAUDE_CONTRACT_BINARY);
  const f = await fixture();
  await f.start();
  let dispatched;
  let rootID;
  f.api.prompt.submit = input => {
    dispatched = native.submit(input, async event => {
      assert.deepEqual(input, { text: 'offline contract only', asUser: true });
      assert.equal(event.text, 'offline contract only');
      rootID = event.turnId;
      await f.emit('turn.start', event);
      await f.poll();
      assert.deepEqual(f.events('turn.start'), [], 'unproven root must not be published');
    });
    return dispatched.then(({ receipt }) => {
      assert.deepEqual(receipt, nativeSubmission(input.text));
      return receipt;
    });
  };
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'offline contract only' });
  await dispatched;
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  await f.emit('turn.complete', { turnId: rootID, isAborted: true });
  await f.poll();
  assert.deepEqual(f.events('overflow'), []);
  assert.equal(f.events('turn.start').at(-1).data.requestId, 'owned-request');
  assert.equal(f.events('turn.complete').at(-1).data.requestId, 'owned-request');
  assert.deepEqual(f.aborts, [{ turnId: rootID }]);
});

test('native rejection of slash commands does not decorate or replay the prompt', { skip: !process.env.CLAUDE_CONTRACT_BINARY }, async () => {
  const { loadPinnedAPI } = await import('./pinned-api.mjs');
  const native = await loadPinnedAPI(process.env.CLAUDE_CONTRACT_BINARY);
  const f = await fixture();
  await f.start();
  const submissions = [];
  let submission;
  f.api.prompt.submit = input => {
    submissions.push(input);
    submission = native.submit(input).then(({ receipt }) => receipt);
    return submission;
  };
  await f.poll({ id: 'slash-request', type: 'prompt', text: '/help' });
  await assert.rejects(submission, /text beginning with \/ would run a command/);
  await f.poll();
  assert.deepEqual(submissions, [{ text: '/help', asUser: true }]);
  assert.equal(f.events('prompt.rejected').at(-1).data.requestId, 'slash-request');
  assert.deepEqual(f.events('prompt.accepted'), []);
  assert.deepEqual(f.events('overflow'), []);
  f.api.prompt.submit = async input => {
    submissions.push(input);
    return nativeSubmission(input.text);
  };
  await f.poll({ id: 'next-request', type: 'prompt', text: 'plain text' });
  await f.poll();
  assert.equal(submissions.length, 2);
  assert.equal(f.events('prompt.accepted').at(-1).data.requestId, 'next-request');
});

for (const text of ['owned', 'prefix owned work', 'owned work suffix', nativeRootText('owned work', false)]) {
  test('root text must exactly match the native receipt: ' + JSON.stringify(text), async () => {
    const f = await fixture();
    await f.start();
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    await f.emit('turn.start', { turnId: 'other-root', text });
    await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
    assert.deepEqual(f.aborts, []);
    assert.equal(f.events('turn.start').at(-1).data.requestId, undefined);
  });
}

test('cancels a queued prompt when its positively identified root finally starts', async () => {
  const f = await fixture();
  await f.start();
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  await f.emit('turn.start', { turnId: 'owned-root', text: f.rootText(0) });
  assert.deepEqual(f.aborts, [{ turnId: 'owned-root' }]);
});

test('cancellation before native submission preserves the protocol field', async () => {
  const f = await fixture();
  await f.start();
  const entered = deferred();
  const draft = deferred();
  f.api.prompt.read = async () => { entered.resolve(); return draft.promise; };
  await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
  await entered.promise;
  await f.poll({ id: 'cancel-owned', type: 'cancel', requestId: 'owned-request' });
  draft.resolve({ text: '' });
  await f.poll();
  await f.poll();
  assert.deepEqual(f.submissions, []);
  assert.equal(f.events('prompt.rejected').at(-1).data.cancelled, true);
  assert.equal(f.events('prompt.rejected').at(-1).data.canceled, undefined);
});

for (const outcome of ['approved', 'denied', 'error', 'turn-complete', 'aborted']) {
  test('resolves permission state after ' + outcome, async () => {
    const f = await fixture();
    await f.start();
    await f.poll({ id: 'owned-request', type: 'prompt', text: 'owned work' });
    await f.emit('turn.start', { turnId: 'owned-root', text: f.rootText(0) });
    const permission = { session_id: 'native-uuid', tool_name: 'Bash', tool_use_id: 'tool-one', tool_input: { command: 'pwd' } };
    await f.emit('classic.PermissionRequest', { ...permission, session_id: 'other-session' });
    await f.poll();
    assert.equal(f.events('needs.input').length, 0);
    await f.emit('classic.PermissionRequest', permission);
    await f.poll();
    assert.equal(f.events('needs.input').at(-1).data.pending, true);
    await f.emit('turn.complete', { turnId: 'child', agentId: 'child' });
    await f.poll();
    assert.equal(f.events('needs.input').at(-1).data.pending, true);
    if (['approved', 'denied', 'error'].includes(outcome)) {
      const completed = f.emit('tool.call', { tool: 'Bash', tool_use_id: 'tool-one' }, async () => {
        if (outcome === 'error') throw new Error('tool failed');
        return outcome === 'denied' ? { deny: 'user denied' } : { content: 'ok' };
      });
      if (outcome === 'error') await assert.rejects(completed, /tool failed/);
      else await completed;
      await f.poll();
      assert.equal(f.events('needs.input').at(-1).data.pending, false);
    }
    await f.emit('turn.complete', { turnId: 'owned-root', isAborted: outcome === 'aborted' });
    await f.poll();
    assert.equal(f.events('needs.input').at(-1).data.pending, false);
    assert.equal(f.events('needs.input').at(-1).data.requestId, 'owned-request');
    assert.equal(f.events('needs.input').at(-1).data.turnId, 'owned-root');
    assert.equal(f.events('needs.input').length, 2);
  });
}

test('one completed tool does not clear another pending permission', async () => {
  const f = await fixture();
  await f.start();
  await f.emit('turn.start', { turnId: 'root', text: 'native work' });
  for (const tool_use_id of ['one', 'two']) await f.emit('classic.PermissionRequest', { tool_use_id });
  await f.emit('tool.call', { tool_use_id: 'one' });
  await f.poll();
  assert.equal(f.events('needs.input').at(-1).data.pending, true);
  await f.emit('tool.call', { tool_use_id: 'two' });
  await f.poll();
  assert.equal(f.events('needs.input').at(-1).data.pending, false);
});

function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

test('session end joins an in-flight poll and drains more than one batch to ACK', async () => {
  const f = await fixture();
  await f.start();
  for (let i = 0; i < 300; i++) await f.emit('session.attach', { clientId: String(i) });
  const entered = deferred();
  const release = deferred();
  f.respond(async (_, reply) => { entered.resolve(); await release.promise; return reply; });
  const polling = f.poll();
  await entered.promise;
  let finished = false;
  const ending = f.emit('session.end', { reason: 'quit' }).then(() => { finished = true; });
  assert.equal(finished, false);
  release.resolve();
  await Promise.all([polling, ending]);
  assert.equal(f.events('session.attach').length, 300);
  assert.equal(f.events('session.end').length, 1);
  assert.equal(f.requests.at(-1).body.events.at(-1).type, 'session.end');
  const count = f.requests.length;
  await f.poll();
  assert.equal(f.requests.length, count, 'the periodic poll must be cancelled');
  assert.deepEqual(f.logs, []);
});

test('session end retries an unacknowledged batch', async () => {
  const f = await fixture();
  await f.start();
  let attempts = 0;
  f.respond(async (_, reply) => ++attempts === 1 ? { ack: 0, commands: [] } : reply);
  await f.emit('session.end');
  assert.equal(attempts, 2);
  assert.equal(f.events('session.end').length, 2);
  assert.deepEqual(f.logs, []);
});

test('session end has a bounded drain when an in-flight request never completes', async () => {
  const f = await fixture();
  await f.start();
  const entered = deferred();
  const release = deferred();
  f.respond(async (_, reply) => { entered.resolve(); await release.promise; return reply; });
  const polling = f.poll();
  await entered.promise;
  const ending = f.emit('session.end');
  f.expire();
  await ending;
  assert.match(f.logs.at(-1), /not acknowledged/);
  const count = f.requests.length;
  release.resolve();
  await polling;
  await f.poll();
  assert.equal(f.requests.length, count);
});

test('serializes HTML characters without escaping and faults oversized events', async () => {
  const f = await fixture();
  await f.start();
  const text = '&'.repeat(25000);
  await f.emit('turn.start', { turnId: 'root', text });
  await f.poll();
  assert.equal(f.events('turn.start').at(-1).data.text, text);
  assert.ok(Buffer.byteLength(f.requests.at(-1).options.body) < 30000);
  await f.emit('session.attach', { text: 'x'.repeat(128 * 1024) });
  await f.poll();
  assert.equal(f.events('overflow').length, 1);
  assert.equal(f.requests.at(-1).body.mode, 'human');
});

test('rejects descriptor redirects without leaking the bearer token', async () => {
  const f = await fixture();
  await f.start();
  const count = f.requests.length;
  f.rotate({ url: 'https://example.com', token: 'private-token' });
  await f.poll();
  assert.equal(f.requests.length, count);
  f.rotate({ url: 'http://127.0.0.1:32102', token: 'new-token' });
  await f.poll();
  assert.equal(f.requests.length, count + 1);
});
