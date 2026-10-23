import { randomUUID } from "node:crypto";
import { isAbsolute, resolve } from "node:path";
import { Readable, Writable } from "node:stream";
import { MCP_BLOCKED_REASON, NATIVE_BLOCKED_REASON, REGISTRATION_PENDING_REASON } from "./native-discovery.mjs";
import {
  agent,
  methods,
  ndJsonStream,
  PROTOCOL_VERSION,
  RequestError,
} from "@agentclientprotocol/sdk";

/**
 * Authenticated HTTP access to the persistent local broker. Implementations
 * reject non-success responses, honor the signal, and never retry mutations.
 * HTTP errors may carry a numeric status to distinguish rejection from an
 * ambiguous transport failure. Requests must remain bounded when given a signal.
 * @typedef {object} BridgeClient
 * @property {(method: string, path: string, body?: unknown, signal?: AbortSignal) => Promise<any>} request
 */

const META = "claude-acp-bridge";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const MODES = [
  {
    id: "human",
    name: "Human",
    description: "Native Claude controls prompts and permissions.",
  },
  {
    id: "automation",
    name: "Automation",
    description: "ACP can submit prompts. Permissions still require native attach.",
  },
];
const LIMITATIONS = {
  history: "retained-broker-events-only",
  fullHistory: false,
  permissions: "native-attach",
  promptContent: ["text"],
  mcpServers: false,
};
const TURN_EVENTS = new Set([
  "turn.start", "text", "thought", "tool.start", "tool.end", "turn.complete",
]);

function bridgeError(message, data) {
  return new RequestError(-32000, message, data);
}

function invalid(message) {
  return new RequestError(-32602, message);
}

function sessionPath(sessionId) {
  return `/v1/sessions/${encodeURIComponent(sessionId)}`;
}

function checkSetup(params) {
  if (!isAbsolute(params.cwd)) throw invalid("cwd must be an absolute path");
  if (params.mcpServers?.length) throw invalid("MCP server injection is not supported");
  if (params.additionalDirectories?.length) {
    throw invalid("Additional directories are not supported");
  }
}

function checkSession(session) {
  if (!session || !UUID.test(session.sessionId) ||
      typeof session.cwd !== "string" || !isAbsolute(session.cwd) ||
      !MODES.some(({ id }) => id === session.mode)) {
    throw bridgeError("Broker returned an invalid native session");
  }
}

function sessionStatus(session) {
  return {
    startedAt: session.startedAt,
    ended: session.ended,
    archived: session.archived === true,
    resumable: session.resumable === true,
    resumePending: session.resumePending === true,
    fault: session.fault,
    stale: session.stale,
    registrationPending: session.registrationPending === true,
    blockedReason: session.registrationPending === true && [MCP_BLOCKED_REASON, NATIVE_BLOCKED_REASON].includes(session.blockedReason) ? session.blockedReason : undefined,
    activeTurnId: typeof session.activeTurnId === "string" ? session.activeTurnId : undefined,
    lastError: session.lastError,
  };
}

function sessionDetails(state) {
  return {
    modes: { currentModeId: state.mode, availableModes: MODES },
    _meta: {
      [META]: {
        ...LIMITATIONS,
        ...sessionStatus(state),
        nativeSessionId: state.sessionId,
        runtimeId: state.runtimeId,
        attachCommand: state.attachCommand,
        cursor: state.cursor,
      },
    },
  };
}

/**
 * Creates ACP method handlers backed only by the persistent broker.
 *
 * serveACP registers these handlers on the SDK's agent(). The connection can be
 * an SDK AgentConnection or a sessionUpdate connection with an optional signal.
 * Closing the transport aborts local HTTP work, never the native session.
 *
 * @param {import("@agentclientprotocol/sdk").AgentConnection | {sessionUpdate: (notification: object) => Promise<void>, signal?: AbortSignal, closed?: Promise<void>}} connection
 * @param {BridgeClient} client
 * @returns {object} ACP request and notification handlers.
 */
