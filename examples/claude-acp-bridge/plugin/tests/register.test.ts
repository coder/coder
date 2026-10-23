import type {
  On,
  PromptSubmitInput,
  SessionVersion,
  TurnStepChunk,
  TurnStepResult,
} from 'claude-code';
import { type Engine, expect, mock, test } from 'claude-code/testing';

const SESSION_ID = 'native-test-session';
const CWD = '/work/bridge-test';
const VERSION: SessionVersion = { version: '2.1.306', base: '2.1.306' };
const DESCRIPTOR = { url: 'http://127.0.0.1:43210', token: 'offline-test-token' };

type Command =
  | { id: string; type: 'prompt'; text: string }
  | { id: string; type: 'cancel'; requestId: string }
  | { id: string; type: 'mode'; mode: 'automation' | 'human' };

type BridgeEvent = {
  seq: number;
  type: string;
  data: Record<string, unknown>;
};

type Heartbeat = {
  sessionId: string;
  runtimeId: string;
  launchId: string;
  version: SessionVersion;
  cwd: string;
  mode: 'automation' | 'human';
  activeTurnId: string | null;
  ended: boolean;
  events: BridgeEvent[];
};

// The native kit reloads the real hooks module for every test. No manual
// register() call or shared import can hide module-state leakage or API errors.
function bridge(on: On, mode: 'automation' | 'human' = 'automation') {
  const clock = mock.clock(on);
  const commands: Command[] = [];
  const heartbeats: Heartbeat[] = [];
  const submissions: PromptSubmitInput[] = [];
  const aborts: string[] = [];
  const registrations: { name: string; immediate?: boolean }[] = [];
  const logs: string[] = [];
  const state = {
    draft: '',
    readCount: 0,
    readDelayMs: 0,
    submitDelayMs: 0,
    drop: '',
  };

  mock.env(on, {
    WORKSPACE_BRIDGE_FILE: '/work/bridge-test/descriptor.json',
    WORKSPACE_BRIDGE_LAUNCH_ID: 'test-launch',
    WORKSPACE_BRIDGE_MODE: mode,
  });
  on('fs.read', ($, e) => {
    expect(e.path).toBe('/work/bridge-test/descriptor.json');
    return { value: JSON.stringify(DESCRIPTOR) };
  });
  on('session.id', () => ({ value: SESSION_ID }));
  on('session.version', () => ({ value: VERSION }));
  on('session.start', () => ({ cwd: CWD }));
  on('session.end', ($, e) => ({ sessionId: e.sessionId }));
  on('command.register', ($, e) => {
    registrations.push(e);
    return { value: undefined };
  });
  on('ui.log', ($, e) => {
    logs.push(e.text);
    return { value: undefined };
  });
  on('http.fetch', ($, e) => {
    expect(e.url).toBe(DESCRIPTOR.url + '/v1/runtime');
    expect(e.init?.method).toBe('POST');
    expect(e.init?.headers).toEqual({
      Authorization: 'Bearer ' + DESCRIPTOR.token,
      'Content-Type': 'application/json',
    });
    const body: Heartbeat = JSON.parse(e.init?.body as string);
    heartbeats.push(body);
    return {
      value: {
        status: 200,
        ok: true,
        headers: {},
        text: JSON.stringify({
          ack: body.events.at(-1)?.seq ?? 0,
          commands: commands.splice(0),
        }),
      },
    };
  });
  on('prompt.read', async () => {
    state.readCount += 1;
    if (state.readDelayMs) await clock.sleep(state.readDelayMs);
    return { value: { text: state.draft, cursor: state.draft.length } };
  });
  on('prompt.submit', async ($, e) => {
    submissions.push(e);
    if (e.origin.kind === 'plugin') {
      if (state.submitDelayMs) await clock.sleep(state.submitDelayMs);
      if (state.drop) return { drop: state.drop };
    }
    return { text: e.text, origin: e.origin };
  });
  on('turn.start', ($, e) => ({ turnId: e.turnId }));
  on('turn.complete', () => ({ text: 'native completion notice' }));
  on('turn.abort', ($, e) => {
    aborts.push(e.turnId);
    return { value: undefined };
  });

  return {
    clock,
    commands,
    heartbeats,
    submissions,
    aborts,
    registrations,
    logs,
    state,
    events(type: string) {
      return heartbeats.flatMap(body => body.events).filter(event => event.type === type);
    },
    data(type: string) {
      return this.events(type).map(event => event.data);
    },
    async poll(...queued: Command[]) {
      commands.push(...queued);
      await clock.advance(200);
    },
  };
}

