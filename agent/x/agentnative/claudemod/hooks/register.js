// This module runs inside native Claude, not in Node or the Agent SDK.
let config;
let descriptorFile;
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
let pumping;
let ended = false;
let transportClosed = false;
let provenanceGeneration = 0;
const competingSubmissions = new Set();
const permissions = new Map();
let overflow = false;
let timer;
let bufferedBytes = 0;

function failStream(reason, detail) {
  overflow = true;
  mode = 'human';
  pending?.receiptDeadline?.cancel();
  events = [{ seq: ++sequence, type: 'overflow', data: { reason, ...(detail ? { detail } : {}) } }];
  bufferedBytes = 0;
}

function emit(type, data) {
  if (!config || overflow) return;
  const event = { seq: ++sequence, type, data };
  // Bound serialized bytes, not just event count. Never silently clip output.
  const bytes = new TextEncoder().encode(JSON.stringify(event)).length;
  if (events.length >= 4000 || bytes > 128 * 1024 - 512 || bufferedBytes + bytes > 4 * 1024 * 1024) {
    failStream('Mod event buffer exhausted; attach to inspect the session');
    return;
  }
  bufferedBytes += bytes;
  events.push(event);
}

function requestFor(turnId) {
  // Candidate events stay buffered until their native receipt is validated.
  return pending && (pending.turnId === turnId || pending.candidate?.turnId === turnId) ? pending.id : undefined;
}

function isOwnedSubmission(event, request) {
  // asUser omits model-facing attribution, not the native receipt's origin.
  return event.origin?.kind === 'plugin' && event.origin.name === 'workspace-bridge' && event.origin.asUser === true && event.text === request.text;
}

function releaseCompetingSubmission(submission) {
  if (submission.accepted && submission.completed) competingSubmissions.delete(submission);
}

async function submit($, command) {
  if (ended || overflow || mode !== 'automation' || activeTurnId || pending) {
    emit('prompt.rejected', { requestId: command.id, reason: 'Session is not idle under automation control' });
    return;
  }
  if (competingSubmissions.size) {
    emit('prompt.rejected', { requestId: command.id, reason: 'Observed native submissions have not been unambiguously completed' });
    return;
  }
  const request = { id: command.id, text: command.text, generation: provenanceGeneration, turnId: null, cancelled: false };
  // Reserve synchronously, before a host call can yield to cancel or takeover.
  pending = request;
  try {
    const draft = await $.prompt.read();
    if (draft.text || mode !== 'automation' || activeTurnId || ended || request.cancelled || request.generation !== provenanceGeneration) {
      emit('prompt.rejected', { requestId: command.id, reason: request.cancelled ? 'Cancelled before submission' : 'Native prompt draft or session state changed before submission', ...(request.cancelled ? { cancelled: true } : {}) });
      if (pending === request) pending = null;
      return;
    }
    // Native suppresses this plugin's prompt.submit hook on reentry. The call's
    // receipt proves ownership without adding tracking text to model context.
    request.dispatched = true;
    request.submitting = true;
    const result = await $.prompt.submit({ text: request.text, asUser: true });
    request.submitting = false;
    request.receiptDeadline?.cancel();
    if (overflow) return;
    if (result.drop && !request.candidate) {
      emit('prompt.rejected', { requestId: command.id, reason: result.drop });
      if (pending === request) pending = null;
    } else {
      const changed = pending !== request || provenanceGeneration !== request.generation;
      const owned = !result.drop && isOwnedSubmission(result, request);
      const matches = !request.candidate || (owned && request.candidate.text === result.text);
      if (changed || !owned || !matches) {
        failStream('Native prompt provenance is ambiguous; inspect the session before submitting more work',
          changed ? 'competing-submission-or-root' : !owned ? 'invalid-native-receipt' : 'root-text-mismatch');
        return;
      }
      request.heldFrom = undefined;
      if (request.candidate) request.turnId = request.candidate.turnId;
      else request.source = result;
      emit('prompt.accepted', { requestId: command.id });
      if (request.candidate?.completed) pending = null;
      else if (request.turnId && activeTurnId === request.turnId && request.cancelled) {
        await $.turn.abort({ turnId: request.turnId });
      }
    }
  } catch (error) {
    if (overflow) return;
    if (request.candidate) {
      failStream('Native prompt outcome is unknown; inspect the session before submitting more work', 'receipt-or-cancellation-failed');
    } else {
      emit('prompt.rejected', { requestId: command.id, reason: String(error.message || error) });
      if (pending === request && !request.turnId) pending = null;
    }
  } finally {
    request.submitting = false;
    request.receiptDeadline?.cancel();
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
    if (ended || overflow || !['human', 'automation'].includes(command.mode)) {
      emit('command.error', { commandId: command.id, reason: 'Bridge session is closed or mode is invalid' });
      return;
    }
    mode = command.mode;
    // A queued bridge prompt must not start after human takeover.
    if (mode === 'human' && pending && !pending.turnId && !pending.candidate) pending.cancelled = true;
    emit('mode', { mode });
    emit('command.done', { commandId: command.id });
  }
}