export function createAgent(connection, client) {
  const sessions = new Map();
  const lifetime = new AbortController();
  const send = connection.sessionUpdate
    ? (notification) => connection.sessionUpdate(notification)
    : (notification) => connection.client.notify(methods.client.session.update, notification);

  function fail(state, error) {
    if (state.error) return;
    state.error = error instanceof RequestError ? error : bridgeError(String(error));
    state.controller.abort(state.error);
    state.pending?.reject(state.error);
  }

  function disconnect() {
    lifetime.abort();
    for (const state of sessions.values()) {
      fail(state, bridgeError("ACP transport closed; native session continues"));
    }
  }

  if (connection.signal?.aborted) disconnect();
  else connection.signal?.addEventListener("abort", disconnect, { once: true });
  // Older connection factories expose only a closed promise.
  if (!connection.signal && connection.closed) {
    void connection.closed.then(disconnect, disconnect);
  }

  async function request(method, path, body, signal = lifetime.signal) {
    try {
      return await client.request(method, path, body, signal);
    } catch (error) {
      throw bridgeError(`Broker ${method} ${path}: ${error.message ?? error}`,
        Number.isInteger(error.status) ? { status: error.status } : undefined);
    }
  }

  function getSession(sessionId) {
    const state = sessions.get(sessionId);
    if (!state) throw invalid("Session is not attached to this ACP connection");
    if (state.error) throw state.error;
    if (!state.ready) throw invalid("Session attachment is still in progress");
    return state;
  }

  async function update(state, value, event) {
    if (state.error) throw state.error;
    await send({
      sessionId: state.sessionId,
      update: value,
      ...(event ? { _meta: { [META]: {
        seq: event.seq,
        turnId: event.data.turnId,
        requestId: event.data.requestId,
      } } } : {}),
    });
  }

  function maybeCancel(state, pending) {
    if (!pending.cancelRequested || !pending.submitted || pending.cancelSent || pending.settled) return;
    pending.cancelSent = true;
    void request("POST", `${sessionPath(state.sessionId)}/cancel`, {
      requestId: pending.requestId,
    }, state.controller.signal).catch((error) => {
      if (state.pending === pending && !pending.settled) fail(state, error);
    });
  }

  async function consume(state, event, replay) {
    const { type, data } = event;
    if (!data || typeof data !== "object") throw bridgeError("Invalid broker event data");
    // Resuming retains earlier conversation text, not earlier runtime control.
    const historical = (replay && state.archived) || event.seq <= (state.runtimeStartCursor ?? 0);
    if (historical && !TURN_EVENTS.has(type)) return;
    replay ||= historical;
    if (type === "session.resuming") {
      if (data.sessionId !== state.sessionId) throw bridgeError("Broker resume changed the session UUID");
      if (event.seq <= (state.runtimeStartCursor ?? 0)) return;
      Object.assign(state, { archived: false, ended: false, fault: false, mode: "human",
        resumable: false, resumePending: true, registrationPending: true,
        runtimeStartCursor: event.seq, blockedReason: undefined });
      state.observedTurns.clear();
      await update(state, { sessionUpdate: "session_info_update", _meta: { [META]: sessionStatus(state) } }, event);
      await update(state, { sessionUpdate: "current_mode_update", currentModeId: "human" }, event);
      return;
    }
    if (type === "session.resume_failed") {
      if (data.sessionId !== state.sessionId || typeof data.reason !== "string" || typeof data.retryable !== "boolean") {
        throw bridgeError("Broker returned an invalid resume failure");
      }
      if (data.retryable) Object.assign(state, { archived: true, ended: true, resumePending: false,
        registrationPending: false, resumable: true, mode: "human" });
      state.lastError = { message: data.reason };
      await update(state, { sessionUpdate: "session_info_update", _meta: { [META]: sessionStatus(state) } }, event);
      return;
    }
    if (type === "runtime.resumed") {
      if (data.sessionId !== state.sessionId || typeof data.runtimeId !== "string" || !data.runtimeId ||
          !MODES.some(({ id }) => id === data.mode) ||
          (!state.resumePending && state.runtimeId !== data.runtimeId)) {
        throw bridgeError("Broker returned an invalid resumed runtime");
      }
      if (state.resumePending) {
        Object.assign(state, { runtimeId: data.runtimeId, mode: data.mode, lastSeen: data.lastSeen,
          archived: false, ended: false, fault: false, registrationPending: false, resumePending: false,
          blockedReason: undefined });
        state.observedTurns.clear();
        await update(state, { sessionUpdate: "session_info_update", _meta: {
          [META]: { ...sessionStatus(state), runtimeId: state.runtimeId, lastSeen: state.lastSeen },
        } }, event);
        await update(state, { sessionUpdate: "current_mode_update", currentModeId: state.mode }, event);
      }
      return;
    }
    if (type === "runtime.replaced" || type === "overflow") {
      throw bridgeError(`Broker event ${type}; reattach explicitly`, { sessionId: state.sessionId });
    }
    if (type === "session.end") {
      throw bridgeError(`Native session ended: ${data.reason ?? "unknown reason"}`);
    }
    if (type === "session.pending") {
      // A load snapshot can already contain the registered runtime while replay
      // still includes earlier discovery notices. Never regress that snapshot.
      if (state.registrationPending) {
        state.blockedReason = data.blockedReason;
        await update(state, { sessionUpdate: "session_info_update", _meta: {
          [META]: sessionStatus(state),
        } }, event);
      }
      return;
    }
    if (type === "runtime.registered") {
      if (data.sessionId !== state.sessionId || typeof data.runtimeId !== "string" || !data.runtimeId ||
          !MODES.some(({ id }) => id === data.mode) ||
          (state.runtimeId !== data.runtimeId && !(state.registrationPending && state.runtimeId == null))) {
        throw bridgeError("Broker session identity changed during registration");
      }
      const wasPending = state.registrationPending;
      Object.assign(state, { registrationPending: false, blockedReason: undefined, runtimeId: data.runtimeId });
      if (wasPending) {
        state.mode = data.mode;
        state.lastSeen = data.lastSeen;
        await update(state, { sessionUpdate: "session_info_update", _meta: {
          [META]: { ...sessionStatus(state), runtimeId: state.runtimeId, lastSeen: state.lastSeen },
        } }, event);
        await update(state, { sessionUpdate: "current_mode_update", currentModeId: state.mode }, event);
      }
      return;
    }
    if (type === "session.start") {
      if (data.sessionId !== state.sessionId || data.runtimeId !== state.runtimeId) {
        throw bridgeError("Broker session identity changed during attachment");
      }
      return;
    }
    // Control acknowledgements are consumed by the broker's HTTP handlers.
    if (["command.done", "command.error", "session.attach", "session.detach"].includes(type)) return;
    if (type === "mode") {
      if (!MODES.some(({ id }) => id === data.mode)) throw bridgeError("Invalid broker mode");
      state.mode = data.mode;
      await update(state, { sessionUpdate: "current_mode_update", currentModeId: data.mode }, event);
      return;
    }
    const pending = state.pending;
    if (type === "needs.input") {
      if (pending && data.requestId !== pending.requestId) return;
      await update(state, {
        sessionUpdate: "session_info_update",
        _meta: { [META]: { pendingInput: data, permissions: "native-attach" } },
      }, event);
      return;
    }
    if (type === "prompt.accepted" || type === "prompt.rejected") {
      if (pending && data.requestId === pending.requestId && !pending.settled) {
        if (type === "prompt.rejected") {
          if (data.cancelled === true) pending.resolve({ stopReason: "cancelled" });
          else pending.reject(bridgeError(`Native prompt rejected: ${data.reason ?? "unknown reason"}`));
        } else {
          pending.submitted = true;
          maybeCancel(state, pending);
        }
      }
      return;
    }
    if (!TURN_EVENTS.has(type)) throw bridgeError(`Unsupported broker event: ${type}`);
    if (typeof data.turnId !== "string" || !data.turnId) throw bridgeError("Broker turn event has no turnId");

    let turn;
    let ownsPrompt = false;
    if (!replay && pending && !pending.settled &&
        (data.requestId == null || data.requestId === pending.requestId)) {
      if (!pending.turnId && data.requestId === pending.requestId) pending.turnId = data.turnId;
      // A subagent can share the request ID, but must never finish the root turn.
      if (pending.turnId === data.turnId) {
        pending.submitted = true;
        maybeCancel(state, pending);
        turn = pending;
        ownsPrompt = true;
      }
    }
    if (!turn) {
      turn = state.observedTurns.get(data.turnId);
      if (!turn) {
        // Observe a native turn independently of any request owned by this ACP
        // connection. Replayed, still-running turns continue through live polling.
        if (!replay && (pending || type !== "turn.start")) return;
        turn = { streamed: false };
        state.observedTurns.set(data.turnId, turn);
      }
    }

    const textUpdate = (sessionUpdate, text) => update(state, {
      sessionUpdate,
      content: { type: "text", text },
    }, event);
    if (type === "turn.start" || type === "text" || type === "thought") {
      if (typeof data.text !== "string") throw bridgeError(`Invalid ${type} text`);
      if (type === "turn.start") {
        if (!ownsPrompt) await textUpdate("user_message_chunk", data.text);
      } else if (type === "text") {
        if (data.text) turn.streamed = true;
        await textUpdate("agent_message_chunk", data.text);
      } else {
        await textUpdate("agent_thought_chunk", data.text);
      }
      return;
    }
    if (type === "tool.start" || type === "tool.end") {
      if (typeof data.toolCallId !== "string" || !data.toolCallId) {
        throw bridgeError("Broker tool event has no toolCallId");
      }
      if (type === "tool.start") {
        if (typeof data.title !== "string") throw bridgeError("Broker tool event has no title");
        await update(state, {
          sessionUpdate: "tool_call",
          toolCallId: data.toolCallId,
          title: data.title,
          kind: "other",
          status: "pending",
          rawInput: data.input,
          _meta: { [META]: { permissions: "native-attach" } },
        }, event);
      } else {
        await update(state, {
          sessionUpdate: "tool_call_update",
          toolCallId: data.toolCallId,
          status: data.isError ? "failed" : "completed",
          rawOutput: data.result,
          content: [{ type: "content", content: {
            type: "text",
            text: typeof data.result === "string" ? data.result : JSON.stringify(data.result ?? null),
          } }],
        }, event);
      }
      return;
    }
    if (!["answer", "aborted", "refusal", "error"].includes(data.reason)) {
      throw bridgeError("Invalid native turn completion reason");
    }
    if (typeof data.answer !== "string") throw bridgeError("Invalid native turn answer");
    if (!turn.streamed && data.answer) await textUpdate("agent_message_chunk", data.answer);
    if (!ownsPrompt) {
      state.observedTurns.delete(data.turnId);
    } else if (data.reason === "error") {
      pending.reject(bridgeError("Native turn failed", { turnId: data.turnId, answer: data.answer }));
    } else {
      pending.resolve({
        stopReason: data.isAborted || data.reason === "aborted" ? "cancelled"
          : data.reason === "refusal" ? "refusal" : "end_turn",
      });
    }
  }

  async function readEvents(state, wait, replay = false, through) {
    const batch = await request("GET", `${sessionPath(state.sessionId)}/events?after=${state.cursor}&wait=${wait ? 1 : 0}`,
      undefined, state.controller.signal);
    if (!batch || !Array.isArray(batch.events) || !Number.isSafeInteger(batch.cursor)) {
      throw bridgeError("Invalid broker event batch");
    }
    let cursor = state.cursor;
    // Validate the whole batch before delivering a completion from a broken stream.
    for (const event of batch.events) {
      if (!Number.isSafeInteger(event.seq) || event.seq !== cursor + 1) {
        throw bridgeError(`Broker event gap: expected ${cursor + 1}, received ${event.seq}`);
      }
      cursor = event.seq;
    }
    if (cursor !== batch.cursor) throw bridgeError("Broker event cursor gap");
    for (const event of batch.events) {
      // An archive snapshot cannot classify controls appended by a later resume.
      // Leave those events unread until a fresh snapshot authorizes following it.
      if (through !== undefined && event.seq > through) break;
      await consume(state, event, replay);
      state.cursor = event.seq;
    }
  }

  async function poll(state) {
    if (state.polling) return;
    state.polling = true;
    try {
      while (!state.archived && !state.controller.signal.aborted) {
        state.reading = readEvents(state, true);
        await state.reading;
      }
    } catch (error) {
      if (state.error) return;
      fail(state, error);
      // Surface idle failures as well as rejecting an active prompt. No answer is
      // fabricated, and clients can inspect the status without notice support.
      await send({ sessionId: state.sessionId, update: {
        sessionUpdate: "session_info_update",
        _meta: { [META]: { error: state.error.message, cursor: state.cursor } },
      } }).catch(() => {});
    } finally { state.polling = false; }
  }

  async function followResume(state, session) {
    if (!state.archived || session.archived || session.ended || session.fault) return;
    checkSession(session);
    if (session.sessionId !== state.sessionId || session.cwd !== state.cwd ||
        !Number.isSafeInteger(session.runtimeStartCursor) || session.runtimeStartCursor < state.cursor) {
      throw bridgeError("Broker returned an invalid resume boundary");
    }
    Object.assign(state, {
      ...sessionStatus(session), mode: session.mode, runtimeId: session.runtimeId,
      runtimeStartCursor: session.runtimeStartCursor, lastSeen: session.lastSeen,
    });
    state.observedTurns.clear();
    await update(state, { sessionUpdate: "session_info_update", _meta: {
      [META]: { ...sessionStatus(state), runtimeId: state.runtimeId, lastSeen: state.lastSeen },
    } });
    await update(state, { sessionUpdate: "current_mode_update", currentModeId: state.mode });
    void poll(state);
  }

  async function attach(session, cwd, replay) {
    checkSession(session);
    if (resolve(session.cwd) !== resolve(cwd)) throw invalid("Native session cwd does not match requested cwd");
    if (sessions.has(session.sessionId)) throw invalid("Session is already attached to this ACP connection");
    lifetime.signal.throwIfAborted();
    if ((session.ended || session.fault) && session.archived !== true) throw bridgeError("Native session is closed or faulted; inspect it before continuing");
    const cursor = session.historyStartCursor ?? 0;
    if (!Number.isSafeInteger(cursor) || cursor < 0) throw bridgeError("Invalid runtime history boundary");
    const state = {
      ...session,
      controller: new AbortController(),
      cursor,
      ready: false,
      pending: null,
      error: null,
      observedTurns: new Map(),
    };
    sessions.set(session.sessionId, state);
    try {
      // Archives replay the retained broker log, not complete native history.
      // A non-waiting snapshot also makes loading an empty session finite.
      await readEvents(state, false, replay, state.archived ? session.cursor : undefined);
      state.ready = true;
      if (!state.archived) void poll(state);
      return state;
    } catch (error) {
      fail(state, error);
      throw state.error;
    }
  }

  return {
    async initialize() {
      return {
        protocolVersion: PROTOCOL_VERSION,
        agentInfo: { name: "claude-acp-bridge", title: "Claude Code", version: "0.1.0" },
        agentCapabilities: {
          loadSession: true,
          promptCapabilities: { image: false, audio: false, embeddedContext: false },
          mcpCapabilities: { http: false, sse: false },
          sessionCapabilities: { list: {} },
        },
        authMethods: [],
        _meta: { [META]: LIMITATIONS },
      };
    },

    async newSession(params) {
      checkSetup(params);
      const session = await request("POST", "/v1/sessions", { cwd: params.cwd });
      const state = await attach(session, params.cwd, false);
      return { sessionId: state.sessionId, ...sessionDetails(state) };
    },

    async loadSession(params) {
      checkSetup(params);
      if (!UUID.test(params.sessionId)) throw invalid("Invalid native session UUID");
      const session = await request("GET", sessionPath(params.sessionId));
      if (session?.sessionId !== params.sessionId) throw bridgeError("Broker returned a different session UUID");
      if (sessions.has(params.sessionId)) {
        const state = getSession(params.sessionId);
        checkSession(session);
        if (resolve(session.cwd) !== resolve(params.cwd)) throw invalid("Native session cwd does not match requested cwd");
        if ((session.ended || session.fault) && session.archived !== true) throw bridgeError("Native session is closed or faulted; inspect it before continuing");
        // Refresh an existing attachment without replaying or starting a second
        // poller. Wait for its stream to reach the snapshot before enabling work.
        const cursor = session.cursor ?? state.cursor;
        if (!Number.isSafeInteger(cursor) || cursor < 0) throw bridgeError("Invalid broker event cursor");
        await followResume(state, session);
        if (state.archived) Object.assign(state, sessionStatus(session));
        while (state.cursor < cursor) {
          if (state.archived) await readEvents(state, false, true, cursor);
          else await state.reading;
          if (state.error) throw state.error;
        }
        if (session.runtimeId !== state.runtimeId && !session.registrationPending) throw bridgeError("Broker session identity changed during attachment");
        return sessionDetails({ ...session, cursor: state.cursor, mode: state.mode, runtimeId: state.runtimeId,
          registrationPending: state.registrationPending, blockedReason: state.blockedReason });
      }
      return sessionDetails(await attach(session, params.cwd, true));
    },

    async listSessions(params = {}) {
      if (params.cursor != null) throw invalid("Session list pagination is not supported");
      const result = await request("GET", "/v1/sessions");
      if (!Array.isArray(result?.sessions)) throw bridgeError("Invalid broker session list");
      for (const session of result.sessions) {
        checkSession(session);
        const state = sessions.get(session.sessionId);
        if (state && !state.error) await followResume(state, session);
      }
      return { sessions: result.sessions.filter((session) => {
        checkSession(session);
        return !params.cwd || resolve(session.cwd) === resolve(params.cwd);
      }).map((session) => ({
        sessionId: session.sessionId,
        cwd: session.cwd,
        ...(session.title ? { title: session.title } : {}),
        _meta: { [META]: { ...LIMITATIONS, ...sessionStatus(session), mode: session.mode, runtimeId: session.runtimeId, lastSeen: session.lastSeen } },
      })) };
    },

    async setSessionMode({ sessionId, modeId }) {
      const state = getSession(sessionId);
      if (!MODES.some(({ id }) => id === modeId)) throw invalid("Unsupported native session mode");
      if (state.archived) {
        if (modeId !== "human" || !state.resumable) throw bridgeError("Saved session history is read-only; Join is required to resume a supported native session");
        const resumed = await request("POST", `${sessionPath(sessionId)}/resume`, {}, state.controller.signal);
        await followResume(state, resumed);
        if (state.archived) throw bridgeError("Native resume was not confirmed; inspect the saved session before retrying");
        return {};
      }
      if (state.registrationPending && modeId !== "human") throw bridgeError(REGISTRATION_PENDING_REASON);
      await request("POST", `${sessionPath(sessionId)}/mode`, { mode: modeId }, state.controller.signal);
      if (state.error) throw state.error;
      // Registered sessions require a runtime acknowledgement. Pending native
      // sessions already own human control and can be joined without a mod.
      state.mode = modeId;
      return {};
    },

    async prompt({ sessionId, prompt }, signal) {
      const state = getSession(sessionId);
      if (state.archived) throw bridgeError("Saved session history is read-only; no native runtime was resumed");
      if (state.registrationPending) throw bridgeError(REGISTRATION_PENDING_REASON);
      if (!Array.isArray(prompt) || !prompt.length ||
          prompt.some((block) => block.type !== "text" || typeof block.text !== "string")) {
        throw invalid("Only text prompt content is supported");
      }
      if (state.pending) throw invalid("A prompt is already pending for this session");
      let resolvePrompt;
      let rejectPrompt;
      const completion = new Promise((resolve, reject) => {
        resolvePrompt = resolve;
        rejectPrompt = reject;
      });
      const pending = {
        requestId: randomUUID(),
        turnId: null,
        submitted: false,
        streamed: false,
        settled: false,
        cancelRequested: false,
        cancelSent: false,
        resolve(value) { this.settled = true; resolvePrompt(value); },
        reject(error) { this.settled = true; rejectPrompt(error); },
      };
      state.pending = pending;
      const cancel = () => {
        if (lifetime.signal.aborted) return;
        pending.cancelRequested = true;
        maybeCancel(state, pending);
      };
      signal?.addEventListener("abort", cancel, { once: true });
      if (signal?.aborted) cancel();
      // Register the UUID before HTTP submission so fast events cannot be lost.
      // Do not retry this POST: a lost response does not mean it was not run.
      void request("POST", `${sessionPath(sessionId)}/prompt`, {
        text: prompt.map((block) => block.text).join("\n"),
        requestId: pending.requestId,
      }, state.controller.signal).then((response) => {
        if (response?.requestId !== pending.requestId) throw bridgeError("Broker prompt requestId mismatch");
        pending.submitted = true;
        maybeCancel(state, pending);
      }).catch((error) => {
        if (state.pending !== pending || pending.settled) return;
        error.data = { ...error.data, requestId: pending.requestId };
        if ([400, 401, 403, 404, 409, 413, 422].includes(error.data.status)) {
          // A definite refusal, such as human control, does not lose the stream.
          pending.reject(error);
        } else {
          fail(state, error);
        }
      });
      try {
        return await completion;
      } finally {
        signal?.removeEventListener("abort", cancel);
        if (state.pending === pending) state.pending = null;
      }
    },

    async cancel({ sessionId }) {
      const state = getSession(sessionId);
      const pending = state.pending;
      if (!pending) return;
      pending.cancelRequested = true;
      // Wait until registration is known, not until native prompt acceptance.
      maybeCancel(state, pending);
    },
  };
}

