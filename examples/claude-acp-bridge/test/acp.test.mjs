import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import { EventEmitter } from "node:events";
import { describe, test } from "node:test";
import {
  client as acpClient,
  methods,
  ndJsonStream,
  PROTOCOL_VERSION,
} from "@agentclientprotocol/sdk";
import { createAgent, serveACP } from "../src/acp.mjs";
import { createBroker } from "../src/broker.mjs";
import { createClient } from "../src/client.mjs";

const META = "claude-acp-bridge";
const initialize = { protocolVersion: PROTOCOL_VERSION, clientCapabilities: {} };

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function observe(promise) {
  const result = { settled: false, promise };
  void promise.then(() => { result.settled = true; }, () => { result.settled = true; });
  return result;
}

class Broker {
  constructor() {
    this.session = {
      sessionId: randomUUID(), cwd: "/work/project", mode: "automation",
      runtimeId: randomUUID(), activeTurnId: null, lastSeen: 123,
      startedAt: 0, ended: false, fault: false, stale: false,
    };
    this.sessions = [this.session];
    this.calls = [];
    this.events = [];
    this.changes = new EventEmitter();
    this.waiters = new Set();
    this.eventFailure = null;
    this.batch = null;
    this.onPrompt = async (body) => ({ requestId: body.requestId });
    this.onCancel = async () => ({});
    this.onMode = async ({ mode }) => { this.emit("mode", { mode }); return {}; };
  }

  emit(type, data) {
    this.events.push({ seq: this.events.length + 1, type, data });
    this.wake();
  }

  wake() {
    for (const wake of this.waiters) wake();
  }

  async until(predicate) {
    while (!predicate()) await new Promise((resolve) => this.changes.once("call", resolve));
  }

  async called(method, suffix, count = 1) {
    const matching = () => this.calls.filter((call) => call.method === method && call.path.endsWith(suffix));
    await this.until(() => matching().length >= count);
    return matching()[count - 1];
  }

  async request(method, path, body, signal) {
    signal?.throwIfAborted();
    this.calls.push({ method, path, body, signal });
    this.changes.emit("call");
    const url = new URL(path, "http://broker.local");
    if (path === "/v1/capabilities") return { harnesses: this.harnesses ?? [{ id: "claude", name: "claude-acp-bridge", title: "Claude Code" }] };
    if (path === "/v1/sessions") {
      if (method === "GET") return { sessions: this.sessions };
      assert.equal(method, "POST");
      this.session.cwd = body.cwd;
      return { ...this.session };
    }
    if (url.pathname.endsWith("/prompt")) return this.onPrompt(body, signal);
    if (url.pathname.endsWith("/cancel")) return this.onCancel(body, signal);
    if (url.pathname.endsWith("/mode")) return this.onMode(body, signal);
    if (url.pathname.endsWith("/resume")) return this.onResume(body, signal);
    if (url.pathname.endsWith("/events")) {
      const after = Number(url.searchParams.get("after"));
      while (true) {
        signal?.throwIfAborted();
        if (this.eventFailure) throw this.eventFailure;
        if (this.batch) return this.batch;
        const events = this.events.filter(({ seq }) => seq > after);
        if (events.length || url.searchParams.get("wait") !== "1") {
          return { events, cursor: events.at(-1)?.seq ?? after };
        }
        await new Promise((resolve, reject) => {
          const cleanup = () => {
            this.waiters.delete(wake);
            signal?.removeEventListener("abort", abort);
          };
          const wake = () => { cleanup(); resolve(); };
          const abort = () => { cleanup(); reject(signal.reason); };
          this.waiters.add(wake);
          signal?.addEventListener("abort", abort, { once: true });
        });
      }
    }
    const session = this.sessions.find(({ sessionId }) => url.pathname.endsWith(`/${sessionId}`));
    if (!session) throw new Error("session not found");
    return { ...session };
  }
}

async function connect(t, broker = new Broker()) {
  const toAgent = new TransformStream();
  const fromAgent = new TransformStream();
  const notifications = [];
  const permissions = [];
  const served = serveACP(broker, { input: toAgent.readable, output: fromAgent.writable });
  const connection = acpClient({ name: "acceptance-client" })
    .onNotification(methods.client.session.update, ({ params }) => { notifications.push(params); })
    .onRequest(methods.client.session.requestPermission, ({ params }) => {
      permissions.push(params);
      throw new Error("Permissions must stay in native Claude");
    })
    .connect(ndJsonStream(toAgent.writable, fromAgent.readable));
  let closed = false;
  async function close() {
    if (closed) return;
    closed = true;
    connection.close();
    await toAgent.writable.close();
    await served;
  }
  t.after(close);
  const rpc = connection.agent;
  const init = await rpc.request(methods.agent.initialize, initialize);
  const sessionId = broker.session.sessionId;
  return {
    broker, rpc, init, notifications, permissions, close,
    newSession: () => rpc.request(methods.agent.session.new, { cwd: broker.session.cwd, mcpServers: [] }),
    loadSession: (params = {}) => rpc.request(methods.agent.session.load, {
      sessionId, cwd: broker.session.cwd, mcpServers: [], ...params,
    }),
    prompt: (text = "hello") => observe(rpc.request(methods.agent.session.prompt, {
      sessionId, prompt: [{ type: "text", text }],
    })),
    async flush() {
      await broker.called("GET", `/events?after=${broker.events.length}&wait=1`);
      await rpc.request(methods.agent.initialize, initialize);
    },
    updates: () => notifications.map(({ update }) => update),
  };
}

function finish(broker, requestId, extra = {}) {
  broker.emit("turn.complete", {
    turnId: "root", requestId, answer: "answer", isAborted: false, reason: "answer", ...extra,
  });
}

function textOf(updates) {
  return updates.filter(({ sessionUpdate }) => sessionUpdate === "agent_message_chunk")
    .map(({ content }) => content.text).join("");
}

