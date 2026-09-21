import path from "node:path";

export const FOREIGN_ID_BASE = 1_000_000_000;

// WebKit's remote inspector broadcasts every response to all attached
// frontends. Playwright asserts on responses whose id it did not issue,
// so teach its WKSession to drop ours.
export function patchPlaywrightForForeignInspector(requireFromPlaywright) {
	// The file is not in playwright-core's export map; resolve from the
	// package root and require by absolute path.
	const coreDir = path.dirname(
		requireFromPlaywright.resolve("playwright-core/package.json"),
	);
	const { WKSession } = requireFromPlaywright(
		path.join(coreDir, "lib/server/webkit/wkConnection.js"),
	);
	const original = WKSession.prototype.dispatchMessage;
	WKSession.prototype.dispatchMessage = function dispatchMessage(object) {
		if (
			typeof object.id === "number" &&
			object.id >= FOREIGN_ID_BASE &&
			!this._callbacks.has(object.id)
		) {
			return;
		}
		return original.call(this, object);
	};
}

// Minimal WebKit remote Web Inspector client used to record Timeline
// (style recalc, layout, paint, event dispatch) and ScriptProfiler
// samples from Playwright's WebKit. Requires the browser to be launched
// with WEBKIT_INSPECTOR_HTTP_SERVER=127.0.0.1:<port>.
//
// The socket lands on the UI-process side of a WebPage; page agents live
// in the web content process and are reached through the Target domain.
export class WebKitInspector {
	constructor(port) {
		this.port = port;
		// Ids in this range are ignored by the patched Playwright WKSession
		// (see patchPlaywrightForForeignInspector) so both frontends can
		// share the page target.
		this.nextId = FOREIGN_ID_BASE;
		this.pending = new Map();
		this.records = [];
		this.samples = [];
		this.cpu = [];
		this.otherEvents = new Map();
		this.targetId = null;
	}

	// Finds the inspectable WebPage target whose URL contains urlPart.
	async connect(urlPart) {
		const html = await (await fetch(`http://127.0.0.1:${this.port}/`)).text();
		const rows = [
			...html.matchAll(
				/<div class="targeturl">([^<]*)<\/div>[\s\S]*?socket\/(\d+\/\d+\/WebPage)/g,
			),
		];
		const row = rows.find((r) => r[1].includes(urlPart)) ?? rows[0];
		if (!row) throw new Error(`no inspectable WebKit target for ${urlPart}`);
		const ws = new WebSocket(`ws://127.0.0.1:${this.port}/socket/${row[2]}`);
		ws.addEventListener("message", (m) => this.onMessage(JSON.parse(m.data)));
		await new Promise((resolve, reject) => {
			ws.addEventListener("open", resolve);
			ws.addEventListener("error", (e) =>
				reject(new Error(`inspector websocket failed: ${e.message ?? e.type}`)),
			);
		});
		this.ws = ws;
		// Target.targetCreated is only sent to the frontend that was attached
		// when the page appeared (Playwright). Probe page ids until one
		// answers with the URL we want.
		for (let n = 1; n <= 64 && !this.targetId; n++) {
			const candidate = `page-${n}`;
			this.targetId = candidate;
			try {
				const r = await this.send("Runtime.evaluate", {
					expression: "location.href",
				});
				if (String(r?.result?.value ?? "").includes(urlPart)) break;
			} catch {
				// not a live target
			}
			this.targetId = null;
		}
		if (!this.targetId)
			throw new Error(`no WebKit page target answering for ${urlPart}`);
		await this.send("Timeline.enable");
		// setInstruments only governs auto and programmatic (console.profile)
		// captures; a frontend-driven recording starts each instrument itself,
		// as Web Inspector does. See start() and stop().
		await this.send("Timeline.setInstruments", {
			instruments: ["Timeline", "ScriptProfiler", "CPU"],
		});
	}

	onMessage(msg) {
		if (msg.method === "Target.dispatchMessageFromTarget") {
			this.onMessage(JSON.parse(msg.params.message));
			return;
		}
		if (msg.id !== undefined) {
			const p = this.pending.get(msg.id);
			if (!p) return;
			this.pending.delete(msg.id);
			if (msg.error)
				p.reject(new Error(`${p.method}: ${JSON.stringify(msg.error)}`));
			else p.resolve(msg.result);
			return;
		}
		switch (msg.method) {
			case "Timeline.eventRecorded":
				this.records.push(msg.params.record);
				break;
			case "ScriptProfiler.trackingComplete":
				this.samples.push(...(msg.params.samples?.stackTraces ?? []));
				this.trackingDone?.();
				break;
			case "CPU.trackingUpdate":
				this.cpu.push(msg.params.event);
				break;
			default:
				if (msg.method)
					this.otherEvents.set(
						msg.method,
						(this.otherEvents.get(msg.method) ?? 0) + 1,
					);
				break;
		}
	}

	// Sends to the UI-process connection (Target domain lives here).
	sendRaw(method, params = {}) {
		const id = this.nextId++;
		return new Promise((resolve, reject) => {
			this.pending.set(id, { resolve, reject, method });
			this.ws.send(JSON.stringify({ id, method, params }));
		});
	}

	// Sends to the page target through Target.sendMessageToTarget. Rejects
	// after a timeout so probing dead target ids does not hang.
	send(method, params = {}, timeoutMs = 10_000) {
		const id = this.nextId++;
		return new Promise((resolve, reject) => {
			const timer = setTimeout(() => {
				if (this.pending.delete(id)) reject(new Error(`${method}: timeout`));
			}, timeoutMs);
			this.pending.set(id, {
				resolve: (v) => {
					clearTimeout(timer);
					resolve(v);
				},
				reject: (e) => {
					clearTimeout(timer);
					reject(e);
				},
				method,
			});
			const outerId = this.nextId++;
			this.pending.set(outerId, {
				resolve: () => {},
				reject: (e) => {
					if (this.pending.delete(id)) {
						clearTimeout(timer);
						reject(e);
					}
				},
				method: `Target.sendMessageToTarget(${method})`,
			});
			this.ws.send(
				JSON.stringify({
					id: outerId,
					method: "Target.sendMessageToTarget",
					params: {
						targetId: this.targetId,
						message: JSON.stringify({ id, method, params }),
					},
				}),
			);
		});
	}

	async start() {
		this.records = [];
		this.samples = [];
		this.cpu = [];
		this.otherEvents = new Map();
		await this.send("Timeline.start", { maxCallStackDepth: 24 });
		// JS sampling profiler (stack samples) and per-thread CPU usage; CPU
		// tells unattributed main-thread work apart from waiting on IPC.
		await this.send("ScriptProfiler.startTracking", { includeSamples: true });
		// CPU needs ENABLE(RESOURCE_USAGE); Playwright's Linux WebKit lacks it.
		this.hasCPU = await this.send("CPU.startTracking").then(
			() => true,
			() => false,
		);
	}

	async stop() {
		const done = new Promise((resolve) => {
			this.trackingDone = resolve;
			setTimeout(resolve, 15_000);
		});
		if (this.hasCPU) await this.send("CPU.stopTracking");
		await this.send("ScriptProfiler.stopTracking");
		await this.send("Timeline.stop");
		await done;
		return {
			records: this.records,
			samples: this.samples,
			cpu: this.cpu,
			otherEvents: Object.fromEntries(this.otherEvents),
		};
	}

	close() {
		this.ws?.close();
	}
}
