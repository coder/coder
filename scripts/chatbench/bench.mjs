// Chat rendering interaction benchmark.
//
// Drives the real production bundle (embedded coderd -> fakellm) in a
// real browser and measures main-thread responsiveness while two long chats
// stream side by side. Chromium runs also record a CDP CPU profile and a
// DevTools trace that can be opened in chrome://tracing or the Performance
// panel.
//
//   node scripts/chatbench/bench.mjs --browser chromium --scenario compare --label before
//   node scripts/chatbench/bench.mjs --browser webkit   --scenario compare --label before
//   node scripts/chatbench/bench.mjs --browser chromium --scenario smoke
//
// --react=1 injects the React DevTools backend (react-devtools-inline from
// $CHATBENCH_ROOT/tools) before the app loads and records a React DevTools
// profile per phase (commits, per-fiber durations, updaters). Requires a
// CODER_REACT_PROFILING=true site build; see up.sh. Summarize with
// react-summary.mjs. --react-changes=1 additionally records change
// descriptions ("why did this render"); DevTools implements that by
// re-executing every rendered function component twice per commit, which
// stalled this app for 50 s per streaming commit, so it is off by default.
// The profiling build also emits React's own "Scheduler" and "Components"
// tracks into the Chromium trace without the hook; see trace-react.mjs.
//
// Requires: scripts/chatbench/up.sh start && scripts/chatbench/seed.py
//
// Debugging aids for the real page, equivalent to editing in the Elements
// panel before measuring:
//   --prep=<a.js>[,<b.js>]     evaluated in the page in order after mount,
//                             before the phases; a step may report by
//                             assigning window.__prepResult (see prep/)
//   --init=<a.js>[,<b.js>]     added as init scripts before the app loads
//   --browser-env=K=V[,K=V]   extra environment for the browser process
//                             (for example WEBKIT_DISABLE_COMPOSITING_MODE=1)
//   --only=idle-focus         focus switching without streaming
//   --idle=<seconds>          length of the stream-idle phase (default 10)
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
	WebKitInspector,
	patchPlaywrightForForeignInspector,
} from "./wk-inspector.mjs";

// Playwright is a site devDependency; resolve it from there so this script
// can live next to the server-side harness pieces.
const REPO = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../..",
);
const requireFromSite = createRequire(path.join(REPO, "site/package.json"));
const { chromium, webkit } = requireFromSite("@playwright/test");
// playwright-core is a transitive dependency; resolve it through the
// `playwright` package that @playwright/test depends on.
const requireFromPlaywright = createRequire(
	createRequire(requireFromSite.resolve("@playwright/test")).resolve(
		"playwright/package.json",
	),
);

const args = Object.fromEntries(
	process.argv.slice(2).map((a) => {
		const [k, v = "true"] = a.replace(/^--/, "").split("=");
		return [k, v];
	}),
);
const browserName = args.browser ?? "chromium";
const scenario = args.scenario ?? "compare";
const label = args.label ?? "run";
const streamSeconds = Number(args.stream ?? 240);
const ROOT =
	process.env.CHATBENCH_ROOT ??
	path.join(
		process.env.XDG_CACHE_HOME ?? path.join(os.homedir(), ".cache"),
		"chatbench",
	);
const state = JSON.parse(
	fs.readFileSync(path.join(ROOT, "state.json"), "utf8"),
);
const outDir = path.join(
	ROOT,
	"results",
	`${label}-${browserName}-${scenario}`,
);
fs.mkdirSync(outDir, { recursive: true });

const API = "http://127.0.0.1:18080";
const WEB = state.web;
const [chatA, chatB] = state.chats;

// Transcript rows. The composer's testid (chat-message-input) shares the
// prefix, so exclude it.
const ROW_SELECTOR =
	'[data-testid^="chat-message-"]:not([data-testid="chat-message-input"])';

const log = (...a) => console.log(new Date().toISOString().slice(11, 23), ...a);

async function api(method, p, body) {
	const r = await fetch(API + p, {
		method,
		headers: {
			"Content-Type": "application/json",
			"Coder-Session-Token": state.token,
		},
		body: body ? JSON.stringify(body) : undefined,
	});
	if (!r.ok) throw new Error(`${method} ${p} -> ${r.status} ${await r.text()}`);
	return r.status === 204 ? null : r.json();
}