async function start($: Engine, fixture: ReturnType<typeof bridge>) {
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: CWD });
  expect(fixture.logs).toEqual([]);
  expect(fixture.heartbeats.length).toBe(1);
  expect(fixture.events('session.start')[0].seq).toBe(1);
}

async function status($: Engine) {
  const result = await $.command.run({ command: 'bridge-status', args: '' });
  return JSON.parse(result.text as string);
}

async function complete($: Engine, turnId: string, agentId?: string, isAborted = false) {
  return $.turn.complete({
    turnId,
    agentId,
    answer: 'finished ' + turnId,
    durationMs: 12,
    isAborted,
    reason: isAborted ? 'aborted' : 'answer',
  });
}

function prompt(id: string): Command {
  return { id, type: 'prompt', text: 'Do offline work for ' + id };
}

function marker(id: string) {
  return '[workspace-bridge:' + id + ']';
}

test('registers the native session and sends acknowledged heartbeats on the mock clock', async ($, on) => {
  const fixture = bridge(on, 'human');
  await start($, fixture);

  expect(fixture.registrations.map(({ name, immediate }) => ({ name, immediate }))).toEqual([
    { name: 'bridge-status', immediate: true },
    { name: 'bridge-human', immediate: true },
    { name: 'bridge-auto', immediate: true },
  ]);
  const first = fixture.heartbeats[0];
  expect(first).toMatchObject({
    sessionId: SESSION_ID,
    launchId: 'test-launch',
    version: VERSION,
    cwd: CWD,
    mode: 'human',
    activeTurnId: null,
    ended: false,
  });
  expect(first.runtimeId).toMatch(/^native-test-session:[a-z0-9]+$/);
  expect(first.events).toEqual([{
    seq: 1,
    type: 'session.start',
    data: { sessionId: SESSION_ID, runtimeId: first.runtimeId, cwd: CWD, version: VERSION },
  }]);

  await fixture.clock.advance(199);
  expect(fixture.heartbeats.length).toBe(1);
  await fixture.clock.advance(1);
  expect(fixture.heartbeats.length).toBe(2);
  await fixture.clock.advance(600);
  expect(fixture.heartbeats.length).toBe(5);
  for (const heartbeat of fixture.heartbeats.slice(1)) {
    expect(heartbeat).toEqual({ ...first, events: [] });
  }
  expect(await status($)).toMatchObject({ connected: true, mode: 'human', overflow: false });
});

test('a polled prompt calls the real mods API with attribution and its correlation marker', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  expect(fixture.submissions).toEqual([]);

  await fixture.poll(prompt('request-one'));
  expect(fixture.submissions.length).toBe(1);
  expect(fixture.submissions[0]).toMatchObject({
    text: marker('request-one') + '\nDo offline work for request-one',
    origin: { kind: 'plugin', name: 'workspace-bridge' },
    wait: false,
  });
  expect(fixture.submissions[0].origin).not.toMatchObject({ asUser: true });
  expect(fixture.state.readCount).toBe(1);
  expect(await status($)).toMatchObject({ mode: 'automation' });
  await fixture.poll();
  expect(fixture.data('prompt.accepted')).toEqual([{ requestId: 'request-one' }]);
  expect(fixture.data('prompt.rejected')).toEqual([]);
});