describe("ACP broker acceptance", { concurrency: true, timeout: 10000 }, () => {
  test("uses real SDK framing and advertises only implemented capabilities", async (t) => {
    assert.equal(typeof createAgent, "function");
    const { init } = await connect(t);
    assert.equal(init.protocolVersion, PROTOCOL_VERSION);
    assert.deepEqual(init.agentInfo, { name: "claude-acp-bridge", title: "Claude Code", version: "0.1.0" });
    assert.deepEqual(init.agentCapabilities, {
      loadSession: true,
      promptCapabilities: { image: false, audio: false, embeddedContext: false },
      mcpCapabilities: { http: false, sse: false },
      sessionCapabilities: { list: {} },
    });
    assert.deepEqual(init.authMethods, []);
    assert.equal(init._meta[META].fullHistory, false);
    assert.equal(init._meta[META].history, "retained-broker-events-only");
    assert.equal(init._meta[META].permissions, "native-attach");
    assert.deepEqual(init._meta[META].promptContent, ["text"]);
  });

  test("new and list preserve native identity, cwd, modes and session status metadata", async (t) => {
    const f = await connect(t);
    const status = {
      startedAt: 0, ended: false, fault: false, stale: false, activeTurnId: "native-turn",
      lastError: { message: "Native tool call failed; inspect the transcript", at: 100 },
    };
    Object.assign(f.broker.session, status);
    const created = await f.newSession();
    assert.equal(created.sessionId, f.broker.session.sessionId);
    assert.equal(created.modes.currentModeId, "automation");
    assert.deepEqual(created.modes.availableModes.map(({ id }) => id), ["human", "automation"]);
    assert.equal(created._meta[META].nativeSessionId, created.sessionId);
    assert.deepEqual((await f.broker.called("POST", "/v1/sessions")).body, { cwd: "/work/project" });
    const listed = await f.rpc.request(methods.agent.session.list, {});
    assert.equal(listed.sessions[0].sessionId, created.sessionId);
    assert.equal(listed.sessions[0].cwd, "/work/project");
    for (const meta of [created._meta[META], listed.sessions[0]._meta[META]]) {
      for (const [key, value] of Object.entries(status)) assert.deepEqual(meta[key], value, key);
    }
    const filtered = await f.rpc.request(methods.agent.session.list, { cwd: "/somewhere/else" });
    assert.deepEqual(filtered.sessions, []);
    await assert.rejects(f.rpc.request(methods.agent.session.list, { cursor: "unknown" }), /pagination/);
    await assert.rejects(f.rpc.request(methods.agent.session.delete, { sessionId: created.sessionId }), /Method not found/);
  });

  test("loads an empty native session without waiting for an event", async (t) => {
    const f = await connect(t);
    f.broker.session.lastError = { message: "Native turn failed; inspect the transcript", at: 10 };
    f.broker.session.stale = true;
    const loaded = await f.loadSession();
    assert.equal(loaded._meta[META].fullHistory, false);
    assert.equal(loaded._meta[META].cursor, 0);
    assert.equal(loaded._meta[META].startedAt, 0);
    assert.equal(loaded._meta[META].ended, false);
    assert.equal(loaded._meta[META].fault, false);
    assert.equal(loaded._meta[META].stale, true);
    assert.equal(loaded._meta[META].activeTurnId, undefined);
    assert.deepEqual(loaded._meta[META].lastError, f.broker.session.lastError);
    assert.equal(f.broker.calls.filter(({ method }) => method === "POST").length, 0);
    assert.deepEqual(await f.loadSession(), loaded, 'read-only reload refreshes the existing attachment');
    assert.equal(f.broker.calls.filter(({ path }) => path.includes('/events?')).length, 2, 'reload does not replay or add a poller');
  });

  test("older brokers do not invent a start time from the activity heartbeat", async (t) => {
    for (const method of ["newSession", "loadSession"]) {
      const f = await connect(t);
      delete f.broker.session.startedAt;
      const attached = await f[method]();
      const listed = await f.rpc.request(methods.agent.session.list, {});
      for (const meta of [attached._meta[META], listed.sessions[0]._meta[META]]) {
        assert.equal(Object.hasOwn(meta, "startedAt"), false);
        assert.equal(Object.hasOwn(meta, "lastError"), false);
        assert.equal(Object.hasOwn(meta, "activeTurnId"), false);
      }
      assert.equal(listed.sessions[0]._meta[META].lastSeen, 123);
    }
  });

  test("load replays retained events in order and labels partial history", async (t) => {
    const broker = new Broker();
    broker.emit("turn.start", { turnId: "old", text: "old question" });
    broker.emit("thought", { turnId: "old", text: "thinking" });
    broker.emit("tool.start", { turnId: "old", toolCallId: "read", title: "Read", input: { path: "a" } });
    broker.emit("tool.end", { turnId: "old", toolCallId: "read", result: "file", isError: false });
    broker.emit("text", { turnId: "old", text: "old answer" });
    finish(broker, undefined, { turnId: "old", answer: "old answer" });
    const f = await connect(t, broker);
    const loaded = await f.loadSession();
    assert.equal(loaded._meta[META].cursor, 6);
    assert.equal(loaded._meta[META].history, "retained-broker-events-only");
    assert.deepEqual(f.updates().map(({ sessionUpdate }) => sessionUpdate), [
      "user_message_chunk", "agent_thought_chunk", "tool_call", "tool_call_update", "agent_message_chunk",
    ]);
    assert.equal(textOf(f.updates()), "old answer");
    assert.equal(Object.hasOwn(f.updates()[3], "rawInput"), false);
    assert.deepEqual(f.notifications.map(({ _meta }) => _meta[META].seq), [1, 2, 3, 4, 5]);
    assert.deepEqual(f.permissions, []);
  });

  test("rejects mismatched load cwd and injected session configuration", async (t) => {
    const f = await connect(t);
    await assert.rejects(f.loadSession({ cwd: "/different" }), /cwd does not match/);
    await assert.rejects(f.loadSession({ sessionId: "../../escape" }), /session ID/);
    for (const method of [methods.agent.session.new, methods.agent.session.load]) {
      const base = { sessionId: f.broker.session.sessionId, cwd: "/work/project", mcpServers: [] };
      await assert.rejects(f.rpc.request(method, { ...base, cwd: "relative" }), /absolute path/);
      await assert.rejects(f.rpc.request(method, {
        ...base, mcpServers: [{ name: "injected", command: "shell", args: [], env: [] }],
      }), /MCP server injection/);
      await assert.rejects(f.rpc.request(method, { ...base, additionalDirectories: ["/extra"] }), /Additional directories/);
    }
    assert.equal(f.broker.calls.filter(({ method }) => method === "POST").length, 0);
    await f.loadSession();
  });

  test("acceptance and unrelated human or subagent completions do not finish the prompt", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    assert.match(requestId, /^[0-9a-f-]{36}$/);
    f.broker.emit("prompt.accepted", { requestId });
    f.broker.emit("turn.start", { requestId, turnId: "root", text: "hello" });
    f.broker.emit("turn.start", { turnId: "human", text: "human question" });
    f.broker.emit("text", { turnId: "human", text: "not ours" });
    finish(f.broker, undefined, { turnId: "human" });
    f.broker.emit("text", { requestId, turnId: "subagent", text: "also not ours" });
    finish(f.broker, requestId, { turnId: "subagent" });
    finish(f.broker, "another-request", { turnId: "root" });
    await f.flush();
    assert.equal(prompt.settled, false);
    assert.deepEqual(f.updates(), []);
    // Once bound to the root turn, requestId may be absent from later events.
    f.broker.emit("text", { turnId: "root", text: "ours" });
    finish(f.broker, undefined, { answer: "ours" });
    assert.deepEqual(await prompt.promise, { stopReason: "end_turn" });
    assert.equal(textOf(f.updates()), "ours");
  });

  test("fast completion can precede both HTTP response and prompt acceptance", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const response = deferred();
    f.broker.onPrompt = async (body) => {
      f.broker.emit("text", { requestId: body.requestId, turnId: "root", text: "fast" });
      finish(f.broker, body.requestId, { answer: "fast" });
      return response.promise;
    };
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    assert.deepEqual(await prompt.promise, { stopReason: "end_turn" });
    assert.equal(textOf(f.updates()), "fast");
    response.resolve({ requestId });
    f.broker.emit("prompt.accepted", { requestId });
    await f.flush();
    assert.equal(textOf(f.updates()), "fast");
  });

  test("thoughts, pending tools, tool results, and text retain broker order", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    const data = { requestId, turnId: "root" };
    f.broker.emit("turn.start", { ...data, text: "hello" });
    f.broker.emit("thought", { ...data, text: "checking" });
    f.broker.emit("tool.start", { ...data, toolCallId: "tool", title: "Run", input: '{"command":' });
    f.broker.emit("tool.end", { ...data, toolCallId: "tool", input: { command: "pwd" }, result: { output: "/work" }, isError: true });
    f.broker.emit("text", { ...data, text: "one" });
    f.broker.emit("text", { ...data, text: "two" });
    finish(f.broker, requestId, { answer: "onetwo" });
    await prompt.promise;
    assert.deepEqual(f.updates().map(({ sessionUpdate }) => sessionUpdate), [
      "agent_thought_chunk", "tool_call", "tool_call_update", "agent_message_chunk", "agent_message_chunk",
    ]);
    assert.equal(f.updates()[1].status, "pending");
    assert.equal(f.updates()[1]._meta[META].permissions, "native-attach");
    assert.equal(f.updates()[1].rawInput, '{"command":');
    assert.deepEqual(f.updates()[2].rawInput, { command: "pwd" });
    assert.equal(f.updates()[2].status, "failed");
    assert.deepEqual(f.updates()[2].rawOutput, { output: "/work" });
    assert.equal(textOf(f.updates()), "onetwo");
    assert.deepEqual(f.permissions, []);
  });

  test("emits final answer once when only thoughts or empty text were streamed", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    f.broker.emit("thought", { turnId: "root", requestId, text: "thinking" });
    f.broker.emit("text", { turnId: "root", requestId, text: "" });
    finish(f.broker, requestId);
    await prompt.promise;
    assert.equal(textOf(f.updates()), "answer");
  });

  test("native prompt rejection fails promptly and allows a later prompt", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const first = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    f.broker.emit("prompt.rejected", { requestId: "other", reason: "unrelated" });
    f.broker.emit("prompt.rejected", { requestId, reason: "human mode" });
    await assert.rejects(first.promise, /Native prompt rejected: human mode/);
    const second = f.prompt();
    const secondId = (await f.broker.called("POST", "/prompt", 2)).body.requestId;
    assert.notEqual(secondId, requestId);
    finish(f.broker, secondId);
    assert.equal((await second.promise).stopReason, "end_turn");
  });

  test("rejects unsupported non-text content without any prompt HTTP mutation", async (t) => {
    const f = await connect(t);
    await f.newSession();
    for (const block of [
      { type: "image", data: "AAAA", mimeType: "image/png" },
      { type: "audio", data: "AAAA", mimeType: "audio/wav" },
      { type: "resource_link", uri: "file:///secret", name: "secret" },
      { type: "resource", resource: { uri: "file:///secret", text: "secret" } },
    ]) {
      await assert.rejects(f.rpc.request(methods.agent.session.prompt, {
        sessionId: f.broker.session.sessionId, prompt: [{ type: "text", text: "hello" }, block],
      }), /Only text/);
    }
    await assert.rejects(f.rpc.request(methods.agent.session.prompt, {
      sessionId: f.broker.session.sessionId, prompt: [],
    }), /Only text/);
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/prompt")).length, 0);
  });

  test("joins text blocks and rejects concurrent prompts on one session", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = observe(f.rpc.request(methods.agent.session.prompt, {
      sessionId: f.broker.session.sessionId, prompt: [{ type: "text", text: "a" }, { type: "text", text: "b" }],
    }));
    const call = await f.broker.called("POST", "/prompt");
    assert.equal(call.body.text, "a\nb");
    await assert.rejects(f.prompt().promise, /already pending/);
    finish(f.broker, call.body.requestId);
    await prompt.promise;
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/prompt")).length, 1);
  });

  test("cancellation before registration queues until HTTP ACK, not native acceptance", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const response = deferred();
    f.broker.onPrompt = () => response.promise;
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    await f.rpc.notify(methods.agent.session.cancel, { sessionId: f.broker.session.sessionId });
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/cancel")).length, 0);
    response.resolve({ requestId });
    const cancel = await f.broker.called("POST", "/cancel");
    assert.deepEqual(cancel.body, { requestId });
    await f.rpc.notify(methods.agent.session.cancel, { sessionId: f.broker.session.sessionId });
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.equal(prompt.settled, false);
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/cancel")).length, 1);
    finish(f.broker, requestId, { reason: "aborted", isAborted: true, answer: "" });
    assert.deepEqual(await prompt.promise, { stopReason: "cancelled" });
  });

  test("accepted event lets a queued cancellation proceed before HTTP returns", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const response = deferred();
    f.broker.onPrompt = () => response.promise;
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    await f.rpc.notify(methods.agent.session.cancel, { sessionId: f.broker.session.sessionId });
    f.broker.emit("prompt.accepted", { requestId });
    await f.broker.called("POST", "/cancel");
    finish(f.broker, requestId, { reason: "aborted", answer: "" });
    assert.equal((await prompt.promise).stopReason, "cancelled");
    response.resolve({ requestId });
  });

  test("cancelling an idle ACP attachment does not cancel a human turn", async (t) => {
    const f = await connect(t);
    f.broker.session.activeTurnId = "human";
    await f.loadSession();
    await f.rpc.notify(methods.agent.session.cancel, { sessionId: f.broker.session.sessionId });
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/cancel")).length, 0);
  });

  test("cancel HTTP failure fails the pending prompt explicitly", async (t) => {
    const f = await connect(t);
    await f.newSession();
    f.broker.onCancel = async () => { throw new Error("cancel disconnected"); };
    const prompt = f.prompt();
    await f.broker.called("POST", "/prompt");
    await f.rpc.notify(methods.agent.session.cancel, { sessionId: f.broker.session.sessionId });
    await assert.rejects(prompt.promise, /cancel disconnected/);
  });

  test("startup and control ACK events do not break attachment or finish prompts", async (t) => {
    const broker = new Broker();
    broker.session.attachCommand = "claude attach " + broker.session.sessionId.slice(0, 8);
    broker.emit("session.start", { ...broker.session });
    const f = await connect(t, broker);
    const created = await f.newSession();
    assert.equal(created._meta[META].attachCommand, broker.session.attachCommand);
    const prompt = f.prompt();
    const { requestId } = (await broker.called("POST", "/prompt")).body;
    broker.emit("command.done", { commandId: requestId });
    broker.emit("command.error", { commandId: "unrelated", reason: "failed control" });
    await f.flush();
    assert.equal(prompt.settled, false);
    assert.deepEqual(f.updates(), []);
    finish(broker, requestId);
    await prompt.promise;
  });

  test("reports native permission waits without requesting or granting ACP permission", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    const input = { requestId, kind: "permission", tool: "Bash", input: { command: "pwd" } };
    f.broker.emit("needs.input", { ...input, requestId: "another" });
    f.broker.emit("needs.input", input);
    await f.flush();
    assert.equal(prompt.settled, false);
    assert.equal(f.updates().length, 1);
    assert.deepEqual(f.updates()[0]._meta[META], { pendingInput: input, permissions: "native-attach" });
    f.broker.emit("needs.input", { requestId, pending: false });
    await f.flush();
    assert.equal(f.updates()[1]._meta["coder-acp"].pendingInput, null);
    assert.equal(prompt.settled, false);
    assert.deepEqual(f.permissions, []);
    finish(f.broker, requestId);
    await prompt.promise;
  });

  test("SDK request cancellation delegates to the broker without inventing completion", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const controller = new AbortController();
    const prompt = observe(f.rpc.request(methods.agent.session.prompt, {
      sessionId: f.broker.session.sessionId, prompt: [{ type: "text", text: "hello" }],
    }, { cancellationSignal: controller.signal }));
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    controller.abort();
    assert.equal((await f.broker.called("POST", "/cancel")).body.requestId, requestId);
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.equal(prompt.settled, false);
    finish(f.broker, requestId, { reason: "aborted", answer: "" });
    assert.equal((await prompt.promise).stopReason, "cancelled");
  });

  test("mode changes wait for runtime ACK and stream the canonical mode", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const ack = deferred();
    f.broker.onMode = () => ack.promise;
    const mode = observe(f.rpc.request(methods.agent.session.setMode, {
      sessionId: f.broker.session.sessionId, modeId: "human",
    }));
    const call = await f.broker.called("POST", "/mode");
    assert.deepEqual(call.body, { mode: "human" });
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.equal(mode.settled, false);
    f.broker.emit("mode", { mode: "human" });
    ack.resolve({});
    assert.deepEqual(await mode.promise, {});
    await f.flush();
    assert.deepEqual(f.updates(), [{ sessionUpdate: "current_mode_update", currentModeId: "human" }]);
    await assert.rejects(f.rpc.request(methods.agent.session.setMode, {
      sessionId: f.broker.session.sessionId, modeId: "bypassPermissions",
    }), /Unsupported native session mode/);
  });

  for (const [reason, isAborted, expected] of [
    ["answer", false, "end_turn"], ["refusal", false, "refusal"],
    ["aborted", false, "cancelled"], ["answer", true, "cancelled"],
    ["error", false, null],
  ]) {
    test(`maps completion ${reason}, aborted=${isAborted} without pretending errors succeeded`, async (t) => {
      const f = await connect(t);
      await f.newSession();
      const prompt = f.prompt();
      const { requestId } = (await f.broker.called("POST", "/prompt")).body;
      finish(f.broker, requestId, { reason, isAborted });
      if (expected) assert.equal((await prompt.promise).stopReason, expected);
      else await assert.rejects(prompt.promise, /Native turn failed/);
      assert.equal(textOf(f.updates()), "answer");
    });
  }

  for (const event of ["runtime.replaced", "overflow", "session.end"]) {
    test(`fails a pending prompt explicitly on ${event}`, async (t) => {
      const f = await connect(t);
      await f.newSession();
      const prompt = f.prompt();
      await f.broker.called("POST", "/prompt");
      f.broker.emit(event, { reason: "native disconnected" });
      await assert.rejects(prompt.promise, event === "session.end" ? /Native session ended/ : new RegExp(event));
      await assert.rejects(f.prompt().promise, /reattach|ended/);
    });
  }

  for (const [name, batch] of [
    ["missing sequence", { events: [{ seq: 2, type: "mode", data: { mode: "human" } }], cursor: 2 }],
    ["cursor skip", { events: [], cursor: 10 }],
    ["duplicate sequence", { events: [{ seq: 0, type: "mode", data: { mode: "human" } }], cursor: 0 }],
    ["out of order", { events: [{ seq: 2, type: "mode", data: { mode: "human" } }, { seq: 1, type: "mode", data: { mode: "human" } }], cursor: 1 }],
  ]) {
    test(`fails on event ${name} without retrying the broken stream`, async (t) => {
      const f = await connect(t);
      await f.newSession();
      const prompt = f.prompt();
      await f.broker.called("POST", "/prompt");
      const reads = f.broker.calls.filter(({ path }) => path.includes("/events?")).length;
      f.broker.batch = batch;
      f.broker.wake();
      await assert.rejects(prompt.promise, /gap/);
      assert.equal(f.broker.calls.filter(({ path }) => path.includes("/events?")).length, reads);
    });
  }

  test("truncated retained history fails load rather than claiming full history", async (t) => {
    const f = await connect(t);
    f.broker.batch = { events: [{ seq: 10, type: "mode", data: { mode: "human" } }], cursor: 10 };
    await assert.rejects(f.loadSession(), /event gap/);
    assert.deepEqual(f.updates(), []);
  });

  test("a broken batch cannot return successful completion before its sequence gap", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    f.broker.batch = {
      events: [
        { seq: 1, type: "turn.complete", data: { turnId: "root", requestId, answer: "no", reason: "answer" } },
        { seq: 3, type: "mode", data: { mode: "human" } },
      ], cursor: 3,
    };
    f.broker.wake();
    await assert.rejects(prompt.promise, /event gap/);
    assert.equal(textOf(f.updates()), "");
  });

  test("event HTTP disconnect fails the prompt and is never retried", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const prompt = f.prompt();
    await f.broker.called("POST", "/prompt");
    f.broker.eventFailure = new Error("connection reset");
    f.broker.wake();
    await assert.rejects(prompt.promise, /connection reset/);
    assert.equal(f.broker.calls.filter(({ path }) => path.includes("/events?")).length, 2);
  });

  test("a definite HTTP prompt rejection allows mode control and a fresh prompt", async (t) => {
    const f = await connect(t);
    await f.newSession();
    f.broker.onPrompt = async () => { throw Object.assign(new Error("human control"), { status: 409 }); };
    await assert.rejects(f.prompt().promise, (error) => {
      assert.match(error.message, /human control/);
      assert.equal(error.data.status, 409);
      assert.match(error.data.requestId, /^[0-9a-f-]{36}$/);
      return true;
    });
    await f.rpc.request(methods.agent.session.setMode, {
      sessionId: f.broker.session.sessionId, modeId: "automation",
    });
    f.broker.onPrompt = async ({ requestId }) => ({ requestId });
    const next = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt", 2)).body;
    finish(f.broker, requestId);
    assert.equal((await next.promise).stopReason, "end_turn");
  });

  test("a late HTTP failure from a completed prompt cannot poison the next turn", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const response = deferred();
    f.broker.onPrompt = async ({ requestId }) => {
      finish(f.broker, requestId);
      return response.promise;
    };
    await f.prompt().promise;
    f.broker.onPrompt = async ({ requestId }) => ({ requestId });
    const next = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt", 2)).body;
    response.reject(new Error("late HTTP response failure"));
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.equal(next.settled, false);
    finish(f.broker, requestId, { turnId: "next" });
    assert.equal((await next.promise).stopReason, "end_turn");
  });

  test("ambiguous prompt HTTP failure is not retried or reported as success", async (t) => {
    const f = await connect(t);
    await f.newSession();
    f.broker.onPrompt = async () => { throw new Error("response lost after submit"); };
    await assert.rejects(f.prompt().promise, /response lost after submit/);
    await assert.rejects(f.prompt().promise, /response lost after submit/);
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/prompt")).length, 1);
  });

  test("an idle event failure is visible and prevents later prompt submission", async (t) => {
    const f = await connect(t);
    await f.newSession();
    f.broker.emit("unsupported.event", {});
    // A request gives the event consumer and its status notification a wire barrier.
    await assert.rejects(f.rpc.request(methods.agent.session.setMode, {
      sessionId: f.broker.session.sessionId, modeId: "human",
    }), /Unsupported broker event/);
    await f.rpc.request(methods.agent.initialize, initialize);
    assert.match(f.updates().at(-1)._meta[META].error, /Unsupported broker event/);
    await assert.rejects(f.prompt().promise, /Unsupported broker event/);
    assert.equal(f.broker.calls.filter(({ path }) => path.endsWith("/prompt")).length, 0);
  });

  test("explicit pre-submission cancellation resolves as cancelled, not an error", async (t) => {
    const f = await connect(t);
    await f.newSession();
    const running = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    await f.rpc.notify(methods.agent.session.cancel, { sessionId: f.broker.session.sessionId });
    await f.broker.called("POST", "/cancel");
    f.broker.emit("prompt.rejected", { requestId, reason: "Cancelled before submission", cancelled: true });
    assert.equal((await running.promise).stopReason, "cancelled");
  });

  test("load of a running native turn continues streaming text and tool completion", async (t) => {
    const f = await connect(t);
    f.broker.emit("turn.start", { turnId: "running", requestId: "old-client", text: "run" });
    f.broker.emit("text", { turnId: "running", requestId: "old-client", text: "before " });
    f.broker.emit("tool.start", { turnId: "running", toolCallId: "read", title: "Read file" });
    await f.loadSession();
    f.broker.emit("text", { turnId: "running", requestId: "old-client", text: "after" });
    f.broker.emit("tool.end", { turnId: "running", toolCallId: "read", result: "contents", isError: false });
    finish(f.broker, "old-client", { turnId: "running", answer: "before after" });
    await f.flush();
    assert.equal(textOf(f.updates()), "before after");
    assert.ok(f.updates().some(update => update.sessionUpdate === "tool_call_update" && update.status === "completed"));
    f.broker.emit("turn.start", { turnId: "human", text: "new native input" });
    finish(f.broker, undefined, { turnId: "human", answer: "native answer" });
    await f.flush();
    assert.equal(textOf(f.updates()), "before afternative answer");
  });

  test("native surface events do not break an ACP connection", async (t) => {
    const f = await connect(t);
    await f.newSession();
    f.broker.emit("session.attach", {});
    f.broker.emit("session.detach", {});
    await f.flush();
    const pending = f.prompt();
    const { requestId } = (await f.broker.called("POST", "/prompt")).body;
    finish(f.broker, requestId);
    assert.equal((await pending.promise).stopReason, "end_turn");
  });

  test("reattachment replays only the current runtime epoch", async (t) => {
    const f = await connect(t);
    f.broker.emit("session.start", { sessionId: f.broker.session.sessionId, runtimeId: "old" });
    f.broker.emit("runtime.replaced", {});
    f.broker.session.historyStartCursor = 2;
    f.broker.emit("session.start", f.broker.session);
    finish(f.broker, "past", { answer: "current runtime" });
    await f.loadSession();
    assert.equal(textOf(f.updates()), "current runtime");
  });

  test("persisted archives replay read-only history without polling or reviving work", async (t) => {
    const f = await connect(t);
    Object.assign(f.broker.session, { archived: true, ended: true, mode: "human", fault: true });
    f.broker.emit("mode", { mode: "automation" });
    f.broker.emit("turn.start", { turnId: "saved-turn", text: "saved prompt" });
    f.broker.emit("text", { turnId: "saved-turn", text: "saved answer" });
    f.broker.emit("turn.complete", { turnId: "saved-turn", reason: "answer", answer: "saved answer" });
    f.broker.emit("session.end", { reason: "stopped" });
    f.broker.emit("overflow", { reason: "historical integrity fault" });
    const loaded = await f.loadSession();
    assert.equal(loaded._meta[META].archived, true);
    assert.equal(loaded._meta[META].ended, true);
    assert.equal(loaded.modes.currentModeId, "human");
    assert.equal(textOf(f.updates()), "saved answer");
    assert.equal(f.updates().filter(u => u.sessionUpdate === "user_message_chunk")[0].content.text, "saved prompt");
    assert.ok(!f.updates().some(u => u.sessionUpdate === "current_mode_update"));
    const before = f.notifications.length;
    await f.loadSession();
    assert.equal(f.notifications.length, before, "reopening does not duplicate archive history");
    const listed = await f.rpc.request(methods.agent.session.list, {});
    assert.equal(listed.sessions[0]._meta[META].archived, true);
    await assert.rejects(f.prompt().promise, /read-only/);
    await assert.rejects(f.rpc.request(methods.agent.session.setMode, { sessionId: f.broker.session.sessionId, modeId: "automation" }), /read-only/);
    assert.equal(f.broker.calls.filter(c => c.method === "POST").length, 0);
    assert.equal(f.broker.calls.filter(c => c.path.includes("/events?")).length, 1);
    assert.ok(!f.broker.calls.some(c => c.path.endsWith("wait=1")));
  });

  test("archived partial history starts at its retained boundary without inventing missing text", async (t) => {
    const f = await connect(t);
    Object.assign(f.broker.session, { archived: true, ended: true, mode: "human", historyStartCursor: 2 });
    f.broker.emit("turn.start", { turnId: "saved-turn", text: "evicted prompt" });
    f.broker.emit("text", { turnId: "saved-turn", text: "evicted answer prefix" });
    f.broker.emit("text", { turnId: "saved-turn", text: "retained suffix" });
    f.broker.emit("turn.complete", { turnId: "saved-turn", reason: "answer", answer: "evicted answer prefix retained suffix" });
    f.broker.events.splice(0, 2);
    const loaded = await f.loadSession();
    assert.equal(textOf(f.updates()), "retained suffix");
    assert.equal(loaded._meta[META].fullHistory, false);
    assert.equal(loaded._meta[META].cursor, 4);
  });

  test("startup restoration appends live output without losing or duplicating saved history", async (t) => {
    const f = await connect(t);
    const id = f.broker.session.sessionId;
    Object.assign(f.broker.session, { archived: true, resumable: true, ended: true, mode: "human" });
    f.broker.emit("session.start", { sessionId: id, runtimeId: f.broker.session.runtimeId });
    f.broker.emit("text", { turnId: "old-turn", text: "saved answer" });
    await f.loadSession();
    assert.equal(f.broker.calls.filter(c => c.method === "POST").length, 0);
    f.broker.emit("session.resuming", { sessionId: id, mode: "human" });
    Object.assign(f.broker.session, { archived: false, resumable: false, ended: false,
      registrationPending: true, resumePending: true, runtimeStartCursor: 3, cursor: 3 });
    await f.rpc.request(methods.agent.session.list, {});
    assert.equal(f.broker.calls.filter(c => c.path.endsWith("/resume")).length, 0);
    assert.equal(textOf(f.updates()), "saved answer");
    await assert.rejects(f.prompt().promise, /registration is pending/);
    const runtimeId = randomUUID();
    f.broker.emit("runtime.resumed", { sessionId: id, runtimeId, mode: "human", lastSeen: 456 });
    f.broker.emit("session.start", { sessionId: id, runtimeId });
    await f.broker.called("GET", "/events?after=5&wait=1");
    const status = f.updates().filter(u => u.sessionUpdate === "session_info_update").at(-1)._meta[META];
    assert.equal(status.archived, false);
    assert.equal(status.ended, false);
    assert.equal(status.resumePending, false);
    assert.equal(status.registrationPending, false);
    f.broker.onPrompt = async body => {
      f.broker.emit("turn.start", { requestId: body.requestId, turnId: "new-turn", text: body.text });
      f.broker.emit("text", { requestId: body.requestId, turnId: "new-turn", text: "new answer" });
      f.broker.emit("turn.complete", { requestId: body.requestId, turnId: "new-turn", reason: "answer", answer: "new answer" });
      return { requestId: body.requestId };
    };
    await f.rpc.request(methods.agent.session.setMode, { sessionId: id, modeId: "automation" });
    assert.deepEqual(await f.prompt().promise, { stopReason: "end_turn" });
    assert.equal(textOf(f.updates()), "saved answernew answer");
  });

  test("control changes never resume an archive or disturb its readable history", async (t) => {
    const f = await connect(t);
    Object.assign(f.broker.session, { archived: true, resumable: true, ended: true, mode: "human" });
    f.broker.emit("text", { turnId: "old-turn", text: "saved answer" });
    await f.loadSession();
    for (const modeId of ["human", "automation"]) {
      await assert.rejects(f.rpc.request(methods.agent.session.setMode, {
        sessionId: f.broker.session.sessionId, modeId,
      }), /read-only/);
    }
    const loaded = await f.loadSession();
    assert.equal(loaded._meta[META].archived, true);
    assert.equal(textOf(f.updates()), "saved answer");
    assert.equal(f.broker.calls.filter(c => c.method === "POST").length, 0);
    assert.ok(!f.broker.calls.some(c => c.path.endsWith("wait=1")));
  });

  test("a fresh attachment replays earlier transcripts but not earlier runtime controls after resume", async (t) => {
    const f = await connect(t);
    const id = f.broker.session.sessionId;
    const oldRuntime = randomUUID();
    const currentRuntime = f.broker.session.runtimeId;
    Object.assign(f.broker.session, { mode: "human", runtimeStartCursor: 5 });
    f.broker.emit("session.start", { sessionId: id, runtimeId: oldRuntime });
    f.broker.emit("mode", { mode: "automation" });
    f.broker.emit("text", { turnId: "old-turn", text: "before restart" });
    f.broker.emit("session.end", { reason: "stopped" });
    f.broker.emit("session.resuming", { sessionId: id, mode: "human" });
    f.broker.emit("runtime.resumed", { sessionId: id, runtimeId: currentRuntime, mode: "human", lastSeen: 456 });
    f.broker.emit("session.start", { sessionId: id, runtimeId: currentRuntime });
    f.broker.emit("text", { turnId: "new-turn", text: "after resume" });
    const loaded = await f.loadSession();
    assert.equal(loaded.modes.currentModeId, "human");
    assert.equal(textOf(f.updates()), "before restartafter resume");
    assert.ok(!f.updates().some(u => u.sessionUpdate === "current_mode_update" && u.currentModeId === "automation"));
    await f.loadSession();
    assert.equal(textOf(f.updates()), "before restartafter resume");
  });

  test("listing or reloading follows a resume initiated by another client without replaying history", async (t) => {
    for (const method of ["list", "load"]) await t.test(method, async t => {
      const f = await connect(t);
      const id = f.broker.session.sessionId;
      Object.assign(f.broker.session, { archived: true, resumable: true, ended: true, mode: "human" });
      f.broker.emit("text", { turnId: "old-turn", text: "saved" });
      await f.loadSession();
      f.broker.emit("session.resuming", { sessionId: id, mode: "human" });
      Object.assign(f.broker.session, { archived: false, ended: false, registrationPending: true,
        resumePending: true, runtimeStartCursor: 2, cursor: 2 });
      if (method === "load") await f.loadSession();
      else await f.rpc.request(methods.agent.session.list, {});
      await f.broker.called("GET", "/events?after=2&wait=1");
      assert.equal(textOf(f.updates()), "saved");
      assert.equal(f.broker.calls.filter(c => c.method === "POST").length, 0);
    });
  });

  test("observers retain the archive after startup restoration is refused without retrying from controls", async (t) => {
    const f = await connect(t);
    const id = f.broker.session.sessionId;
    Object.assign(f.broker.session, { archived: true, resumable: true, ended: true, mode: "human" });
    f.broker.emit("text", { turnId: "old-turn", text: "saved" });
    await f.loadSession();
    f.broker.emit("session.resuming", { sessionId: id, mode: "human" });
    Object.assign(f.broker.session, { archived: false, ended: false, registrationPending: true,
      resumePending: true, runtimeStartCursor: 2, cursor: 2 });
    await f.rpc.request(methods.agent.session.list, {});
    await f.broker.called("GET", "/events?after=2&wait=1");
    f.broker.emit("session.resume_failed", { sessionId: id, reason: "Resume refused", retryable: true });
    Object.assign(f.broker.session, { archived: true, ended: true, registrationPending: false, resumePending: false, cursor: 3 });
    await f.loadSession();
    assert.equal(textOf(f.updates()), "saved");
    await assert.rejects(f.rpc.request(methods.agent.session.setMode, {
      sessionId: id, modeId: "human",
    }), /read-only/);
    await f.rpc.request(methods.agent.session.list, {});
    assert.equal(textOf(f.updates()), "saved");
    assert.equal(f.broker.calls.filter(c => c.method === "POST").length, 0);
  });

  test("an archived startup session without a runtime or transcript loads as empty read-only history", async (t) => {
    const f = await connect(t);
    Object.assign(f.broker.session, { archived: true, ended: true, mode: "human", runtimeId: undefined });
    const loaded = await f.loadSession();
    assert.equal(loaded._meta[META].archived, true);
    assert.equal(loaded._meta[META].runtimeId, undefined);
    assert.deepEqual(f.updates(), []);
    await assert.rejects(f.rpc.request(methods.agent.session.setMode, { sessionId: f.broker.session.sessionId, modeId: "human" }), /read-only/);
    assert.ok(!f.broker.calls.some(c => c.method === "POST" || c.path.endsWith("wait=1")));
  });

  test("closed and faulted sessions stay listed with their error but cannot be loaded as usable", async (t) => {
    for (const field of ["ended", "fault"]) {
      const f = await connect(t);
      f.broker.session[field] = true;
      f.broker.session.lastError = { message: "Runtime event gap; work state is unknown", at: 100 };
      const listed = await f.rpc.request(methods.agent.session.list, {});
      assert.equal(listed.sessions.length, 1);
      assert.equal(listed.sessions[0]._meta[META][field], true);
      assert.deepEqual(listed.sessions[0]._meta[META].lastError, f.broker.session.lastError);
      await assert.rejects(f.loadSession(), /closed or faulted/);
      assert.equal(f.broker.calls.some(({ path }) => path.includes("/events?")), false);
    }
  });

  test("mismatched HTTP prompt identity fails explicitly", async (t) => {
    const f = await connect(t);
    await f.newSession();
    f.broker.onPrompt = async () => ({ requestId: randomUUID() });
    await assert.rejects(f.prompt().promise, /requestId mismatch/);
  });

  test("ACP disconnect aborts broker reads without cancelling or deleting native sessions", async (t) => {
    const broker = new Broker();
    const first = await connect(t, broker);
    await first.newSession();
    const prompt = first.prompt();
    const { requestId } = (await broker.called("POST", "/prompt")).body;
    const rejected = assert.rejects(prompt.promise, /connection closed/);
    await first.close();
    await rejected;
    assert.equal(broker.waiters.size, 0);
    assert.equal(broker.calls.filter(({ path }) => path.endsWith("/cancel")).length, 0);
    assert.equal(broker.calls.filter(({ method }) => method === "DELETE").length, 0);
    assert.ok(broker.calls.every(({ signal }) => signal.aborted));
    // Native work continues and a fresh ACP connection can attach the same UUID.
    finish(broker, requestId, { answer: "completed while detached" });
    const second = await connect(t, broker);
    await second.loadSession();
    assert.equal(textOf(second.updates()), "completed while detached");
    assert.equal(broker.calls.filter(({ path, method }) => path === "/v1/sessions" && method === "POST").length, 1);
  });
});