/**
 * Serves stable ACP v1 over stdio using SDK 2.0.1. The optional byte streams let
 * embedders test the same framing and dispatch without replacing process stdio.
 * Resolves when the ACP transport closes; native broker sessions remain alive.
 * @param {BridgeClient} client Authenticated broker HTTP client.
 * @param {{input?: ReadableStream<Uint8Array>, output?: WritableStream<Uint8Array>}} [streams]
 * @returns {Promise<void>} Transport lifetime.
 */
export function serveACP(client, streams = {}) {
  let handlers;
  const app = agent({ name: "claude-acp-bridge" }).onConnect((connection) => {
    handlers = createAgent(connection, client);
  });
  const requests = {
    [methods.agent.initialize]: "initialize",
    [methods.agent.session.new]: "newSession",
    [methods.agent.session.load]: "loadSession",
    [methods.agent.session.list]: "listSessions",
    [methods.agent.session.setMode]: "setSessionMode",
    [methods.agent.session.prompt]: "prompt",
  };
  for (const [method, name] of Object.entries(requests)) {
    app.onRequest(method, ({ params, signal }) => handlers[name](params, signal));
  }
  app.onNotification(methods.agent.session.cancel, ({ params }) => handlers.cancel(params));
  const connection = app.connect(ndJsonStream(
    streams.output ?? Writable.toWeb(process.stdout),
    streams.input ?? Readable.toWeb(process.stdin),
  ));
  return connection.closed;
}