async function reloadDescriptor($) {
  const next = JSON.parse(await $.fs.read(descriptorFile));
  const url = new URL(next.url);
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.username || url.password || url.search || url.hash || url.pathname !== '/' || typeof next.token !== 'string' || !next.token) {
    throw new Error('Bridge descriptor must use authenticated loopback HTTP');
  }
  config = next;
}

function pump($) {
  if (transportClosed || !descriptorFile) return Promise.resolve();
  if (!pumping) pumping = sendBatch($).finally(() => { pumping = undefined; });
  return pumping;
}

async function sendBatch($) {
  const batch = [];
  let bytes = 0;
  for (const event of events.slice(0, 128)) {
    if (!overflow && pending?.heldFrom !== undefined && event.seq >= pending.heldFrom) break;
    bytes += new TextEncoder().encode(JSON.stringify(event)).length;
    if (bytes > 1024 * 1024) break;
    batch.push(event);
  }
  // Agent-only restarts rotate the listener and token, but must not replace the
  // runtime identity, event sequence, pending turn, or deduplication receipts.
  await reloadDescriptor($);
  if (transportClosed) return;
  const reply = await $.http.fetch(config.url + '/v1/runtime', {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + config.token, 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId, runtimeId, launchId, version, cwd, mode, activeTurnId, ended, events: batch }),
  });
  if (transportClosed || !reply.ok) return;
  const body = JSON.parse(reply.text);
  const maxSent = batch.length ? batch[batch.length - 1].seq : 0;
  if (Number.isInteger(body.ack) && body.ack >= 0 && body.ack <= maxSent) {
    events = events.filter(event => event.seq > body.ack);
    bufferedBytes = events.reduce((total, event) => total + new TextEncoder().encode(JSON.stringify(event)).length, 0);
  }
  for (const command of body.commands || []) {
    try { await handle($, command); }
    catch (error) { emit('command.error', { commandId: command.id, reason: String(error.message || error) }); }
  }
}

async function drain($) {
  if (!descriptorFile) return;
  let deadline;
  const expired = new Promise(resolve => {
    deadline = $.clock.after(5000, () => { transportClosed = true; resolve(); });
  });
  try {
    await Promise.race([expired, (async () => {
      // Join a poll already in progress, then send every remaining batch. An
      // unavailable host must not hold native shutdown open indefinitely.
      if (pumping) await pumping.catch(() => {});
      for (let attempt = 0; !transportClosed && events.length && attempt < 64; attempt++) {
        const first = events[0].seq;
        await pump($).catch(() => {});
        if (events[0]?.seq === first) await $.clock.sleep(100);
      }
    })()]);
    if (events.length) $.ui.log('Workspace bridge final events were not acknowledged before shutdown.');
  } finally {
    transportClosed = true;
    deadline.cancel();
  }
}

function resolvePermissions(matches) {
  let resolved;
  for (const [key, permission] of permissions) {
    if (matches(permission)) {
      permissions.delete(key);
      resolved = permission;
    }
  }
  if (resolved) emit('needs.input', { ...resolved, pending: permissions.size > 0 });
}

async function connect($, e) {
  descriptorFile = await $.env.get('WORKSPACE_BRIDGE_FILE');
  if (!descriptorFile) return;
  await reloadDescriptor($);
  sessionId = await $.session.id();
  runtimeId = sessionId + ':' + Math.random().toString(36).slice(2);
  launchId = await $.env.get('WORKSPACE_BRIDGE_LAUNCH_ID');
  version = await $.session.version();
  cwd = e.cwd;
  mode = (await $.env.get('WORKSPACE_BRIDGE_MODE')) === 'automation' ? 'automation' : 'human';
  emit('session.start', { sessionId, runtimeId, cwd, version });
  timer = $.clock.every(200, () => pump($).catch(() => {}));
  await pump($);
}