test('redelivered commands execute once even after the first prompt completes', async ($, on) => {
  const fixture = bridge(on, 'human');
  await start($, fixture);
  const enable: Command = { id: 'enable', type: 'mode', mode: 'automation' };
  const command = prompt('redelivered');
  await fixture.poll(enable, enable, command, command);
  expect(fixture.submissions.length).toBe(1);
  await $.turn.start({ turnId: 'original-turn', text: marker('redelivered') });
  await complete($, 'original-turn');

  await fixture.poll(enable, command);
  await fixture.poll();
  expect(fixture.submissions.length).toBe(1);
  expect(fixture.state.readCount).toBe(1);
  expect(fixture.data('prompt.accepted')).toEqual([{ requestId: 'redelivered' }]);
  expect(fixture.data('command.done')).toEqual([{ commandId: 'enable' }]);
  expect(fixture.data('mode')).toEqual([{ mode: 'automation' }]);
});

test('human control rejects a polled prompt without reading or replacing the native draft', async ($, on) => {
  const fixture = bridge(on, 'human');
  fixture.state.draft = 'unfinished human input';
  await start($, fixture);
  await fixture.poll(prompt('human-rejected'));
  await fixture.poll();
  expect(fixture.submissions).toEqual([]);
  expect(fixture.state.readCount).toBe(0);
  expect(fixture.state.draft).toBe('unfinished human input');
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'human-rejected',
    reason: 'Session is not idle under automation control',
  }]);
});

test('an existing native draft rejects automation and releases the pending reservation', async ($, on) => {
  const fixture = bridge(on);
  fixture.state.draft = 'keep this draft';
  await start($, fixture);
  await fixture.poll(prompt('draft-rejected'));
  await fixture.poll();
  expect(fixture.submissions).toEqual([]);
  expect(fixture.state.draft).toBe('keep this draft');
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'draft-rejected',
    reason: 'Native prompt draft or session state changed before submission',
  }]);

  fixture.state.draft = '';
  await fixture.poll(prompt('after-draft'));
  expect(fixture.submissions.map(e => e.text)).toEqual([
    marker('after-draft') + '\nDo offline work for after-draft',
  ]);
});

test('a busy main turn rejects automation until that exact turn completes', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await $.turn.start({ turnId: 'busy-main', text: 'already running' });
  await fixture.poll(prompt('busy-rejected'));
  await fixture.poll();
  expect(fixture.submissions).toEqual([]);
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'busy-rejected',
    reason: 'Session is not idle under automation control',
  }]);

  await complete($, 'different-turn');
  expect(await status($)).toMatchObject({ activeTurnId: 'busy-main' });
  await complete($, 'busy-main');
  await fixture.poll(prompt('after-busy'));
  expect(fixture.submissions.length).toBe(1);
});

test('a pending submission reserves the session before another command can submit', async ($, on) => {
  const fixture = bridge(on);
  fixture.state.readDelayMs = 600;
  await start($, fixture);
  await fixture.poll(prompt('reserved'), prompt('second'));
  await fixture.poll();
  expect(fixture.submissions).toEqual([]);
  expect(fixture.state.readCount).toBe(1);
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'second',
    reason: 'Session is not idle under automation control',
  }]);
  await fixture.clock.advance(400);
  expect(fixture.submissions.map(e => e.text)).toEqual([
    marker('reserved') + '\nDo offline work for reserved',
  ]);
});

test('only the matching marker correlates a turn, not an unrelated human turn or completion', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await fixture.poll(prompt('correlated'));
  await $.turn.start({ turnId: 'human-turn', text: 'Do offline work for correlated' });
  await complete($, 'human-turn');
  await fixture.poll();
  expect(fixture.data('turn.start')).toEqual([{
    turnId: 'human-turn', text: 'Do offline work for correlated',
  }]);
  expect(fixture.data('turn.complete')[0].requestId).toBeUndefined();

  // Native attribution can precede the marker, so it need not start the text.
  await $.turn.start({
    turnId: 'bridge-turn',
    text: 'The workspace-bridge plugin sent a message:\n' + fixture.submissions[0].text,
  });
  const result = await complete($, 'bridge-turn');
  expect(result).toEqual({ text: 'native completion notice' });
  await fixture.poll();
  expect(fixture.data('turn.start')[1]).toMatchObject({
    turnId: 'bridge-turn', requestId: 'correlated',
  });
  expect(fixture.data('turn.complete')[1]).toMatchObject({
    turnId: 'bridge-turn', requestId: 'correlated', answer: 'finished bridge-turn',
  });
  expect(await status($)).toMatchObject({ activeTurnId: null });
  await fixture.poll(prompt('next-request'));
  expect(fixture.submissions.length).toBe(2);
});