async function realBrokerFixture(t) {
  let time = 0;
  let timerId = 0;
  let polls = 0;
  const waitMs = 15000;
  const timers = new Map();
  const changes = new EventEmitter();
  const clock = {
    now: () => time,
    setTimer(callback, delay) {
      const id = ++timerId;
      timers.set(id, { callback, at: time + delay });
      if (delay === waitMs) { polls++; changes.emit("poll"); }
      return id;
    },
    clearTimer(id) { timers.delete(id); },
    advance(ms) {
      time += ms;
      for (const [id, timer] of [...timers]) {
        if (timer.at <= time && timers.delete(id)) timer.callback();
      }
    },
  };
  const broker = await createBroker({ ...clock, waitMs });
  const connections = [];
  t.after(async () => {
    for (const controller of connections) controller.abort();
    await broker.close();
  });
  const client = createClient(broker);
  const calls = [];
  const trackedClient = {
    request(method, path, body, signal) {
      const response = client.request(method, path, body, signal);
      calls.push({ method, path, body, signal, response });
      return response;
    },
  };
  const session = {
    sessionId: randomUUID(), runtimeId: randomUUID(), cwd: "/work/offline-test",
    mode: "automation", activeTurnId: null,
  };
  const path = `/v1/sessions/${session.sessionId}`;
  let sequence = 0;
  const heartbeat = (events = [], overrides = {}) => client.request("POST", "/v1/runtime", {
    ...session, ...overrides, events: events.map((event) => ({ ...event, seq: ++sequence })),
  });
  await heartbeat([{ type: "session.start", data: session }]);
  async function waitForPoll(after) {
    while (polls <= after) await new Promise((resolve) => changes.once("poll", resolve));
  }
  return {
    clock, client, calls, session, path, waitForPoll,
    get polls() { return polls; },
    async attach() {
      const controller = new AbortController();
      connections.push(controller);
      const notifications = [];
      const agent = createAgent({
        signal: controller.signal,
        sessionUpdate: async (notification) => { notifications.push(notification); },
      }, trackedClient);
      const before = polls;
      await agent.loadSession({ sessionId: session.sessionId, cwd: session.cwd, mcpServers: [] });
      await waitForPoll(before);
      return {
        agent,
        close: () => controller.abort(),
        updates: () => notifications.map(({ update }) => update),
        async prompt(text = "hello") {
          const pending = observe(agent.prompt({ sessionId: session.sessionId, prompt: [{ type: "text", text }] }));
          const call = calls.at(-1);
          assert.equal(call.path, path + "/prompt");
          await call.response;
          pending.body = call.body;
          return pending;
        },
      };
    },
    async publish(events = [], overrides = {}) {
      const before = polls;
      const response = await heartbeat(events, overrides);
      // A new server-side long poll proves that ACP consumed the entire batch.
      await waitForPoll(before);
      return response;
    },
  };
}