/** Register the native-session control and observation hooks. */
export function register(on) {
  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'bridge-status', description: 'Show workspace bridge state', immediate: true });
    await $.command.register({ name: 'bridge-human', description: 'Pause new bridge prompts; current work continues', immediate: true });
    await $.command.register({ name: 'bridge-auto', description: 'Allow bridge prompts when Claude is idle', immediate: true });
    try { await connect($, e); }
    catch { $.ui.log('Workspace bridge unavailable; inspect the private agent descriptor.'); }
    return next(e);
  });

  on('command.run', { command: 'bridge-status' }, async () => ({ text: JSON.stringify({ sessionId, runtimeId, mode, activeTurnId, connected: Boolean(config), overflow }) }));
  on('command.run', { command: 'bridge-human' }, async () => {
    mode = 'human';
    if (pending && !pending.turnId && !pending.candidate) pending.cancelled = true;
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
    // Native suppresses our own submission hook, so any observed submission is
    // competing, even with identical text and origin. A late receipt cannot rearm it.
    provenanceGeneration++;
    if (pending) pending.source = undefined;
    const human = ['composer', 'bridge'].includes(e.origin?.kind);
    const submission = {
      text: e.text,
      literal: human || (e.origin?.kind === 'plugin' && e.origin.asUser === true),
      turnId: null, accepted: false, completed: false,
    };
    if (config && !overflow) {
      if (competingSubmissions.size >= 64) {
        failStream('Native submission tracking exhausted; inspect the session before submitting more work');
      } else competingSubmissions.add(submission);
    }
    if (config && human) {
      mode = 'human';
      if (pending && !pending.turnId && !pending.candidate) pending.cancelled = true;
      emit('mode', { mode });
    }
    // next() can drop or throw after core entry, so neither releases the fence.
    // Even a successful result needs a completed root. Do not reconstruct native
    // attribution wrappers or guess which of several identical submissions ran.
    const result = await next(e);
    submission.accepted = result?.drop === undefined && result?.text === submission.text;
    releaseCompetingSubmission(submission);
    return result;
  });
  on('turn.start', async ($, e, next) => {
    if (e.agentId) return next(e);
    activeTurnId = e.turnId;
    if (!pending || (!pending.dispatched && pending.generation !== provenanceGeneration)) {
      const matches = [...competingSubmissions].filter(submission =>
        !submission.turnId && submission.literal && submission.text === e.text);
      if (matches.length === 1) matches[0].turnId = e.turnId;
    }
    // Root hooks and submit results cross host/worker RPC independently. Hold a
    // matching candidate's events, but grant no control until the receipt arrives.
    // Any competing submission or second root still invalidates that receipt.
    const matched = pending && !pending.turnId && pending.source && e.text === pending.source.text;
    const candidate = !overflow && pending?.submitting && !pending.candidate && !pending.turnId &&
      pending.generation === provenanceGeneration && e.text === pending.text;
    if (candidate) {
      pending.candidate = { turnId: e.turnId, text: e.text, completed: false };
      pending.heldFrom = sequence + 1;
      pending.receiptDeadline = $.clock.after(5000, () => {
        failStream('Native prompt receipt did not arrive; inspect the session before submitting more work', 'receipt-timeout');
      });
    } else {
      provenanceGeneration++;
      if (pending) pending.source = undefined;
    }
    if (matched) pending.turnId = e.turnId;
    emit('turn.start', { turnId: e.turnId, text: e.text, requestId: matched || candidate ? pending.id : undefined });
    if (matched && (pending.cancelled || mode === 'human')) {
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
    } finally {
      if (e.tool_use_id) resolvePermissions(permission => permission.toolCallId === e.tool_use_id);
    }
  });
  on('classic.PermissionRequest', async ($, e, next) => {
    if (config && (!e.session_id || e.session_id === sessionId)) {
      const permission = { kind: 'permission', tool: e.tool_name, input: e.tool_input, toolCallId: e.tool_use_id, turnId: activeTurnId, requestId: requestFor(activeTurnId) };
      permissions.set(e.tool_use_id || Symbol(), permission);
      emit('needs.input', { ...permission, pending: true });
    }
    // Returning from PermissionRequest does not mean the user has answered it.
    return next(e);
  });
  on('turn.complete', async ($, e, next) => {
    if (!e.agentId) {
      resolvePermissions(permission => permission.turnId === e.turnId);
      emit('turn.complete', { ...e, requestId: requestFor(e.turnId) });
      if (pending?.turnId === e.turnId) pending = null;
      else if (pending?.candidate?.turnId === e.turnId) pending.candidate.completed = true;
      if (activeTurnId === e.turnId) activeTurnId = null;
      for (const submission of competingSubmissions) {
        if (submission.turnId !== e.turnId) continue;
        submission.completed = true;
        releaseCompetingSubmission(submission);
      }
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
    if (pending?.heldFrom !== undefined) {
      failStream('Native session ended before prompt receipt; inspect the session', 'ended-before-receipt');
    }
    resolvePermissions(() => true);
    emit('session.end', { reason: e.reason });
    // Identity-changing native commands require a fresh mod load. Do not release
    // the native shutdown hook until the final event is ACKed or the drain ends.
    await drain($).catch(() => {});
    return next(e);
  });
}
