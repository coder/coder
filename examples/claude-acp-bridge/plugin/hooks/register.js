// This module runs inside native Claude, not in Node or the Agent SDK.
let config;
let sessionId;
let runtimeId;
let launchId;
let version;
let cwd;
let mode = 'human';
let activeTurnId = null;
let pending = null;
let sequence = 0;
let events = [];
let seen = new Set();
let pumping = false;
let ended = false;
let overflow = false;
let timer;
let bufferedBytes = 0;

function failStream(reason) {
  overflow = true;
  mode = 'human';
  events = [{ seq: ++sequence, type: 'overflow', data: { reason } }];
  bufferedBytes = 0;
}

function emit(type, data) {
  if (!config || overflow) return;
  const event = { seq: ++sequence, type, data };
  // Bound serialized bytes, not just event count. Never silently clip output.
  const bytes = new TextEncoder().encode(JSON.stringify(event)).length;
  if (events.length >= 4000 || bytes > 128 * 1024 || bufferedBytes + bytes > 4 * 1024 * 1024) {
    failStream('Mod event buffer exhausted; attach to inspect the session');
    return;
  }
  bufferedBytes += bytes;
  events.push(event);
}

function requestFor(turnId) {
  return pending?.turnId === turnId ? pending.id : undefined;
}

async function submit($, command) {
  if (ended || overflow || mode !== 'automation' || activeTurnId || pending) {
    emit('prompt.rejected', { requestId: command.id, reason: 'Session is not idle under automation control' });
    return;
  }
  const marker = '[workspace-bridge:' + command.id + ']';
  const request = { id: command.id, marker, turnId: null, cancelled: false };
  // Reserve synchronously, before a host call can yield to cancel or takeover.
  pending = request;
  try {
    const draft = await $.prompt.read();
    if (draft.text || mode !== 'automation' || activeTurnId || ended || request.cancelled) {
      emit('prompt.rejected', { requestId: command.id, reason: request.cancelled ? 'Cancelled before submission' : 'Native prompt draft or session state changed before submission', ...(request.cancelled ? { cancelled: true } : {}) });
      if (pending === request) pending = null;
      return;
    }
    // Keep attribution. A visible unique marker correlates turn.start even if
    // the native runtime starts and finishes before this promise resolves.
    const result = await $.prompt.submit({ text: marker + '\n' + command.text });
    if (result.drop) {
      emit('prompt.rejected', { requestId: command.id, reason: result.drop });
      if (pending === request) pending = null;
    } else {
      emit('prompt.accepted', { requestId: command.id });
    }
  } catch (error) {
    emit('prompt.rejected', { requestId: command.id, reason: String(error.message || error) });
    if (pending === request && !request.turnId) pending = null;
  }
}

async function handle($, command) {
  if (seen.has(command.id)) return;
  if (seen.size >= 10000) {
    if (!overflow) failStream('Command deduplication budget exhausted; restart the bridge session');
    return;
  }
  seen.add(command.id);
  if (command.type === 'prompt') {
    // Never wait for idle inside the transport pump; cancel must stay usable.
    submit($, command).catch(error => emit('prompt.rejected', { requestId: command.id, reason: String(error) }));
  } else if (command.type === 'cancel') {
    if (pending?.id === command.requestId) {
      pending.cancelled = true;
      if (pending.turnId && activeTurnId === pending.turnId) {
        await $.turn.abort({ turnId: pending.turnId });
      }
    }
    emit('command.done', { commandId: command.id });
  } else if (command.type === 'mode') {
    if (ended || overflow) {
      emit('command.error', { commandId: command.id, reason: 'Bridge session is closed' });
      return;
    }
    mode = command.mode;
    // A queued bridge prompt must not start after human takeover.
    if (mode === 'human' && pending && !pending.turnId) pending.cancelled = true;
    emit('mode', { mode });
    emit('command.done', { commandId: command.id });
  }
}

async function pump($) {
  if (!config || pumping) return;
  pumping = true;
  const batch = [];
  let bytes = 0;
  for (const event of events.slice(0, 128)) {
    bytes += new TextEncoder().encode(JSON.stringify(event)).length;
    if (bytes > 1024 * 1024) break;
    batch.push(event);
  }
  try {
    const reply = await $.http.fetch(config.url + '/v1/runtime', {
      method: 'POST',
      headers: { Authorization: 'Bearer ' + config.token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ sessionId, runtimeId, launchId, version, cwd, mode, activeTurnId, ended, events: batch }),
    });
    if (!reply.ok) return;
    const body = JSON.parse(reply.text);
    const maxSent = batch.length ? batch[batch.length - 1].seq : 0;
    if (Number.isInteger(body.ack) && body.ack <= maxSent) {
      events = events.filter(event => event.seq > body.ack);
      bufferedBytes = events.reduce((total, event) => total + new TextEncoder().encode(JSON.stringify(event)).length, 0);
    }
    for (const command of body.commands || []) {
      try { await handle($, command); }
      catch (error) { emit('command.error', { commandId: command.id, reason: String(error.message || error) }); }
    }
  } finally {
    pumping = false;
  }
}