test('subagent starts and completions cannot claim the main request or replace its abort ID', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await fixture.poll(prompt('main-request'));
  await $.turn.start({ turnId: 'main-turn', text: marker('main-request') });
  await $.turn.start({
    turnId: 'child-turn', agentId: 'child-agent', text: 'Discuss ' + marker('main-request'),
  });
  await complete($, 'child-turn', 'child-agent', true);
  await fixture.poll({ id: 'cancel-main', type: 'cancel', requestId: 'main-request' });
  expect(fixture.aborts).toEqual(['main-turn']);
  expect(await status($)).toMatchObject({ activeTurnId: 'main-turn', mode: 'automation' });
  await complete($, 'main-turn');
  await fixture.poll();
  expect(fixture.data('turn.start')).toEqual([{
    turnId: 'main-turn', text: marker('main-request'), requestId: 'main-request',
  }]);
  expect(fixture.data('turn.complete').map(e => ({ turnId: e.turnId, requestId: e.requestId }))).toEqual([
    { turnId: 'main-turn', requestId: 'main-request' },
  ]);
});

test('a subagent completion without a start does not release the pending main request', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await fixture.poll(prompt('pending-main'));
  await complete($, 'child-only-completion', 'child-agent', true);
  await fixture.poll(prompt('still-pending'));
  await fixture.poll();
  expect(fixture.submissions.length).toBe(1);
  expect(fixture.data('turn.complete')).toEqual([]);
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'still-pending', reason: 'Session is not idle under automation control',
  }]);
  expect(await status($)).toMatchObject({ mode: 'automation' });
  await $.turn.start({ turnId: 'eventual-main', text: marker('pending-main') });
  await complete($, 'eventual-main');
  await fixture.poll();
  expect(fixture.data('turn.complete')[0].requestId).toBe('pending-main');
});

test('cancel targets the correlated native turn once and ignores another request ID', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await fixture.poll(prompt('cancel-request'));
  await $.turn.start({ turnId: 'native-turn-to-abort', text: marker('cancel-request') });
  await fixture.poll({ id: 'wrong-cancel', type: 'cancel', requestId: 'unrelated-request' });
  expect(fixture.aborts).toEqual([]);
  const cancel: Command = { id: 'cancel-command', type: 'cancel', requestId: 'cancel-request' };
  await fixture.poll(cancel, cancel);
  await fixture.poll(cancel);
  expect(fixture.aborts).toEqual(['native-turn-to-abort']);
  expect(fixture.data('command.done')).toEqual([
    { commandId: 'wrong-cancel' }, { commandId: 'cancel-command' },
  ]);
  await complete($, 'native-turn-to-abort', undefined, true);
  expect(await status($)).toMatchObject({ mode: 'human', activeTurnId: null });
});

test('cancel remains pollable while prompt.submit waits and aborts when its turn starts', async ($, on) => {
  const fixture = bridge(on);
  fixture.state.submitDelayMs = 800;
  await start($, fixture);
  await fixture.poll(prompt('queued-cancel'));
  expect(fixture.submissions.length).toBe(1);
  await fixture.poll({ id: 'cancel-before-start', type: 'cancel', requestId: 'queued-cancel' });
  expect(fixture.aborts).toEqual([]);
  await $.turn.start({ turnId: 'late-native-turn', text: marker('queued-cancel') });
  expect(fixture.aborts).toEqual(['late-native-turn']);
  await complete($, 'late-native-turn', undefined, true);
  await fixture.clock.advance(600);
  expect(fixture.data('command.done')).toEqual([{ commandId: 'cancel-before-start' }]);
});

test('cancel during prompt.read prevents submission instead of aborting an unrelated turn', async ($, on) => {
  const fixture = bridge(on);
  fixture.state.readDelayMs = 600;
  await start($, fixture);
  await fixture.poll(prompt('cancel-before-read'));
  await fixture.poll({ id: 'cancel-reading', type: 'cancel', requestId: 'cancel-before-read' });
  await fixture.clock.advance(600);
  expect(fixture.submissions).toEqual([]);
  expect(fixture.aborts).toEqual([]);
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'cancel-before-read', reason: 'Cancelled before submission', cancelled: true,
  }]);
});