async function acceptedPrompt(f, attachment) {
  const pending = await attachment.prompt();
  const { requestId, text } = pending.body;
  assert.deepEqual((await f.publish()).commands, [{ id: requestId, type: "prompt", text }]);
  assert.deepEqual((await f.publish([
    { type: "prompt.accepted", data: { requestId } },
    { type: "turn.start", data: { requestId, turnId: "root", text } },
    { type: "text", data: { requestId, turnId: "root", text: "before " } },
  ], { activeTurnId: "root" })).commands, []);
  assert.equal(pending.settled, false);
  return pending;
}

async function nextPrompt(f, attachment) {
  const pending = await attachment.prompt("next");
  const { requestId, text } = pending.body;
  assert.deepEqual((await f.publish()).commands, [{ id: requestId, type: "prompt", text }]);
  assert.deepEqual((await f.publish([
    { type: "turn.start", data: { requestId, turnId: "next", text } },
    { type: "turn.complete", data: { requestId, turnId: "next", answer: "next answer", reason: "answer" } },
  ])).commands, []);
  assert.deepEqual(await pending.promise, { stopReason: "end_turn" });
  return pending;
}

describe("ACP with a real offline broker", { concurrency: true, timeout: 10000 }, () => {
  test("a pending prompt survives bounded stale polls without being retried", async (t) => {
    const f = await realBrokerFixture(t);
    const attachment = await f.attach();
    const pending = await acceptedPrompt(f, attachment);
    for (let i = 0; i < 2; i++) {
      const before = f.polls;
      f.clock.advance(15000);
      const state = await f.client.request("GET", f.path);
      assert.equal(state.stale, true);
      assert.equal(state.fault, false);
      assert.equal(state.ended, false);
      await f.waitForPoll(before);
      assert.equal(pending.settled, false);
    }
    const { requestId } = pending.body;
    assert.deepEqual((await f.publish([
      { type: "text", data: { requestId, turnId: "root", text: "after" } },
      { type: "turn.complete", data: { requestId, turnId: "root", answer: "before after", reason: "answer" } },
    ])).commands, []);
    assert.deepEqual(await pending.promise, { stopReason: "end_turn" });
    assert.equal(textOf(attachment.updates()), "before after");
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/prompt")).length, 1);
    const next = await nextPrompt(f, attachment);
    assert.notEqual(next.body.requestId, requestId);
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/prompt")).length, 2);
    assert.equal(attachment.updates().some((update) => update._meta?.[META]?.error), false);
    const state = await f.client.request("GET", f.path);
    assert.equal(state.runtimeId, f.session.runtimeId);
    assert.equal(state.stale, false);
    assert.equal(state.fault, false);
  });

  test("disconnect and reattach while stale observes the original completion before new work", async (t) => {
    const f = await realBrokerFixture(t);
    const first = await f.attach();
    const pending = await acceptedPrompt(f, first);
    f.clock.advance(10001);
    const state = await f.client.request("GET", f.path);
    assert.equal(state.stale, true);
    assert.equal(state.fault, false);
    const rejected = assert.rejects(pending.promise, /ACP transport closed/);
    first.close();
    await rejected;
    const second = await f.attach();
    assert.equal(textOf(second.updates()), "before ");
    assert.equal((await f.client.request("POST", f.path + "/prompt", pending.body)).status, "running");
    const { requestId } = pending.body;
    assert.deepEqual((await f.publish([
      { type: "text", data: { requestId, turnId: "root", text: "after" } },
      { type: "turn.complete", data: { requestId, turnId: "root", answer: "before after", reason: "answer" } },
    ])).commands, []);
    assert.equal(textOf(second.updates()), "before after");
    assert.equal((await f.client.request("POST", f.path + "/prompt", pending.body)).status, "completed");
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/prompt")).length, 1);
    await nextPrompt(f, second);
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/prompt")).length, 2);
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/cancel")).length, 0);
    assert.equal((await f.client.request("GET", f.path)).runtimeId, f.session.runtimeId);
  });

  test("stale cancellation fails only its ACP attachment and is not queued or retried", async (t) => {
    const f = await realBrokerFixture(t);
    const first = await f.attach();
    const pending = await acceptedPrompt(f, first);
    f.clock.advance(10001);
    assert.equal((await f.client.request("GET", f.path)).stale, true);
    const rejected = assert.rejects(pending.promise, (error) => error.data?.status === 409 && /stale/.test(error.message));
    await first.agent.cancel({ sessionId: f.session.sessionId });
    await rejected;
    await assert.rejects(first.agent.cancel({ sessionId: f.session.sessionId }), /stale/);
    await assert.rejects(first.agent.prompt({ sessionId: f.session.sessionId, prompt: [{ type: "text", text: "retry" }] }), /stale/);
    const state = await f.client.request("GET", f.path);
    assert.equal(state.fault, false);
    assert.equal(state.ended, false);
    assert.equal(state.activeTurnId, "root");
    assert.equal((await f.client.request("POST", f.path + "/prompt", pending.body)).status, "running");
    const second = await f.attach();
    await second.agent.cancel({ sessionId: f.session.sessionId });
    assert.deepEqual((await f.publish([], { activeTurnId: "root" })).commands, []);
    const { requestId } = pending.body;
    assert.deepEqual((await f.publish([
      { type: "text", data: { requestId, turnId: "root", text: "after" } },
      { type: "turn.complete", data: { requestId, turnId: "root", answer: "before after", reason: "answer" } },
    ])).commands, []);
    assert.equal(textOf(second.updates()), "before after");
    assert.equal((await f.client.request("POST", f.path + "/prompt", pending.body)).status, "completed");
    await nextPrompt(f, second);
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/cancel")).length, 1);
    assert.equal(f.calls.filter(({ path }) => path.endsWith("/prompt")).length, 2);
    assert.equal((await f.client.request("GET", f.path)).fault, false);
  });
});