async function connect($, e) {
  const file = await $.env.get('WORKSPACE_BRIDGE_FILE');
  if (!file) return;
  config = JSON.parse(await $.fs.read(file));
  const url = new URL(config.url);
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !config.token) {
    config = undefined;
    throw new Error('Bridge descriptor must use authenticated loopback HTTP');
  }
  sessionId = await $.session.id();
  runtimeId = sessionId + ':' + Math.random().toString(36).slice(2);
  launchId = await $.env.get('WORKSPACE_BRIDGE_LAUNCH_ID');
  version = await $.session.version();
  cwd = e.cwd;
  mode = (await $.env.get('WORKSPACE_BRIDGE_MODE')) === 'automation' ? 'automation' : 'human';
  emit('session.start', { sessionId, runtimeId, cwd, version });
  await pump($);
  timer = $.clock.every(200, () => pump($).catch(() => {}));
}

/** Register the native-session control and observation hooks. */
export function register(on) {
  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'bridge-status', description: 'Show workspace bridge state', immediate: true });
    await $.command.register({ name: 'bridge-human', description: 'Pause new bridge prompts; current work continues', immediate: true });
    await $.command.register({ name: 'bridge-auto', description: 'Allow bridge prompts when Claude is idle', immediate: true });
    try { await connect($, e); }
    catch (error) { $.ui.log('Workspace bridge unavailable: ' + String(error.message || error)); }
    return next(e);
  });

  on('command.run', { command: 'bridge-status' }, async () => ({ text: JSON.stringify({ sessionId, runtimeId, mode, activeTurnId, connected: Boolean(config), overflow }) }));
  on('command.run', { command: 'bridge-human' }, async () => {
    mode = 'human';
    if (pending && !pending.turnId) pending.cancelled = true;
    emit('mode', { mode });
    return { text: 'Human control. New bridge prompts are paused; running work is not cancelled.' };
  });
  on('command.run', { command: 'bridge-auto' }, async () => {
    if (ended || overflow) return { text: 'Bridge unavailable; restart this session before enabling automation.' };
    mode = 'automation';
    emit('mode', { mode });
    return { text: 'Automation may submit prompts when this session is idle.' };
  });

  on('prompt.submit', async ($, e, next) => {
    if (config && ['composer', 'bridge'].includes(e.origin?.kind)) {
      mode = 'human';
      if (pending && !pending.turnId) pending.cancelled = true;
      emit('mode', { mode });
    }
    return next(e);
  });
  on('turn.start', async ($, e, next) => {
    if (e.agentId) return next(e);
    activeTurnId = e.turnId;
    const matched = pending && e.text.includes(pending.marker);
    if (matched) pending.turnId = e.turnId;
    emit('turn.start', { turnId: e.turnId, text: e.text, requestId: matched ? pending.id : undefined });
    if (matched && (pending.cancelled || mode === 'human')) {
      // Abort through the runtime, never by synthesizing an Escape key.
      await $.turn.abort({ turnId: e.turnId });
    }
    return next(e);
  });
  on('turn.step', async function* ($, e, next) {
    const stream = next(e);
    let part = await stream.next();
    while (!part.done) {
      const chunk = part.value;
      if (!e.agentId && (chunk.kind === 'text' || chunk.kind === 'thinking')) {
        emit(chunk.kind === 'text' ? 'text' : 'thought', { turnId: e.turnId, requestId: requestFor(e.turnId), text: chunk.text });
      }
      yield chunk;
      part = await stream.next();
    }
    return part.value;
  });
  on('tool.call', async ($, e, next) => {
    if (e.agentId) return next(e);
    const data = { turnId: activeTurnId, requestId: requestFor(activeTurnId), toolCallId: e.tool_use_id, title: e.tool, input: e };
    emit('tool.start', data);
    try {
      const result = await next(e);
      emit('tool.end', { ...data, result, isError: Boolean(result.deny || result.isError) });
      return result;
    } catch (error) {
      emit('tool.end', { ...data, result: String(error), isError: true });
      throw error;
    }
  });
  on('classic.PermissionRequest', async ($, e, next) => {
    emit('needs.input', { kind: 'permission', tool: e.tool_name, input: e.tool_input, requestId: pending?.id });
    return next(e);
  });
  on('turn.complete', async ($, e, next) => {
    if (!e.agentId) {
      emit('turn.complete', { ...e, requestId: requestFor(e.turnId) });
      if (pending?.turnId === e.turnId) pending = null;
      if (activeTurnId === e.turnId) activeTurnId = null;
      if (e.isAborted) {
        mode = 'human';
        emit('mode', { mode });
      }
    }
    return next(e);
  });
  on('session.attach', async ($, e, next) => {
    emit('session.attach', e);
    return next(e);
  });
  on('session.detach', async ($, e, next) => {
    emit('session.detach', e);
    return next(e);
  });
  on('session.end', async ($, e, next) => {
    ended = true;
    mode = 'human';
    timer?.cancel();
    emit('session.end', { reason: e.reason });
    // Session identity changes (/clear, /resume) require a fresh mod load.
    // Do not keep controlling a different conversation under the old ID.
    pump($).catch(() => {});
    return next(e);
  });
}