test('the human native composer takes control without cancelling work already running', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await fixture.poll(prompt('running-request'));
  await $.turn.start({ turnId: 'running-turn', text: marker('running-request') });
  const human = await $.prompt.submit({
    text: 'I will take it from here', origin: { kind: 'composer' }, wait: false,
  });
  expect(human).toMatchObject({ text: 'I will take it from here' });
  expect(await status($)).toMatchObject({ mode: 'human', activeTurnId: 'running-turn' });
  expect(fixture.aborts).toEqual([]);
  await complete($, 'running-turn');
  await fixture.poll(prompt('after-takeover'));
  await fixture.poll();
  expect(fixture.submissions.filter(e => e.origin.kind === 'plugin').length).toBe(1);
  expect(fixture.data('mode')).toEqual([{ mode: 'human' }]);
  expect(fixture.data('prompt.rejected')).toEqual([{
    requestId: 'after-takeover', reason: 'Session is not idle under automation control',
  }]);
});

test('human composer takeover cancels a bridge prompt still waiting to start', async ($, on) => {
  const fixture = bridge(on);
  fixture.state.submitDelayMs = 800;
  await start($, fixture);
  await fixture.poll(prompt('queued-takeover'));
  await $.prompt.submit({ text: 'human input', origin: { kind: 'composer' }, wait: false });
  await $.turn.start({ turnId: 'human-turn', text: 'human input' });
  expect(fixture.aborts).toEqual([]);
  await complete($, 'human-turn');
  await $.turn.start({ turnId: 'queued-bridge-turn', text: marker('queued-takeover') });
  expect(fixture.aborts).toEqual(['queued-bridge-turn']);
  await complete($, 'queued-bridge-turn', undefined, true);
  await fixture.clock.advance(800);
  expect(await status($)).toMatchObject({ mode: 'human', activeTurnId: null });
});

test('session.end reports the old session and stops all further automation polling', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await fixture.poll();
  await $.session.end({ reason: 'clear', sessionId: SESSION_ID, resume: { id: SESSION_ID } });
  const final = fixture.heartbeats.at(-1);
  expect(final).toMatchObject({ sessionId: SESSION_ID, ended: true, mode: 'human' });
  expect(fixture.data('session.end')).toEqual([{ reason: 'clear' }]);
  const count = fixture.heartbeats.length;
  const enable = await $.command.run({ command: 'bridge-auto', args: '' });
  expect(enable.text).toContain('Bridge unavailable');
  fixture.commands.push(prompt('never-polled'));
  await fixture.clock.advance(2000);
  expect(fixture.heartbeats.length).toBe(count);
  expect(fixture.submissions).toEqual([]);
  expect(await status($)).toMatchObject({ mode: 'human' });
});

test('commands returned by the final session.end poll cannot reenable automation', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  fixture.commands.push(
    { id: 'late-enable', type: 'mode', mode: 'automation' },
    prompt('late-prompt'),
  );
  await $.session.end({ reason: 'resume', sessionId: SESSION_ID, resume: { id: SESSION_ID } });
  await fixture.clock.advance(1000);
  expect(fixture.submissions).toEqual([]);
  expect(await status($)).toMatchObject({ mode: 'human' });
});

for (const reason of [
  'the session was stopped and nobody has sent input since',
  'the prompt was removed from the queue before it ran',
  'Blocked by the native prompt policy',
]) {
  test('reports a rejected prompt without acceptance or automatic retry: ' + reason, async ($, on) => {
    const fixture = bridge(on);
    fixture.state.drop = reason;
    await start($, fixture);
    await fixture.poll(prompt('dropped-request'));
    await fixture.poll();
    expect(fixture.data('prompt.rejected')).toEqual([{ requestId: 'dropped-request', reason }]);
    expect(fixture.data('prompt.accepted')).toEqual([]);
    await fixture.clock.advance(1000);
    expect(fixture.submissions.length).toBe(1);

    fixture.state.drop = '';
    await fixture.poll(prompt('after-drop'));
    await fixture.poll();
    expect(fixture.submissions.length).toBe(2);
    expect(fixture.data('prompt.accepted')).toEqual([{ requestId: 'after-drop' }]);
  });
}