async function chatStatus(id) {
	return (await api("GET", `/api/v2/chats/${id}`)).status;
}

async function interruptAll() {
	for (const id of state.chats) {
		if ((await chatStatus(id)) === "running") {
			await api("POST", `/api/v2/chats/${id}/interrupt`).catch(() => {});
		}
	}
}

async function startStreams(seconds) {
	for (const id of state.chats) {
		await api("POST", `/api/v2/chats/${id}/messages`, {
			content: [
				{
					type: "text",
					text: `BENCH:stream ${seconds} keep going with the deep dive`,
				},
			],
		});
	}
}

// In-page instrumentation. Works in both engines: a rAF heartbeat records
// frames that arrive late (main thread was busy), and interactions record
// keydown -> next-frame-after-paint latency. Chromium additionally reports
// Event Timing entries (input to next paint per event).
const instrument = () => {
	const b = (window.__bench = {
		longFrames: [],
		interactions: [],
		eventTiming: [],
		marks: [],
		running: true,
	});
	let last = performance.now();
	const beat = (now) => {
		const gap = now - last;
		if (gap > 50)
			b.longFrames.push({ start: Math.round(last), dur: Math.round(gap) });
		last = now;
		if (b.running) requestAnimationFrame(beat);
	};
	requestAnimationFrame(beat);
	// Latency from the key event to the frame after the one that rendered it.
	document.addEventListener(
		"keydown",
		(e) => {
			const t0 = performance.now();
			const key = e.key;
			requestAnimationFrame(() => {
				requestAnimationFrame((t2) => {
					b.interactions.push({
						type: "key",
						key,
						t0: Math.round(t0),
						latency: Math.round(t2 - t0),
					});
				});
			});
		},
		{ capture: true },
	);
	document.addEventListener(
		"pointerdown",
		() => {
			const t0 = performance.now();
			requestAnimationFrame(() => {
				requestAnimationFrame((t2) => {
					b.interactions.push({
						type: "pointer",
						t0: Math.round(t0),
						latency: Math.round(t2 - t0),
					});
				});
			});
		},
		{ capture: true },
	);
	if (
		typeof PerformanceObserver !== "undefined" &&
		PerformanceObserver.supportedEntryTypes?.includes("event")
	) {
		new PerformanceObserver((list) => {
			for (const e of list.getEntries()) {
				b.eventTiming.push({
					name: e.name,
					start: Math.round(e.startTime),
					duration: Math.round(e.duration),
					processing: Math.round(e.processingEnd - e.processingStart),
					delay: Math.round(e.processingStart - e.startTime),
				});
			}
		}).observe({ type: "event", durationThreshold: 16, buffered: true });
	}
	b.mark = (name) => b.marks.push({ name, t: Math.round(performance.now()) });
	// Whole-document invalidation suspects: stylesheet add/remove, root or
	// body attribute changes, and font loading.
	b.domEvents = [];
	const mo = new MutationObserver((muts) => {
		for (const m of muts) {
			if (m.type === "attributes") {
				b.domEvents.push({
					t: Math.round(performance.now()),
					kind: `attr ${m.target.tagName.toLowerCase()}[${m.attributeName}]`,
					value: String(m.target.getAttribute(m.attributeName)).slice(0, 60),
				});
				continue;
			}
			for (const n of [...m.addedNodes, ...m.removedNodes]) {
				const tag = n.nodeName?.toLowerCase();
				const sheet =
					tag === "style" ||
					(tag === "link" && /stylesheet/i.test(n.getAttribute?.("rel") ?? ""))
						? n
						: n.querySelector?.("style, link[rel=stylesheet]");
				if (sheet) {
					b.domEvents.push({
						t: Math.round(performance.now()),
						kind: `${[...m.addedNodes].includes(n) ? "added" : "removed"} <${sheet.nodeName.toLowerCase()}> under ${m.target.tagName?.toLowerCase()}.${String(m.target.className).slice(0, 40)}`,
						value: (sheet.textContent ?? sheet.href ?? "").slice(0, 60),
					});
				}
			}
		}
	});
	const startObserving = () => {
		mo.observe(document.documentElement, {
			childList: true,
			subtree: true,
			attributes: true,
			attributeFilter: [
				"class",
				"style",
				"dir",
				"lang",
				"data-theme",
				"data-scroll-locked",
			],
		});
		document.fonts?.addEventListener("loadingdone", (e) =>
			b.domEvents.push({
				t: Math.round(performance.now()),
				kind: "fonts loadingdone",
				value: [...(e.fontfaces ?? [])]
					.map((f) => f.family)
					.join(",")
					.slice(0, 60),
			}),
		);
	};
	if (document.documentElement) startObserving();
	else
		document.addEventListener("DOMContentLoaded", startObserving, {
			once: true,
		});
};