test('turn start and completion can arrive before prompt.submit resolves without losing correlation', async ($, on) => {
  const fixture = bridge(on);
  fixture.state.submitDelayMs = 800;
  await start($, fixture);
  await fixture.poll(prompt('fast-turn-request'));
  await $.turn.start({ turnId: 'fast-turn', text: marker('fast-turn-request') });
  await complete($, 'fast-turn');
  await fixture.clock.advance(1000);
  expect(fixture.data('turn.complete')[0]).toMatchObject({
    turnId: 'fast-turn', requestId: 'fast-turn-request',
  });
  expect(fixture.data('prompt.accepted')).toEqual([{ requestId: 'fast-turn-request' }]);
  expect(fixture.data('prompt.rejected')).toEqual([]);
  fixture.state.submitDelayMs = 0;
  await fixture.poll(prompt('after-fast-turn'));
  expect(fixture.submissions.length).toBe(2);
});

test('oversized telemetry faults automation instead of silently truncating or retrying forever', async ($, on) => {
  const fixture = bridge(on);
  await start($, fixture);
  await $.turn.start({ turnId: 'oversized', text: 'x'.repeat(140000) });
  await fixture.poll();
  expect(await status($)).toMatchObject({ mode: 'human', overflow: true });
  expect(fixture.data('overflow').length).toBe(1);
  expect((await $.command.run({ command: 'bridge-auto', args: '' })).text).toContain('Bridge unavailable');
  await fixture.poll(prompt('must-not-run'));
  expect(fixture.submissions).toEqual([]);
});

test('stream observation preserves every chunk and the generator final result, excluding subagent telemetry', async ($, on) => {
  const fixture = bridge(on);
  const usage = {
    model: 'offline-test-model', input_tokens: 11, output_tokens: 7,
    cache_read_input_tokens: 3, cache_creation_input_tokens: 2,
  };
  const chunks: TurnStepChunk[] = [
    { kind: 'thinking', index: 0, text: 'Plan offline' },
    { kind: 'text', index: 1, text: 'Read ' },
    { kind: 'text', index: 1, text: 'the file' },
    { kind: 'tool', index: 2, id: 'tool-use', name: 'Read' },
    { kind: 'input', index: 2, json: '{"file_path":"/work/example"}' },
    { kind: 'stop', stopReason: 'tool_use', usage },
  ];
  const final: Omit<TurnStepResult, 'turnId' | 'index'> = {
    answer: 'Final response metadata is not a yielded chunk',
    toolUses: [{ name: 'Read', input: { file_path: '/work/example' } }],
    stopReason: 'tool_use',
    usage,
  };
  on('turn.step', async function* ($, e) {
    for (const chunk of chunks) yield chunk;
    return { ...final, turnId: e.turnId, index: e.index };
  });
  await start($, fixture);
  await fixture.poll(prompt('stream-request'));
  await $.turn.start({ turnId: 'stream-turn', text: marker('stream-request') });

  for (const agentId of [undefined, 'child-agent']) {
    const turnId = agentId ? 'child-turn' : 'stream-turn';
    const stream = $.turn.step({ turnId, agentId, index: 4, model: 'offline-test-model', messageCount: 2 });
    const received: TurnStepChunk[] = [];
    let part = await stream.next();
    while (!part.done) {
      received.push(part.value);
      part = await stream.next();
    }
    expect(received).toEqual(chunks);
    expect(part.value).toEqual({ ...final, turnId, index: 4 });
  }
  await fixture.poll();
  expect(fixture.data('thought')).toEqual([{
    turnId: 'stream-turn', requestId: 'stream-request', text: 'Plan offline',
  }]);
  expect(fixture.data('text')).toEqual([
    { turnId: 'stream-turn', requestId: 'stream-request', text: 'Read ' },
    { turnId: 'stream-turn', requestId: 'stream-request', text: 'the file' },
  ]);
});