function summarize(nums) {
	if (nums.length === 0) return { n: 0 };
	const s = [...nums].sort((a, b) => a - b);
	const q = (p) => s[Math.min(s.length - 1, Math.floor(p * s.length))];
	return {
		n: s.length,
		min: s[0],
		p50: q(0.5),
		p90: q(0.9),
		max: s[s.length - 1],
	};
}

// React DevTools backend as a page init script. The inline backend is a
// CommonJS bundle with `react` as an external, so wrap it with a module
// shim and a stub require (the external is only used by hook inspection,
// which the profiler does not need). Activation asks the (absent)
// frontend for saved preferences; answer with the DevTools defaults so the
// renderer attaches with host components filtered out, as in DevTools.
function reactDevtoolsInitScript() {
	const backend = path.join(
		ROOT,
		"tools/node_modules/react-devtools-inline/dist/backend.js",
	);
	if (!fs.existsSync(backend)) {
		throw new Error(
			`${backend} missing; run: mkdir -p ${ROOT}/tools && cd ${ROOT}/tools && npm init -y && npm install react-devtools-inline`,
		);
	}
	const src = fs.readFileSync(backend, "utf8");
	return `(() => {
		const module = { exports: {} };
		const require = (name) => {
			if (name === "react") return { __CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE: {} };
			throw new Error("react-devtools backend required " + name);
		};
		${src}
		const rdt = module.exports;
		rdt.initialize(window);
		let listener = null;
		const wall = {
			listen(fn) {
				listener = fn;
				return () => {
					listener = null;
				};
			},
			send(event) {
				if (event === "getSavedPreferences" && listener) {
					listener({
						event: "savedPreferences",
						payload: {
							appendComponentStack: false,
							breakOnConsoleErrors: false,
							componentFilters: [{ type: 1, value: 7, isEnabled: true }],
							showInlineWarningsAndErrors: false,
							hideConsoleLogsInStrictMode: false,
						},
					});
				}
			},
		};
		rdt.activate(window, { bridge: rdt.createBridge(window, wall) });
	})();`;
}

// Runs in the page: start a React DevTools profiling session on every
// attached renderer. Records the page time so commit timestamps (relative
// to the session start) can be aligned with the interaction log.
const reactProfileStart = (recordChangeDescriptions) => {
	const hook = window.__REACT_DEVTOOLS_GLOBAL_HOOK__;
	const interfaces = hook ? [...hook.rendererInterfaces.values()] : [];
	if (interfaces.length === 0)
		throw new Error(
			"no React renderer attached to the DevTools hook (is this a CODER_REACT_PROFILING build?)",
		);
	window.__reactProfileStart = performance.now();
	for (const ri of interfaces) ri.startProfiling(recordChangeDescriptions);
	return interfaces.length;
};

// Runs in the page: stop profiling and export the DevTools profiling data,
// resolving fiber ids to display names while the fibers are still mounted.
const reactProfileStop = () => {
	const hook = window.__REACT_DEVTOOLS_GLOBAL_HOOK__;
	const out = [];
	for (const ri of hook.rendererInterfaces.values()) {
		ri.stopProfiling();
		const data = ri.getProfilingData();
		const names = {};
		for (const root of data.dataForRoots) {
			for (const commit of root.commitData) {
				for (const [id] of commit.fiberSelfDurations) {
					if (!(id in names)) names[id] = ri.getDisplayNameForElementID(id);
				}
				if (commit.changeDescriptions) {
					for (const [id] of commit.changeDescriptions) {
						if (!(id in names)) names[id] = ri.getDisplayNameForElementID(id);
					}
				}
			}
		}
		out.push({
			...data,
			names,
			profilingStart: Math.round(window.__reactProfileStart),
		});
	}
	return out;
};

async function main() {
	const engine = browserName === "webkit" ? webkit : chromium;
	const wantProfile = args.profile !== "0";
	const WK_INSPECTOR_PORT = 9230;
	if (browserName === "webkit" && wantProfile)
		patchPlaywrightForForeignInspector(requireFromPlaywright);
	const browserEnv = Object.fromEntries(
		(args["browser-env"] ?? "")
			.split(",")
			.filter(Boolean)
			.map((kv) => kv.split("=")),
	);
	const browser = await engine.launch({
		headless: true,
		args: browserName === "chromium" ? ["--enable-precise-memory-info"] : [],
		env: {
			...process.env,
			...(browserName === "webkit" && wantProfile
				? { WEBKIT_INSPECTOR_HTTP_SERVER: `127.0.0.1:${WK_INSPECTOR_PORT}` }
				: {}),
			...browserEnv,
		},
	});
	const context = await browser.newContext({
		viewport: { width: 1800, height: 1000 },
	});
	await context.addCookies([
		{ name: "coder_session_token", value: state.token, url: WEB },
	]);
	const page = await context.newPage();
	page.setDefaultTimeout(120_000);
	page.on("pageerror", (e) => log("pageerror", e.message));
	page.on("response", async (r) => {
		if (r.status() >= 400 && r.url().includes("/api/")) {
			log(
				`http ${r.status()} ${r.request().method()} ${r.url()} ${(await r.text().catch(() => "")).slice(0, 200)}`,
			);
		}
		if (/\.(css|woff2?|ttf|otf)(\?|$)/.test(r.url())) {
			const t = await page
				.evaluate(() => Math.round(performance.now()))
				.catch(() => -1);
			log(`asset @${t}ms ${r.status()} ${r.url().split("/").pop()}`);
		}
	});
	page.on("console", (m) => {
		if (m.type() === "error") log("console.error", m.text().slice(0, 200));
	});
	const wantReact = args.react === "1";
	if (wantReact) await page.addInitScript(reactDevtoolsInitScript());
	// --init=<a.js>[,<b.js>]: scripts that must run before the app loads,
	// for example to wrap a browser API the app captures at mount.
	for (const file of (args.init ?? "").split(",").filter(Boolean)) {
		await page.addInitScript(fs.readFileSync(file, "utf8"));
	}
	await page.addInitScript(instrument);

	let cdp;
	let wk;
	let profiling = false;
	if (browserName === "chromium") {
		cdp = await context.newCDPSession(page);
		await cdp.send("Performance.enable");
	}

	const startProfile = async () => {
		if (wk) {
			await wk.start();
			profiling = true;
			return;
		}
		if (!cdp) return;
		await cdp.send("Profiler.enable");
		await cdp.send("Profiler.setSamplingInterval", { interval: 200 });
		await cdp.send("Profiler.start");
		await cdp.send("Tracing.start", {
			transferMode: "ReturnAsStream",
			traceConfig: {
				includedCategories: [
					"devtools.timeline",
					"disabled-by-default-devtools.timeline",
					"disabled-by-default-devtools.timeline.frame",
					"disabled-by-default-devtools.timeline.stack",
					// Emits one event per invalidated node; traces grow to
					// hundreds of MB when whole-document recalcs happen.
					...(args.invalidation === "1"
						? ["disabled-by-default-devtools.timeline.invalidationTracking"]
						: []),
					"blink.user_timing",
					"v8.execute",
					"disabled-by-default-v8.cpu_profiler",
					"loading",
					"latencyInfo",
				],
			},
		});
		profiling = true;
	};
	const stopProfile = async (name) => {
		if (wk && profiling) {
			profiling = false;
			const data = await wk.stop();
			fs.writeFileSync(
				path.join(outDir, `${name}.wktimeline.json`),
				JSON.stringify(data),
			);
			log(
				`saved ${name}.wktimeline.json (${data.records.length} records, ${data.samples.length} samples, ${data.cpu.length} cpu samples, other events ${JSON.stringify(data.otherEvents)})`,
			);
			return;
		}
		if (!cdp || !profiling) return;
		profiling = false;
		const { profile } = await cdp.send("Profiler.stop");
		fs.writeFileSync(
			path.join(outDir, `${name}.cpuprofile`),
			JSON.stringify(profile),
		);
		const done = new Promise((resolve) =>
			cdp.once("Tracing.tracingComplete", resolve),
		);
		await cdp.send("Tracing.end");
		const { stream } = await done;
		const chunks = [];
		for (;;) {
			const { data, base64Encoded, eof } = await cdp.send("IO.read", {
				handle: stream,
			});
			chunks.push(
				base64Encoded ? Buffer.from(data, "base64") : Buffer.from(data),
			);
			if (eof) break;
		}
		await cdp.send("IO.close", { handle: stream });
		fs.writeFileSync(
			path.join(outDir, `${name}.trace.json`),
			Buffer.concat(chunks),
		);
		log(`saved ${name}.cpuprofile and ${name}.trace.json`);
	};

	const url =
		scenario === "single"
			? `${WEB}/agents/${chatA}`
			: `${WEB}/agents/compare/${chatA}/${chatB}`;
	log("open", url);
	await interruptAll();
	await page.goto(url, { waitUntil: "domcontentloaded" });
	if (browserName === "webkit" && wantProfile) {
		wk = new WebKitInspector(WK_INSPECTOR_PORT);
		await wk.connect("/agents/");
		log("attached WebKit remote inspector");
	}
	const composers = page.getByRole("textbox", { name: "Chat message" });
	const wantComposers = scenario === "single" ? 1 : 2;
	await composers
		.nth(wantComposers - 1)
		.waitFor({ state: "visible", timeout: 60_000 });
	// All initially loaded rows for both chats.
	await page.waitForFunction(
		([selector, n]) => document.querySelectorAll(selector).length >= n,
		[ROW_SELECTOR, wantComposers * 20],
		{ timeout: 60_000 },
	);
	const rowCountNow = () => page.locator(ROW_SELECTOR).count();
	// Runs in the page: scroll every transcript container that holds rows
	// to the given edge.
	const scrollTranscripts = ([selector, edge]) => {
		const scrollers = new Set();
		for (const row of document.querySelectorAll(selector)) {
			let el = row.parentElement;
			while (el && el.scrollHeight <= el.clientHeight) el = el.parentElement;
			if (el) scrollers.add(el);
		}
		for (const s of scrollers)
			s.scrollTop = edge === "top" ? 0 : s.scrollHeight;
	};
	// Load the whole history of every pane: scroll each transcript to the
	// top until no more rows arrive. Every bench run adds a turn, so the
	// seeded turn count in state.json understates the history; stop on
	// no progress instead, with a generous page cap as a safety net.
	const loadFullHistory = async () => {
		for (let i = 0; i < 200; i++) {
			const before = await rowCountNow();
			await page.evaluate(scrollTranscripts, [ROW_SELECTOR, "top"]);
			try {
				await page.waitForFunction(
					([selector, n]) => document.querySelectorAll(selector).length > n,
					[ROW_SELECTOR, before],
					{ timeout: 20000 },
				);
			} catch {
				break;
			}
		}
		await page.evaluate(scrollTranscripts, [ROW_SELECTOR, "bottom"]);
	};
	if (args.history !== "page") await loadFullHistory();
	const rowCount = await rowCountNow();
	const nodeCount = await page.evaluate(
		() => document.getElementsByTagName("*").length,
	);
	log(`mounted: ${rowCount} rows, ${nodeCount} DOM nodes`);
	await page.screenshot({ path: path.join(outDir, "mounted.png") });
	if (args.prep) {
		for (const file of args.prep.split(",")) {
			const body = fs.readFileSync(file, "utf8");
			const result = await page.evaluate(
				`(() => { window.__prepResult = undefined;\n${body}\nreturn window.__prepResult; })()`,
			);
			log(`prep ${path.basename(file)}: ${JSON.stringify(result)}`);
		}
		// Stylesheet or DOM edits leave a whole-document style recalc
		// pending; flush it here so it is not charged to the first interaction.
		await page.evaluate(
			() =>
				new Promise((resolve) => {
					void document.body.offsetHeight;
					requestAnimationFrame(() => requestAnimationFrame(resolve));
				}),
		);
		await page.screenshot({ path: path.join(outDir, "prepped.png") });
	}
	if (scenario === "smoke") {
		await browser.close();
		return;
	}

	const results = {
		browser: browserName,
		scenario,
		label,
		rowCount,
		nodeCount,
		phases: {},
	};

	const resetBench = () =>
		page.evaluate(() => {
			window.__bench.longFrames = [];
			window.__bench.interactions = [];
			window.__bench.eventTiming = [];
			window.__bench.marks = [];
			window.__bench.domEvents = [];
			// Prep scripts may install per-phase probes, see
			// prep/geometry-probe.js.
			window.__bench.probeReset?.();
		});
	const collect = () =>
		page.evaluate(() => ({
			longFrames: window.__bench.longFrames,
			interactions: window.__bench.interactions,
			eventTiming: window.__bench.eventTiming,
			marks: window.__bench.marks,
			domEvents: window.__bench.domEvents,
			probe: window.__bench.probe?.(),
		}));

	const typeWord = async (composer, word) => {
		await composer.click();
		await page.evaluate(() => window.__bench.mark("type-start"));
		for (const ch of word) {
			await page.keyboard.type(ch, { delay: 0 });
			await page.waitForTimeout(120);
		}
		await page.evaluate(() => window.__bench.mark("type-end"));
	};
	const clearComposer = async (composer) => {
		await composer.click();
		await page.keyboard.press(
			process.platform === "darwin" ? "Meta+A" : "Control+A",
		);
		await page.keyboard.press("Backspace");
	};

	const only = args.only ? new Set(args.only.split(",")) : null;
	const phase = async (name, fn, { profile = false } = {}) => {
		if (only && !only.has(name)) return;
		log(`phase ${name}`);
		// Starting the profiler and tracing stalls the renderer for up to a
		// second; let that settle before the counters start.
		if (profile && wantProfile) await startProfile();
		await page.waitForTimeout(500);
		await resetBench();
		if (wantReact)
			await page.evaluate(reactProfileStart, args["react-changes"] === "1");
		const t0 = Date.now();
		await fn();
		const wall = Date.now() - t0;
		const raw = await collect();
		if (wantReact) {
			const data = await page.evaluate(reactProfileStop);
			const commits = data.reduce(
				(n, d) =>
					n + d.dataForRoots.reduce((m, r) => m + r.commitData.length, 0),
				0,
			);
			fs.writeFileSync(
				path.join(outDir, `${name}.reactprofile.json`),
				JSON.stringify({
					phase: name,
					interactions: raw.interactions,
					marks: raw.marks,
					renderers: data,
				}),
			);
			log(`saved ${name}.reactprofile.json (${commits} commits)`);
		}
		if (profile && wantProfile) await stopProfile(name);
		const keyLat = raw.interactions
			.filter((i) => i.type === "key")
			.map((i) => i.latency);
		const ptrLat = raw.interactions
			.filter((i) => i.type === "pointer")
			.map((i) => i.latency);
		const summary = {
			wallMs: wall,
			keyLatency: summarize(keyLat),
			pointerLatency: summarize(ptrLat),
			longFrames: {
				count: raw.longFrames.length,
				totalMs: raw.longFrames.reduce((a, f) => a + f.dur, 0),
				max: Math.max(0, ...raw.longFrames.map((f) => f.dur)),
			},
			eventTiming: summarize(raw.eventTiming.map((e) => e.duration)),
		};
		results.phases[name] = { summary, raw };
		const bigFrames = raw.longFrames
			.filter((f) => f.dur >= 300)
			.map((f) => `@${f.start}ms:${f.dur}ms`);
		if (bigFrames.length)
			log(`  frames >=300ms at page time: ${bigFrames.join(" ")}`);
		if (raw.probe) log(`  probe: ${JSON.stringify(raw.probe)}`);
		for (const f of raw.longFrames.filter((f) => f.dur >= 300)) {
			const near = raw.domEvents.filter(
				(e) => e.t >= f.start - 1500 && e.t <= f.start + f.dur,
			);
			for (const e of near)
				log(`    dom @${e.t}ms ${e.kind} ${JSON.stringify(e.value)}`);
		}
		log(
			`  ${name}: key p50=${summary.keyLatency.p50}ms p90=${summary.keyLatency.p90}ms max=${summary.keyLatency.max}ms | pointer p50=${summary.pointerLatency.p50}ms max=${summary.pointerLatency.max}ms | long frames ${summary.longFrames.count} (${summary.longFrames.totalMs}ms busy, max ${summary.longFrames.max}ms) in ${wall}ms`,
		);
	};

	const A = composers.nth(0);
	const B = composers.nth(wantComposers - 1);

	const focusSwitch = async () => {
		for (let i = 0; i < 4; i++) {
			await A.click();
			await page.waitForTimeout(400);
			await B.click();
			await page.waitForTimeout(400);
		}
	};

	// 0. Focus switching with nothing streaming (opt-in via --only).
	if (only?.has("idle-focus"))
		await phase("idle-focus", focusSwitch, { profile: true });

	// 0b. Hovering across transcript rows with nothing streaming (opt-in).
	// Long frames here mean :hover state changes are expensive.
	if (only?.has("idle-hover")) {
		await phase(
			"idle-hover",
			async () => {
				const box = await page.locator(ROW_SELECTOR).first().boundingBox();
				const vp = page.viewportSize();
				const x = (box?.x ?? 100) + 200;
				for (let i = 0; i < 12; i++) {
					await page.mouse.move(
						x + (i % 2) * 400,
						120 + ((i * 70) % (vp.height - 300)),
					);
					await page.waitForTimeout(250);
				}
			},
			{ profile: true },
		);
	}

	// 1. Idle baseline: nothing streaming.
	await phase(
		"idle-type",
		async () => {
			await typeWord(A, "when");
			if (wantComposers > 1) await typeWord(B, "when");
			await clearComposer(A);
			if (wantComposers > 1) await clearComposer(B);
		},
		{ profile: true },
	);

	// 2. Start both streams and let them run.
	if (only && ![...only].some((n) => n.startsWith("stream"))) {
		fs.writeFileSync(
			path.join(outDir, "results.json"),
			JSON.stringify(results, null, 1),
		);
		await browser.close();
		return;
	}
	await startStreams(streamSeconds);
	// Both turns must be running on the server before the streaming phases
	// start; the fake provider begins emitting within a second of that.
	const deadline = Date.now() + 30_000;
	let running = await Promise.all(state.chats.map(chatStatus));
	while (running.some((s) => s !== "running")) {
		if (Date.now() > deadline)
			throw new Error(`streams did not start: ${running.join(",")}`);
		await new Promise((r) => setTimeout(r, 250));
		running = await Promise.all(state.chats.map(chatStatus));
	}
	// Let the first tokens land and the streaming rows mount before
	// measuring, so stream-idle reflects steady-state streaming.
	await page.waitForTimeout(8000);
	log("chat statuses during stream:", running.join(","));

	// 3. Streaming, no interaction: cost of the streams alone.
	const idleSeconds = Number(args.idle ?? 10);
	await phase(
		"stream-idle",
		async () => {
			await page.waitForTimeout(idleSeconds * 1000);
		},
		{ profile: true },
	);

	// 4. Streaming + focus switching between the two composers.
	await phase("stream-focus", focusSwitch, { profile: true });

	// 5. Streaming + typing.
	await phase(
		"stream-type",
		async () => {
			await typeWord(A, "when");
			if (wantComposers > 1) await typeWord(B, "when");
			await page.waitForTimeout(500);
		},
		{ profile: true },
	);

	await page.screenshot({ path: path.join(outDir, "streaming.png") });
	await clearComposer(A);
	if (wantComposers > 1) await clearComposer(B);

	await interruptAll();
	fs.writeFileSync(
		path.join(outDir, "results.json"),
		JSON.stringify(results, null, 1),
	);
	const table = Object.entries(results.phases)
		.map(
			([n, p]) =>
				`${n.padEnd(14)} key p50/p90/max ${p.summary.keyLatency.p50 ?? "-"}/${p.summary.keyLatency.p90 ?? "-"}/${p.summary.keyLatency.max ?? "-"}ms | pointer p50/max ${p.summary.pointerLatency.p50 ?? "-"}/${p.summary.pointerLatency.max ?? "-"}ms | longframes ${p.summary.longFrames.count} busy ${p.summary.longFrames.totalMs}ms max ${p.summary.longFrames.max}ms`,
		)
		.join("\n");
	fs.writeFileSync(
		path.join(outDir, "summary.txt"),
		`${browserName} ${scenario} ${label}\nrows=${rowCount} nodes=${nodeCount}\n${table}\n`,
	);
	console.log(
		`\n=== ${browserName} ${scenario} ${label} (rows=${rowCount}, nodes=${nodeCount})\n${table}\nresults: ${outDir}`,
	);
	wk?.close();
	await browser.close();
}

main().catch(async (e) => {
	console.error(e);
	await interruptAll().catch(() => {});
	process.exit(1);
});
