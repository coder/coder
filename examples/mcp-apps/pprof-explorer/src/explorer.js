// pprof explorer view. Runs inside the MCP Apps sandbox iframe and talks to
// the host with raw JSON-RPC over postMessage. The server inlines this file
// as a module script, so there are no imports; the pure helpers below are
// exported for Node tests and the DOM bootstrap is guarded at the bottom.

export const PROTOCOL_VERSION = "2026-01-26";
export const EXPLAIN_MAX_CHARS = 1536;
export const MODEL_CONTEXT_MAX_CHARS = 1024;
export const IDLE_MS = 10000;
export const REQUEST_TIMEOUT_MS = 60000;
const ROW_LIMIT = 50;
const NAME_MAX_CHARS = 200;

const SAMPLE_TYPE_LABELS = {
	inuse_space: "in-use heap",
	inuse_objects: "in-use objects",
	alloc_space: "allocated bytes",
	alloc_objects: "allocated objects",
	cpu: "CPU time",
	samples: "CPU samples",
	delay: "delay",
	contentions: "contentions",
	goroutine: "goroutines",
};

const GOROUTINE_KINDS = { goroutine: true, goroutineleak: true };

/** Cuts a string to `max` characters, marking the cut with "...". */
export function clip(text, max) {
	const s = String(text == null ? "" : text);
	if (s.length <= max) return s;
	if (max <= 3) return s.slice(0, max);
	return s.slice(0, max - 3) + "...";
}

function fixed(n, digits) {
	return n.toFixed(digits);
}

function groupThousands(n) {
	const sign = n < 0 ? "-" : "";
	const digits = String(Math.round(Math.abs(n)));
	return sign + digits.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/**
 * Formats a sample value in its pprof unit: bytes as B/KiB/MiB/GiB,
 * nanoseconds as ns/us/ms/s, counts with thousands separators. The sign is
 * kept so diff deltas read naturally.
 */
export function formatValue(value, unit) {
	const n = Number(value) || 0;
	const sign = n < 0 ? "-" : "";
	const abs = Math.abs(n);
	if (unit === "bytes") {
		if (abs < 1024) return sign + Math.round(abs) + " B";
		if (abs < 1024 * 1024) return sign + fixed(abs / 1024, 1) + " KiB";
		if (abs < 1024 * 1024 * 1024) return sign + fixed(abs / (1024 * 1024), 1) + " MiB";
		return sign + fixed(abs / (1024 * 1024 * 1024), 2) + " GiB";
	}
	if (unit === "nanoseconds") {
		if (abs < 1e3) return sign + Math.round(abs) + " ns";
		if (abs < 1e6) return sign + fixed(abs / 1e3, 1) + " us";
		if (abs < 1e9) return sign + fixed(abs / 1e6, 1) + " ms";
		return sign + fixed(abs / 1e9, 2) + " s";
	}
	if (unit === "count" || !unit) {
		return groupThousands(n);
	}
	return groupThousands(n) + " " + unit;
}

/** Formats a percentage in the 0..100 range with one decimal. */
export function formatPct(pct) {
	const n = Number(pct) || 0;
	return fixed(n, 1) + "%";
}

/** Prefixes positive values with "+" so deltas carry their sign. */
export function formatSigned(value, unit) {
	const n = Number(value) || 0;
	const text = formatValue(n, unit);
	return n > 0 ? "+" + text : text;
}

/**
 * Splits a function name into chunks that may be separated by soft break
 * opportunities: after "." and "/" and before "(". Concatenating the chunks
 * yields the original name.
 */
export function softWrapName(name) {
	const s = String(name == null ? "" : name);
	const chunks = [];
	let start = 0;
	for (let i = 0; i < s.length; i++) {
		const c = s[i];
		if ((c === "." || c === "/") && i + 1 < s.length) {
			chunks.push(s.slice(start, i + 1));
			start = i + 1;
		} else if (c === "(" && i > start) {
			chunks.push(s.slice(start, i));
			start = i;
		}
	}
	if (start < s.length) chunks.push(s.slice(start));
	return chunks.length > 0 ? chunks : [s];
}

/**
 * Compiles a user focus string into a case-sensitive RegExp. Returns null
 * for an empty string and for patterns outside the accepted subset: at most
 * 128 characters, at most 3 quantifiers, no quantifier applied to a group,
 * no lookaround, no backreferences, and it must compile.
 */
export function compileFocus(focus) {
	const s = String(focus == null ? "" : focus).trim();
	if (!s || s.length > 128) return null;
	if (!focusPatternAllowed(s)) return null;
	try {
		return new RegExp(s);
	} catch (_err) {
		return null;
	}
}

// Scans the pattern once, skipping escapes and character classes. Rejects a
// quantifier that follows ")", more than 3 quantifiers, "(?=" "(?!" "(?<="
// "(?<!" lookarounds, and "\1".. "\9" or "\k<" backreferences.
function focusPatternAllowed(s) {
	let quantifiers = 0;
	let inClass = false;
	let prev = "";
	for (let i = 0; i < s.length; i++) {
		const c = s[i];
		if (c === "\\") {
			const next = s[i + 1] || "";
			if (!inClass && ((next >= "1" && next <= "9") || (next === "k" && s[i + 2] === "<"))) return false;
			i++;
			prev = "a";
			continue;
		}
		if (inClass) {
			if (c === "]") inClass = false;
			continue;
		}
		if (c === "[") {
			inClass = true;
			prev = "a";
			continue;
		}
		if (c === "(" && s[i + 1] === "?") {
			const rest = s.slice(i + 2, i + 4);
			if (rest[0] === "=" || rest[0] === "!" || rest === "<=" || rest === "<!") return false;
			i++;
			prev = "(";
			continue;
		}
		let isQuantifier = c === "+" || c === "*" || c === "?";
		if (c === "{") {
			const m = /^\{\d+(,\d*)?\}/.exec(s.slice(i));
			if (m) {
				isQuantifier = true;
				i += m[0].length - 1;
			}
		}
		if (isQuantifier) {
			if (prev === ")") return false;
			quantifiers++;
			if (quantifiers > 3) return false;
			prev = "q";
			continue;
		}
		prev = c;
	}
	return true;
}

/**
 * Applies the user's focus filter and sort to server diff rows. The filter
 * matches the function name or the file; sorting is by absolute delta of
 * the chosen column, largest first.
 */
export function computeDiffRows(rows, opts) {
	const list = Array.isArray(rows) ? rows.slice() : [];
	const o = opts || {};
	const re = compileFocus(o.focus);
	const filtered = re ? list.filter((r) => re.test(String(r.name || "")) || (r.file && re.test(String(r.file)))) : list;
	const key = o.sort === "cum" ? "delta_cum" : "delta_flat";
	filtered.sort((a, b) => Math.abs(Number(b[key]) || 0) - Math.abs(Number(a[key]) || 0));
	let maxAbs = 0;
	for (const r of filtered) {
		const v = Math.abs(Number(r[key]) || 0);
		if (v > maxAbs) maxAbs = v;
	}
	return { rows: filtered, maxAbs, key };
}

/** Renders an ISO timestamp as HH:MM:SSZ; anything unparseable is clipped. */
export function formatTimeUtc(iso) {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return clip(iso, 24);
	return d.toISOString().slice(11, 19) + "Z";
}

function formatTimeLocal(iso) {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return clip(iso, 8);
	const hh = String(d.getHours()).padStart(2, "0");
	const mm = String(d.getMinutes()).padStart(2, "0");
	const ss = String(d.getSeconds()).padStart(2, "0");
	return hh + ":" + mm + ":" + ss;
}

function sampleTypeLabel(sampleType) {
	return SAMPLE_TYPE_LABELS[sampleType] || String(sampleType || "samples");
}

function responsibleFor(unit) {
	if (unit === "bytes") return "holding the memory";
	if (unit === "nanoseconds") return "consuming the time";
	if (unit === "count") return "producing the count";
	return "responsible";
}

function pctOf(value, total) {
	const t = Number(total) || 0;
	if (t <= 0) return 0;
	return (Number(value) || 0) * 100 / t;
}

/**
 * Builds the Explain message for one target. `kind` is one of "row",
 * "diffRow", "group", "edge", "profile". `ctx.snapshot` carries
 * {profile_id, kind, captured_at, sample_type, unit, total}; `ctx.base` is
 * optional {profile_id, kind, captured_at}. The message is capped at
 * EXPLAIN_MAX_CHARS; the data block is cut first so the ask and the final
 * instruction always survive.
 */
export function buildExplainMessage(kind, ctx) {
	const snap = (ctx && ctx.snapshot) || {};
	const base = (ctx && ctx.base) || null;
	const id = String(snap.profile_id || "?");
	const unit = snap.unit || "";
	const stLabel = sampleTypeLabel(snap.sample_type);
	const name = (s) => clip(s, NAME_MAX_CHARS);
	const quoted = (s) => '"' + name(s) + '"';

	const data = [
		"  snapshot " + id + ", " + (snap.kind || "?") + ", " + (snap.sample_type || "?") +
			", captured " + formatTimeUtc(snap.captured_at) + ", total " + formatValue(snap.total, unit),
	];
	if (base && base.profile_id) {
		data.push("  base " + base.profile_id + " (" + (base.kind || "?") + ", " + formatTimeUtc(base.captured_at) + ")");
	}

	let ask = "";
	let goal = "";
	const tools = ["top", "callers_callees"];
	if (base && base.profile_id) tools.push('diff_profiles against "' + base.profile_id + '"');
	if (GOROUTINE_KINDS[snap.kind]) tools.push("goroutine_groups");

	if (kind === "row") {
		const row = ctx.row || {};
		ask = "Explain why the function " + quoted(row.name) + " accounts for " + formatPct(row.flat_pct) + " of " + stLabel + " in " + id + ".";
		data.push("  row: " + name(row.name));
		data.push("    flat " + formatValue(row.flat, unit) + " (" + formatPct(row.flat_pct) + "), cum " +
			formatValue(row.cum, unit) + " (" + formatPct(row.cum_pct) + ")");
		if (row.file) data.push("    " + clip(row.file, NAME_MAX_CHARS) + (row.line ? ":" + row.line : ""));
		goal = "what this row means, what is likely " + responsibleFor(unit) + ", and what to check next";
	} else if (kind === "diffRow") {
		const row = ctx.row || {};
		const baseId = base && base.profile_id ? base.profile_id : "the base";
		ask = "Explain why the function " + quoted(row.name) + " changed by " + formatSigned(row.delta_flat, unit) + " of " + stLabel +
			" between " + baseId + " and " + id + ".";
		data.push("  row: " + name(row.name));
		data.push("    flat: base " + formatValue(row.base_flat, unit) + ", current " + formatValue(row.flat, unit) +
			", delta " + formatSigned(row.delta_flat, unit));
		data.push("    cum: base " + formatValue(row.base_cum, unit) + ", current " + formatValue(row.cum, unit) +
			", delta " + formatSigned(row.delta_cum, unit));
		goal = "what changed between the two snapshots, what is likely " + responsibleFor(unit) + ", and what to check next";
	} else if (kind === "group") {
		const g = ctx.group || {};
		const count = Number(g.count) || 0;
		ask = "Explain what " + count + " goroutines at the function " + quoted(g.top_frame) + " in " + id + " are doing and whether they are leaked.";
		data.push("  group: " + count + " goroutines (" + formatPct(pctOf(count, snap.total)) + " of " + formatValue(snap.total, "count") + ")");
		const labels = g.labels && typeof g.labels === "object" ? Object.keys(g.labels) : [];
		if (labels.length > 0) {
			data.push("    labels: " + labels.map((k) => clip(k, 64) + "=" + clip(g.labels[k], 64)).join(", "));
		}
		const frames = Array.isArray(g.frames) ? g.frames : [];
		const shown = frames.slice(0, 8);
		if (shown.length > 0) data.push("    stack (innermost first):");
		for (const f of shown) {
			const loc = f.file ? " " + clip(f.file, NAME_MAX_CHARS) + (f.line ? ":" + f.line : "") : "";
			data.push("      " + name(f.name) + loc);
		}
		if (frames.length > shown.length) data.push("      (+" + (frames.length - shown.length) + " more frames)");
		goal = "what these goroutines are waiting on, whether this is a leak, and what to check next";
	} else if (kind === "edge") {
		const e = ctx.edge || {};
		const fn = e.function || {};
		const other = name(e.name);
		const self = name(fn.name);
		const weight = formatValue(e.weight, unit) + " (" + formatPct(pctOf(e.weight, snap.total)) + ")";
		if (e.direction === "caller") {
			ask = "Explain why the function " + quoted(e.name) + " calls into " + quoted(fn.name) + " for " + weight + " of " + stLabel + " in " + id + ".";
			data.push("  edge: " + other + " -> " + self + ", weight " + weight);
		} else {
			ask = "Explain why the function " + quoted(fn.name) + " calls into " + quoted(e.name) + " for " + weight + " of " + stLabel + " in " + id + ".";
			data.push("  edge: " + self + " -> " + other + ", weight " + weight);
		}
		data.push("  function: " + self + ", flat " + formatValue(fn.flat, unit) + " (" + formatPct(pctOf(fn.flat, snap.total)) +
			"), cum " + formatValue(fn.cum, unit) + " (" + formatPct(pctOf(fn.cum, snap.total)) + ")");
		goal = "how this call path contributes, what is likely " + responsibleFor(unit) + ", and what to check next";
	} else {
		ask = "Assess the " + (snap.kind || "?") + " profile " + id + " (" + (snap.sample_type || "?") + ") and point out anything pathological.";
		const top = Array.isArray(ctx && ctx.top) ? ctx.top.slice(0, 3) : [];
		if (top.length > 0) data.push("  top rows by flat:");
		for (const r of top) {
			data.push("    " + name(r.name) + " flat " + formatValue(r.flat, unit) + " (" + formatPct(r.flat_pct) + "), cum " +
				formatValue(r.cum, unit) + " (" + formatPct(r.cum_pct) + ")");
		}
		goal = "the main hotspots, whether anything looks pathological, and what to check next";
	}

	const instruction = 'Use the pprof explorer tools on profile_id "' + id + '" (' + tools.join(", ") +
		") to investigate, then explain " + goal + ".";
	return capMessage(clip(ask, 480), "Profile data (verbatim from the profiled process):\n" + data.join("\n"), instruction, EXPLAIN_MAX_CHARS);
}

function capMessage(ask, block, instruction, max) {
	const sep = "\n\n";
	let text = ask + sep + block + sep + instruction;
	if (text.length <= max) return text;
	const marker = "\n  [truncated]";
	const budget = max - ask.length - instruction.length - sep.length * 2 - marker.length;
	if (budget > 0) {
		text = ask + sep + block.slice(0, budget) + marker + sep + instruction;
		if (text.length <= max) return text;
	}
	// The ask and the instruction alone do not fit: split the budget.
	const half = Math.floor((max - sep.length) / 2);
	return clip(ask, half) + sep + clip(instruction, max - sep.length - half);
}

/**
 * Builds the one-line text sent with ui/update-model-context. Carries only
 * snapshot ids, the effective view settings, and the selected row.
 */
export function buildModelContext(state) {
	const s = state || {};
	const profiles = Array.isArray(s.profiles) ? s.profiles : [];
	const byId = (id) => profiles.find((p) => p.profile_id === id);
	if (!s.selectedId) {
		const ids = profiles.slice(0, 12).map((p) => p.profile_id + " " + p.kind).join(", ");
		const more = profiles.length > 12 ? ", ..." : "";
		return clip("pprof explorer: no snapshot selected; " + profiles.length + " snapshot" +
			(profiles.length === 1 ? "" : "s") + " available" + (ids ? " (" + ids + more + ")" : "") + ".", MODEL_CONTEXT_MAX_CHARS);
	}
	const snap = byId(s.selectedId) || {};
	const kind = snap.kind || "?";
	const st = s.sampleType || "default type";
	const parts = [];
	if (s.mode === "diff" && s.baseId) {
		parts.push("Viewing diff " + s.baseId + " -> " + s.selectedId + " (" + kind + ", " + st + ", sort " + (s.sort || "flat") +
			(s.focus ? ', filter "' + clip(s.focus, 64) + '"' : "") + ")");
	} else if (s.mode === "groups") {
		parts.push("Viewing " + s.selectedId + " goroutine groups (" + kind +
			(s.groupCount != null ? ", " + s.groupCount + " groups" : "") + ")");
	} else {
		parts.push("Viewing " + s.selectedId + " (" + kind + ", " + st + ", sort " + (s.sort || "flat") +
			(s.focus ? ', filter "' + clip(s.focus, 64) + '"' : "") + ")");
	}
	const row = s.selectedRow;
	if (row && row.name) {
		if (s.mode === "diff") {
			parts.push("selected " + clip(row.name, NAME_MAX_CHARS) + " delta " + formatSigned(row.delta_flat, s.unit));
		} else if (s.mode === "groups") {
			parts.push("selected group " + clip(row.name, NAME_MAX_CHARS) + " x" + (row.count || 0));
		} else {
			parts.push("selected " + clip(row.name, NAME_MAX_CHARS) + " flat " + formatValue(row.flat, s.unit) + " (" + formatPct(row.flat_pct) + ")");
		}
	}
	if (s.baseId && s.mode !== "diff") parts.push("base " + s.baseId);
	return clip(parts.join("; ") + ".", MODEL_CONTEXT_MAX_CHARS);
}

/** Short description of an agent tool result for the banner. */
export function describeAgentResult(sc) {
	if (!sc || typeof sc !== "object") return "";
	const id = sc.profile_id || "?";
	switch (sc.view) {
		case "top":
			return "top of " + id + " (" + (sc.sample_type || "default") + ", sort " + (sc.sort || "flat") +
				(sc.focus ? ', filter "' + clip(sc.focus, 40) + '"' : "") + ")";
		case "callers":
			return "callers of " + clip(sc.function && sc.function.name, 80) + " in " + id;
		case "diff":
			return "diff " + (sc.base_id || "?") + " -> " + id;
		case "groups":
			return "goroutine groups of " + id;
		default:
			return String(sc.view || "result") + " of " + id;
	}
}

/**
 * Maps tool-call arguments to the tool name they belong to, by argument
 * shape: `kind` is a capture and returns null (captures are never re-run),
 * `base_id` a diff, `function` a callers lookup, sort/focus/sample_type a
 * top; a bare `profile_id` is goroutine_groups for goroutine snapshots and
 * top otherwise; no arguments is list_profiles.
 */
export function inferToolFromArgs(args, kindOfProfile) {
	const a = args && typeof args === "object" ? args : {};
	if (typeof a.kind === "string") return null;
	if (typeof a.base_id === "string") return "diff_profiles";
	if (typeof a.function === "string") return "callers_callees";
	if (typeof a.sort === "string" || typeof a.focus === "string" || typeof a.sample_type === "string") return "top";
	if (typeof a.profile_id === "string") return GOROUTINE_KINDS[kindOfProfile] ? "goroutine_groups" : "top";
	return "list_profiles";
}

function textOfResult(result) {
	const content = result && Array.isArray(result.content) ? result.content : [];
	return content.filter((c) => c && c.type === "text" && typeof c.text === "string").map((c) => c.text).join(" ").trim();
}

// Bridge

/**
 * JSON-RPC over postMessage to the parent frame. Messages are accepted only
 * from window.parent; the origin of the first accepted message is pinned
 * and every later message must come from it and is posted back to it. Only
 * the initial ui/initialize request goes out before the origin is known.
 * Requests reject after REQUEST_TIMEOUT_MS without a response.
 */
function createBridge(win, handlers) {
	let targetOrigin = null;
	let nextId = 1;
	const pending = new Map();

	function post(msg) {
		win.parent.postMessage(msg, targetOrigin || "*");
	}

	function request(method, params) {
		const id = nextId++;
		return new Promise((resolve, reject) => {
			const timer = setTimeout(() => {
				if (!pending.delete(id)) return;
				reject(new Error(method + " timed out after " + REQUEST_TIMEOUT_MS / 1000 + "s"));
			}, REQUEST_TIMEOUT_MS);
			pending.set(id, { resolve, reject, timer });
			post({ jsonrpc: "2.0", id, method, params: params || {} });
		});
	}

	function notify(method, params) {
		post({ jsonrpc: "2.0", method, params: params || {} });
	}

	function respond(id, result) {
		post({ jsonrpc: "2.0", id, result });
	}

	function respondError(id, code, message) {
		post({ jsonrpc: "2.0", id, error: { code, message } });
	}

	function isJsonRpc(data) {
		if (!data || typeof data !== "object" || data.jsonrpc !== "2.0") return false;
		if (typeof data.method === "string") return true;
		return (typeof data.id === "number" || typeof data.id === "string") &&
			(Object.hasOwn(data, "result") || Object.hasOwn(data, "error"));
	}

	win.addEventListener("message", (event) => {
		if (event.source !== win.parent) return;
		if (targetOrigin === null) {
			if (typeof event.origin !== "string" || event.origin === "" || event.origin === "null") return;
			targetOrigin = event.origin;
		} else if (event.origin !== targetOrigin) {
			return;
		}
		const data = event.data;
		if (!isJsonRpc(data)) return;

		if (data.method === undefined) {
			const p = pending.get(data.id);
			if (!p) return;
			pending.delete(data.id);
			clearTimeout(p.timer);
			if (data.error) p.reject(new Error((data.error && data.error.message) || "Request failed"));
			else p.resolve(data.result);
			return;
		}

		if (data.id !== undefined) {
			if (data.method === "ui/resource-teardown") {
				handlers.onTeardown && handlers.onTeardown(data.params || {});
				respond(data.id, {});
			} else {
				respondError(data.id, -32601, "Method not found");
			}
			return;
		}
		handlers.onNotification && handlers.onNotification(data.method, data.params || {});
	});

	return { request, notify };
}

// App

/**
 * Mounts the explorer into `doc` and connects the bridge through `win`
 * (`win.parent.postMessage` and `win.addEventListener("message")`). The
 * document must already contain the explorer.html markup.
 */
export function mountExplorer(doc, win) {
	const $ = (id) => doc.getElementById(id);
	const el = {
		target: $("target"),
		captures: $("captures"),
		targetStatus: $("target-status"),
		chips: $("chips"),
		baseLine: $("base-line"),
		baseId: $("base-id"),
		clearBase: $("clear-base"),
		banner: $("banner"),
		bannerText: $("banner-text"),
		bannerShow: $("banner-show"),
		bannerDismiss: $("banner-dismiss"),
		controls: $("controls"),
		sampleType: $("sample-type"),
		sort: $("sort"),
		filter: $("filter"),
		modes: $("modes"),
		explainProfile: $("explain-profile"),
		explainStatus: $("explain-status"),
		notice: $("notice"),
		report: $("report"),
		drill: $("drill"),
	};

	const state = {
		target: "",
		profiles: [],
		selectedId: null,
		baseId: null,
		sampleType: null,
		sort: "flat",
		focus: "",
		mode: "top",
		report: null,
		unit: "",
		drill: null,
		selectedRow: null,
		targetStatus: "",
		loading: 0,
		error: "",
		info: "",
		agentBanner: null,
		lastToolInput: null,
		explainPending: false,
		explainStatus: "",
		lastInteraction: 0,
		tornDown: false,
		confirmDropId: null,
	};
	let reportSeq = 0;
	let drillSeq = 0;
	let filterTimer = null;
	let confirmDropTimer = null;

	// helpers

	const profile = (id) => state.profiles.find((p) => p.profile_id === id) || null;
	const selected = () => profile(state.selectedId);
	const idle = () => Date.now() - state.lastInteraction > IDLE_MS;
	// A selection whose report is still loading is not empty.
	const viewEmpty = () => !state.selectedId || (!state.report && state.loading === 0);

	function sampleTypesOf(p) {
		if (!p) return [];
		if (Array.isArray(p.sample_types) && p.sample_types.length > 0) return p.sample_types.map((s) => s.type);
		return p.totals && typeof p.totals === "object" ? Object.keys(p.totals) : [];
	}

	function availableModes() {
		const p = selected();
		const base = profile(state.baseId);
		const modes = ["top"];
		if (p && base && base.profile_id !== p.profile_id && base.kind === p.kind) modes.push("diff");
		if (p && GOROUTINE_KINDS[p.kind]) modes.push("groups");
		return modes;
	}

	function defaultMode() {
		const modes = availableModes();
		const p = selected();
		if (p && GOROUTINE_KINDS[p.kind]) return "groups";
		if (modes.includes("diff")) return "diff";
		return "top";
	}

	function idNum(id) {
		const m = /^p(\d+)$/.exec(String(id));
		return m ? Number(m[1]) : Number.MAX_SAFE_INTEGER;
	}

	// Keeps only the snapshot fields shared by the list and capture shapes.
	function toProfile(p) {
		const out = { profile_id: p.profile_id, kind: p.kind, captured_at: p.captured_at, totals: p.totals || {} };
		if (typeof p.label === "string") out.label = p.label;
		if (Array.isArray(p.sample_types)) out.sample_types = p.sample_types;
		if (typeof p.default_sample_type === "string") out.default_sample_type = p.default_sample_type;
		return out;
	}

	function upsertProfile(p) {
		if (!p || typeof p.profile_id !== "string") return;
		const next = toProfile(p);
		const existing = profile(p.profile_id);
		if (existing) Object.assign(existing, next);
		else state.profiles.push(next);
		state.profiles.sort((a, b) => idNum(a.profile_id) - idNum(b.profile_id));
	}

	function removeProfile(id) {
		state.profiles = state.profiles.filter((p) => p.profile_id !== id);
		if (state.baseId === id) state.baseId = null;
		if (state.selectedId === id) {
			state.selectedId = null;
			state.report = null;
			state.drill = null;
			state.selectedRow = null;
		}
	}

	function setError(text) {
		state.error = text || "";
		renderNotice();
	}

	function snapshotCtx() {
		const p = selected() || {};
		const r = state.report || {};
		return {
			profile_id: p.profile_id,
			kind: p.kind,
			captured_at: p.captured_at,
			sample_type: r.sample_type || state.sampleType || p.default_sample_type,
			unit: r.unit || state.unit || (state.mode === "groups" ? "count" : ""),
			total: r.total != null ? r.total : (p.totals && r.sample_type ? p.totals[r.sample_type] : undefined),
		};
	}

	function baseCtx() {
		const b = profile(state.baseId);
		return b ? { profile_id: b.profile_id, kind: b.kind, captured_at: b.captured_at } : null;
	}

	// bridge

	const bridge = createBridge(win, {
		onNotification: handleNotification,
		onTeardown: () => {
			state.tornDown = true;
		},
	});

	async function callTool(name, args) {
		const result = await bridge.request("tools/call", { name, arguments: args || {} });
		if (result && result.isError) {
			throw new Error(textOfResult(result) || name + " failed");
		}
		return result || {};
	}

	function syncModelContext() {
		if (state.tornDown) return;
		const text = buildModelContext({
			profiles: state.profiles,
			selectedId: state.selectedId,
			baseId: state.baseId,
			mode: state.mode,
			sampleType: (state.report && state.report.sample_type) || state.sampleType,
			sort: state.sort,
			focus: state.focus,
			selectedRow: state.selectedRow,
			unit: state.unit,
			groupCount: state.mode === "groups" && state.report ? (state.report.groups || []).length : null,
		});
		bridge.request("ui/update-model-context", { content: [{ type: "text", text }] }).catch(() => {});
	}

	function applyTheme(theme) {
		if (theme === "light" || theme === "dark") doc.documentElement.setAttribute("data-theme", theme);
	}

	// data loading

	async function refreshList() {
		try {
			const result = await callTool("list_profiles", {});
			const sc = result.structuredContent;
			if (!sc || sc.view !== "list") return;
			applyList(sc);
		} catch (err) {
			setError("list_profiles: " + err.message);
		}
	}

	function applyList(sc) {
		if (typeof sc.target === "string") state.target = sc.target;
		const seen = new Set();
		for (const p of Array.isArray(sc.profiles) ? sc.profiles : []) {
			upsertProfile(p);
			seen.add(p.profile_id);
		}
		for (const p of state.profiles.slice()) {
			if (!seen.has(p.profile_id)) removeProfile(p.profile_id);
		}
		renderAll();
	}

	async function loadReport() {
		const p = selected();
		if (!p) {
			state.report = null;
			renderAll();
			return;
		}
		const seq = ++reportSeq;
		state.loading++;
		setError("");
		renderReport();
		try {
			let sc;
			if (state.mode === "diff" && state.baseId) {
				const args = { base_id: state.baseId, profile_id: p.profile_id, limit: ROW_LIMIT };
				if (state.sampleType) args.sample_type = state.sampleType;
				sc = (await callTool("diff_profiles", args)).structuredContent;
			} else if (state.mode === "groups") {
				sc = (await callTool("goroutine_groups", { profile_id: p.profile_id, limit: ROW_LIMIT })).structuredContent;
			} else {
				const args = { profile_id: p.profile_id, sort: state.sort, limit: ROW_LIMIT };
				if (state.sampleType) args.sample_type = state.sampleType;
				if (state.focus) args.focus = state.focus;
				sc = (await callTool("top", args)).structuredContent;
			}
			if (seq !== reportSeq) return;
			applyReport(sc);
		} catch (err) {
			if (seq !== reportSeq) return;
			state.report = null;
			setError(err.message);
		} finally {
			state.loading--;
			renderAll();
			syncModelContext();
		}
	}

	function applyReport(sc) {
		if (!sc || typeof sc !== "object") {
			state.report = null;
			return;
		}
		state.report = sc;
		if (sc.view === "groups") {
			state.mode = "groups";
			state.unit = "count";
		} else if (sc.view === "diff") {
			state.mode = "diff";
			state.unit = sc.unit || state.unit;
		} else {
			state.mode = "top";
			state.unit = sc.unit || state.unit;
		}
		if (typeof sc.sample_type === "string") state.sampleType = sc.sample_type;
		const row = state.selectedRow;
		if (row) {
			const rows = sc.rows || sc.groups || [];
			const still = rows.some((r) => (r.name || r.top_frame) === row.name);
			if (!still) {
				state.selectedRow = null;
				state.drill = null;
			}
		}
	}

	async function loadDrill(fnName) {
		const p = selected();
		if (!p) return;
		const seq = ++drillSeq;
		state.drill = { loading: true, function: { name: fnName } };
		renderDrill();
		try {
			const args = { profile_id: p.profile_id, function: fnName };
			if (state.sampleType) args.sample_type = state.sampleType;
			const sc = (await callTool("callers_callees", args)).structuredContent;
			if (seq !== drillSeq) return;
			state.drill = sc && sc.view === "callers" ? sc : null;
			if (sc && sc.unit) state.unit = sc.unit;
		} catch (err) {
			if (seq !== drillSeq) return;
			state.drill = null;
			setError(err.message);
		}
		renderDrill();
	}

	async function capture(kind, seconds) {
		const buttons = el.captures.querySelectorAll("button");
		buttons.forEach((b) => { b.disabled = true; });
		setError("");
		state.info = "Capturing " + kind + (seconds ? " for " + seconds + "s" : "") + "...";
		renderNotice();
		try {
			const args = { kind };
			if (seconds) args.seconds = seconds;
			const sc = (await callTool("capture_profile", args)).structuredContent;
			if (sc && sc.view === "capture") {
				upsertProfile(sc);
				if (typeof sc.target_status === "string") state.targetStatus = sc.target_status;
				select(sc.profile_id, { sampleType: sc.default_sample_type || null });
			} else {
				await refreshList();
			}
		} catch (err) {
			setError("capture " + kind + ": " + err.message);
		} finally {
			state.info = "";
			buttons.forEach((b) => { b.disabled = false; });
			renderAll();
		}
	}

	async function drop(id) {
		clearTimeout(confirmDropTimer);
		state.confirmDropId = null;
		setError("");
		try {
			const sc = (await callTool("drop_profile", { profile_id: id })).structuredContent;
			removeProfile(sc && sc.profile_id ? sc.profile_id : id);
			renderAll();
			syncModelContext();
		} catch (err) {
			setError("drop " + id + ": " + err.message);
			renderChips();
		}
	}

	// First click on a trash button arms it for 3 seconds; the second click
	// within that window drops the snapshot.
	function armDrop(id) {
		clearTimeout(confirmDropTimer);
		state.confirmDropId = id;
		confirmDropTimer = setTimeout(() => {
			state.confirmDropId = null;
			renderChips();
		}, 3000);
		renderChips();
	}

	function select(id, opts) {
		const o = opts || {};
		const changed = state.selectedId !== id;
		state.selectedId = id;
		if (changed) {
			state.selectedRow = null;
			state.drill = null;
			state.report = null;
			state.sampleType = o.sampleType !== undefined ? o.sampleType : null;
		}
		if (o.mode && availableModes().includes(o.mode)) state.mode = o.mode;
		else if (changed || !availableModes().includes(state.mode)) state.mode = defaultMode();
		renderAll();
		loadReport();
	}

	function setBase(id) {
		const before = state.mode;
		state.baseId = state.baseId === id ? null : id;
		if (!availableModes().includes(state.mode)) state.mode = defaultMode();
		else if (state.baseId && availableModes().includes("diff") && state.mode === "top") state.mode = "diff";
		if (state.mode !== before) {
			state.selectedRow = null;
			state.drill = null;
			renderAll();
			loadReport();
		} else {
			renderAll();
			syncModelContext();
		}
	}

	function applyFocus(value) {
		const next = String(value || "").trim();
		if (next === state.focus) return;
		state.focus = next;
		if (state.mode === "diff") {
			renderAll();
			syncModelContext();
		} else {
			loadReport();
		}
	}

	// follow the agent

	function handleNotification(method, params) {
		if (method === "ui/notifications/host-context-changed") {
			applyTheme(params.theme);
		} else if (method === "ui/notifications/tool-input") {
			state.lastToolInput = params.arguments && typeof params.arguments === "object" ? params.arguments : {};
		} else if (method === "ui/notifications/tool-result") {
			handleAgentResult(params);
		} else if (method === "ui/notifications/tool-cancelled") {
			state.agentBanner = { text: "Agent tool call cancelled" + (params.reason ? ": " + clip(params.reason, 80) : ""), apply: null };
			renderBanner();
		}
	}

	function handleAgentResult(result) {
		const args = state.lastToolInput || {};
		if (result && result.isError) {
			setError("Agent tool call failed: " + (textOfResult(result) || "unknown error"));
			syncModelContext();
			return;
		}
		const sc = result && result.structuredContent;
		if (!sc || typeof sc !== "object") {
			reissue(args);
			return;
		}
		switch (sc.view) {
			case "capture": {
				upsertProfile(sc);
				if (typeof sc.target_status === "string") state.targetStatus = sc.target_status;
				if (!state.selectedId) {
					select(sc.profile_id, { sampleType: sc.default_sample_type || null });
					return;
				}
				state.agentBanner = {
					text: "Agent captured " + sc.profile_id + " " + (sc.kind || ""),
					apply: () => select(sc.profile_id, { sampleType: sc.default_sample_type || null }),
				};
				renderAll();
				syncModelContext();
				return;
			}
			case "list":
				applyList(sc);
				syncModelContext();
				return;
			case "dropped":
				removeProfile(sc.profile_id);
				renderAll();
				syncModelContext();
				return;
			case "top":
			case "diff":
			case "callers":
			case "groups":
				break;
			default:
				return;
		}
		const text = "Agent viewed " + describeAgentResult(sc);
		if (viewEmpty() || idle()) {
			state.agentBanner = { text, apply: null };
			renderBanner();
			applyAgentView(sc);
			return;
		}
		state.agentBanner = { text, apply: () => applyAgentView(sc) };
		renderBanner();
		syncModelContext();
	}

	async function applyAgentView(sc) {
		const id = sc.profile_id;
		if (typeof id !== "string") return;
		if (!profile(id) || (sc.view === "diff" && !profile(sc.base_id))) await refreshList();
		if (!profile(id)) return;
		// Any response still in flight for the previous report or drill-down
		// is stale from here on and must not overwrite the applied state.
		reportSeq++;
		drillSeq++;
		if (sc.view === "diff" && profile(sc.base_id)) state.baseId = sc.base_id;
		const mode = sc.view === "callers" ? undefined : sc.view;
		const changed = state.selectedId !== id;
		state.selectedId = id;
		if (changed) {
			state.selectedRow = null;
			state.drill = null;
			state.report = null;
		}
		if (typeof sc.sample_type === "string") state.sampleType = sc.sample_type;
		if (mode && availableModes().includes(mode)) state.mode = mode;
		else if (!availableModes().includes(state.mode)) state.mode = defaultMode();

		if (sc.view === "callers") {
			state.drill = sc;
			state.selectedRow = { name: sc.function && sc.function.name, flat: sc.function && sc.function.flat, cum: sc.function && sc.function.cum };
			if (sc.unit) state.unit = sc.unit;
			renderAll();
			if (!state.report) loadReport();
			else syncModelContext();
			return;
		}
		const sameSettings = sc.view !== "top" || ((sc.sort || "flat") === state.sort && (sc.focus || "") === state.focus);
		if (sameSettings) {
			applyReport(sc);
			renderAll();
			syncModelContext();
		} else {
			renderAll();
			loadReport();
		}
	}

	async function reissue(args) {
		const id = args && typeof args.profile_id === "string" ? args.profile_id : null;
		if (id && !profile(id)) await refreshList();
		const tool = inferToolFromArgs(args, id ? (profile(id) || {}).kind : undefined);
		if (tool === null) {
			await refreshList();
			syncModelContext();
			return;
		}
		try {
			const result = await callTool(tool, args);
			handleAgentResult(result);
		} catch (err) {
			setError("Agent result was too large to display and re-running " + tool + " failed: " + err.message);
		}
	}

	// explain

	async function explain(kind, ctx) {
		if (state.explainPending || state.tornDown) return;
		state.explainPending = true;
		state.explainStatus = "Waiting for confirmation...";
		renderExplainState();
		const text = buildExplainMessage(kind, Object.assign({ snapshot: snapshotCtx(), base: baseCtx() }, ctx));
		try {
			await bridge.request("ui/message", { role: "user", content: [{ type: "text", text }] });
			state.explainStatus = "Sent to the agent";
		} catch (err) {
			state.explainStatus = "Not sent: " + clip(err.message, 80);
		} finally {
			state.explainPending = false;
			renderExplainState();
			syncModelContext();
		}
	}

	function explainButton(kind, ctx, label) {
		const b = doc.createElement("button");
		b.type = "button";
		b.className = "explain";
		b.textContent = label || "Explain";
		b.disabled = state.explainPending;
		b.addEventListener("click", (e) => {
			e.stopPropagation();
			explain(kind, ctx);
		});
		return b;
	}

	// rendering

	function nameNode(name, extraClass) {
		const span = doc.createElement("span");
		span.className = "name mono" + (extraClass ? " " + extraClass : "");
		const chunks = softWrapName(name);
		chunks.forEach((chunk, i) => {
			if (i > 0) span.appendChild(doc.createElement("wbr"));
			span.appendChild(doc.createTextNode(chunk));
		});
		return span;
	}

	function cell(cls, text) {
		const d = doc.createElement("span");
		d.className = cls;
		d.textContent = text;
		return d;
	}

	// Makes a non-button element keyboard operable: focusable, with the given
	// role, activated by click, Enter, or Space. Clicks on nested buttons
	// (Explain) are left to the button.
	function activatable(node, role, onActivate) {
		node.setAttribute("tabindex", "0");
		node.setAttribute("role", role);
		node.addEventListener("click", (e) => {
			if (e.target && e.target.closest && e.target.closest("button") && e.target !== node) return;
			onActivate();
		});
		node.addEventListener("keydown", (e) => {
			if (e.target !== node) return;
			if (e.key === "Enter" || e.key === " " || e.key === "Spacebar") {
				e.preventDefault();
				onActivate();
			}
		});
		return node;
	}

	function renderAll() {
		el.target.textContent = state.target || "unknown";
		el.targetStatus.hidden = !state.targetStatus;
		el.targetStatus.textContent = state.targetStatus;
		renderChips();
		renderBanner();
		renderControls();
		renderNotice();
		renderReport();
		renderDrill();
	}

	function trashIcon() {
		const ns = "http://www.w3.org/2000/svg";
		const svg = doc.createElementNS(ns, "svg");
		svg.setAttribute("viewBox", "0 0 24 24");
		svg.setAttribute("aria-hidden", "true");
		const path = doc.createElementNS(ns, "path");
		path.setAttribute("d", "M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3");
		svg.appendChild(path);
		return svg;
	}

	function renderChips() {
		el.chips.textContent = "";
		if (state.profiles.length === 0) {
			const empty = doc.createElement("span");
			empty.className = "muted";
			empty.textContent = "no snapshots yet; capture one above";
			el.chips.appendChild(empty);
		}
		for (const p of state.profiles) {
			const chip = doc.createElement("span");
			chip.className = "chip" + (p.profile_id === state.selectedId ? " selected" : "") + (p.profile_id === state.baseId ? " base" : "");
			const sel = doc.createElement("button");
			sel.type = "button";
			sel.className = "chip-select";
			sel.textContent = p.profile_id + " " + p.kind + " " + formatTimeLocal(p.captured_at);
			sel.title = (p.label ? p.label + " " : "") + String(p.captured_at || "");
			sel.setAttribute("aria-pressed", p.profile_id === state.selectedId ? "true" : "false");
			sel.addEventListener("click", () => select(p.profile_id));
			const base = doc.createElement("button");
			base.type = "button";
			base.className = "chip-base";
			base.textContent = "base";
			const isBase = p.profile_id === state.baseId;
			const isSelected = p.profile_id === state.selectedId;
			if (isSelected && !isBase) {
				base.disabled = true;
				base.title = "Select another " + p.kind + " snapshot to diff against " + p.profile_id;
			} else {
				base.title = isBase ? "Clear base" : "Set " + p.profile_id + " as diff base";
			}
			base.setAttribute("aria-pressed", isBase ? "true" : "false");
			base.addEventListener("click", () => setBase(p.profile_id));
			const del = doc.createElement("button");
			del.type = "button";
			if (state.confirmDropId === p.profile_id) {
				del.className = "chip-drop confirm";
				del.textContent = "drop?";
				del.setAttribute("aria-label", "Confirm dropping " + p.profile_id);
				del.title = "Click again to drop " + p.profile_id;
				del.addEventListener("click", () => drop(p.profile_id));
			} else {
				del.className = "chip-drop";
				del.setAttribute("aria-label", "Drop " + p.profile_id);
				del.title = "Drop " + p.profile_id;
				del.appendChild(trashIcon());
				del.addEventListener("click", () => armDrop(p.profile_id));
			}
			chip.appendChild(sel);
			chip.appendChild(base);
			chip.appendChild(del);
			el.chips.appendChild(chip);
		}
		el.baseLine.hidden = !state.baseId;
		el.baseId.textContent = state.baseId || "";
	}

	function renderBanner() {
		const b = state.agentBanner;
		el.banner.hidden = !b;
		if (!b) return;
		el.bannerText.textContent = b.text;
		el.bannerShow.hidden = !b.apply;
	}

	function renderControls() {
		const p = selected();
		el.controls.hidden = !p;
		if (!p) return;
		const types = sampleTypesOf(p);
		const current = (state.report && state.report.sample_type) || state.sampleType || p.default_sample_type || "";
		const existing = Array.from(el.sampleType.options).map((o) => o.value);
		if (existing.join("\n") !== types.join("\n")) {
			el.sampleType.textContent = "";
			for (const t of types) {
				const o = doc.createElement("option");
				o.value = t;
				o.textContent = t;
				el.sampleType.appendChild(o);
			}
		}
		if (current && types.includes(current)) el.sampleType.value = current;
		el.sampleType.disabled = types.length <= 1 || state.mode === "groups";
		el.sort.value = state.sort;
		el.sort.disabled = state.mode === "groups";
		el.filter.disabled = state.mode === "groups";
		if (el.filter.value.trim() !== state.focus && doc.activeElement !== el.filter) el.filter.value = state.focus;
		const modes = availableModes();
		for (const b of el.modes.querySelectorAll("button")) {
			const m = b.dataset.mode;
			b.hidden = !modes.includes(m);
			b.classList.toggle("active", m === state.mode);
		}
		renderExplainState();
	}

	function renderExplainState() {
		el.explainStatus.textContent = state.explainStatus;
		for (const b of doc.querySelectorAll("button.explain")) b.disabled = state.explainPending;
		if (!state.report) el.explainProfile.disabled = true;
	}

	function renderNotice() {
		const text = state.error || state.info;
		el.notice.hidden = !text;
		el.notice.className = state.error ? "notice error" : "notice";
		el.notice.textContent = text;
	}

	function renderReport() {
		el.report.textContent = "";
		const p = selected();
		if (!p) {
			if (state.profiles.length > 0) {
				const n = doc.createElement("div");
				n.className = "notice";
				n.textContent = "Select a snapshot to view it.";
				el.report.appendChild(n);
			}
			return;
		}
		if (!state.report) {
			const n = doc.createElement("div");
			n.className = "notice";
			n.textContent = state.loading > 0 ? "Loading..." : (state.error ? "" : "No data.");
			if (n.textContent) el.report.appendChild(n);
			return;
		}
		const r = state.report;
		if (r.view === "groups") renderGroups(r);
		else if (r.view === "diff") renderDiff(r);
		else renderTop(r);
		if (r.truncated) {
			const shown = Array.isArray(r.groups) ? r.groups.length : (Array.isArray(r.rows) ? r.rows.length : 0);
			const t = doc.createElement("div");
			t.className = "truncated";
			t.textContent = "Showing the first " + shown + " entries; narrow with the filter.";
			el.report.appendChild(t);
		}
	}

	function header(cols) {
		const h = doc.createElement("div");
		h.className = "thead";
		for (const c of cols) h.appendChild(cell(c[0], c[1]));
		return h;
	}

	function rowShell(name, pct, barVar, selectedName) {
		const row = doc.createElement("div");
		const isSelected = Boolean(selectedName && selectedName === name);
		row.className = "row" + (isSelected ? " selected" : "");
		row.setAttribute("aria-selected", isSelected ? "true" : "false");
		const w = Math.max(0, Math.min(100, pct));
		if (w > 0) row.style.background = "linear-gradient(90deg, var(" + barVar + ") " + w.toFixed(1) + "%, transparent " + w.toFixed(1) + "%)";
		return row;
	}

	function selectRow(row) {
		state.selectedRow = row;
		renderReport();
		loadDrill(row.name);
		syncModelContext();
	}

	function renderTop(r) {
		const unit = r.unit || state.unit;
		const rows = Array.isArray(r.rows) ? r.rows : [];
		const table = doc.createElement("div");
		table.setAttribute("role", "table");
		table.appendChild(header([["c-flat", "flat"], ["c-flatp", "flat%"], ["c-cum", "cum"], ["c-cump", "cum%"], ["c-name", "function"], ["c-act", ""]]));
		const selName = state.selectedRow && state.selectedRow.name;
		for (const row of rows) {
			const div = rowShell(row.name, Number(row.flat_pct) || 0, "--bar", selName);
			const metrics = doc.createElement("span");
			metrics.className = "metrics";
			metrics.appendChild(cell("c-flat", formatValue(row.flat, unit)));
			metrics.appendChild(cell("c-flatp", formatPct(row.flat_pct)));
			metrics.appendChild(cell("c-cum", formatValue(row.cum, unit)));
			metrics.appendChild(cell("c-cump", formatPct(row.cum_pct)));
			const nameCell = doc.createElement("span");
			nameCell.className = "c-name";
			nameCell.appendChild(nameNode(row.name));
			if (row.file) {
				const f = doc.createElement("span");
				f.className = "c-file mono";
				f.textContent = row.file + (row.line ? ":" + row.line : "");
				nameCell.appendChild(f);
			}
			const act = doc.createElement("span");
			act.className = "c-act";
			act.appendChild(explainButton("row", { row }));
			// Metrics come first so grid auto-placement matches the header
			// columns; the narrow layout positions cells by grid-area instead.
			div.appendChild(metrics);
			div.appendChild(nameCell);
			div.appendChild(act);
			activatable(div, "row", () => selectRow(row));
			table.appendChild(div);
		}
		if (rows.length === 0) {
			const n = doc.createElement("div");
			n.className = "notice";
			n.textContent = state.focus ? "No functions match the filter." : "No samples.";
			table.appendChild(n);
		}
		el.report.appendChild(table);
	}

	function renderDiff(r) {
		const unit = r.unit || state.unit;
		const computed = computeDiffRows(r.rows, { sort: state.sort, focus: state.focus });
		const table = doc.createElement("div");
		table.className = "diff";
		table.setAttribute("role", "table");
		table.appendChild(header([["c-flat", "flat base > cur"], ["c-flatp", "delta"], ["c-cum", "cum base > cur"], ["c-cump", "delta"], ["c-name", "function"], ["c-act", ""]]));
		const selName = state.selectedRow && state.selectedRow.name;
		for (const row of computed.rows) {
			const delta = Number(row[computed.key]) || 0;
			const pct = computed.maxAbs > 0 ? Math.abs(delta) * 100 / computed.maxAbs : 0;
			const div = rowShell(row.name, pct, delta >= 0 ? "--bar-up" : "--bar-down", selName);
			const metrics = doc.createElement("span");
			metrics.className = "metrics";
			metrics.appendChild(cell("c-flat", formatValue(row.base_flat, unit) + " > " + formatValue(row.flat, unit)));
			metrics.appendChild(cell("c-flatp", formatSigned(row.delta_flat, unit)));
			metrics.appendChild(cell("c-cum", formatValue(row.base_cum, unit) + " > " + formatValue(row.cum, unit)));
			metrics.appendChild(cell("c-cump", formatSigned(row.delta_cum, unit)));
			const nameCell = doc.createElement("span");
			nameCell.className = "c-name";
			nameCell.appendChild(nameNode(row.name));
			const act = doc.createElement("span");
			act.className = "c-act";
			act.appendChild(explainButton("diffRow", { row }));
			div.appendChild(metrics);
			div.appendChild(nameCell);
			div.appendChild(act);
			activatable(div, "row", () => selectRow(row));
			table.appendChild(div);
		}
		if (computed.rows.length === 0) {
			const n = doc.createElement("div");
			n.className = "notice";
			n.textContent = state.focus ? "No functions match the filter." : "No differences.";
			table.appendChild(n);
		}
		el.report.appendChild(table);
	}

	function renderGroups(r) {
		const groups = Array.isArray(r.groups) ? r.groups : [];
		const total = Number(r.total) || 0;
		const list = doc.createElement("div");
		const summary = doc.createElement("div");
		summary.className = "muted";
		summary.textContent = total + " goroutines in " + groups.length + " group" + (groups.length === 1 ? "" : "s");
		list.appendChild(summary);
		const selName = state.selectedRow && state.selectedRow.name;
		groups.forEach((g, index) => {
			const div = doc.createElement("div");
			div.className = "group";
			const pct = total > 0 ? (Number(g.count) || 0) * 100 / total : 0;
			if (pct > 0) div.style.background = "linear-gradient(90deg, var(--bar) " + pct.toFixed(1) + "%, transparent " + pct.toFixed(1) + "%)";
			const head = doc.createElement("div");
			head.className = "group-head";
			const badge = doc.createElement("span");
			badge.className = "badge";
			badge.textContent = String(g.count || 0);
			head.appendChild(badge);
			head.appendChild(nameNode(g.top_frame));
			head.appendChild(explainButton("group", { group: g }));
			div.appendChild(head);
			const labels = g.labels && typeof g.labels === "object" ? Object.keys(g.labels) : [];
			if (labels.length > 0) {
				const wrap = doc.createElement("div");
				wrap.className = "labels";
				for (const k of labels) {
					const l = doc.createElement("span");
					l.className = "label";
					l.textContent = k + "=" + g.labels[k];
					wrap.appendChild(l);
				}
				div.appendChild(wrap);
			}
			const expanded = selName === g.top_frame && state.selectedRow && state.selectedRow.index === index;
			if (expanded) {
				const ul = doc.createElement("ul");
				ul.className = "frames";
				for (const f of Array.isArray(g.frames) ? g.frames : []) {
					const li = doc.createElement("li");
					li.appendChild(nameNode(f.name));
					if (f.file) {
						const loc = doc.createElement("span");
						loc.className = "muted";
						loc.textContent = " " + f.file + (f.line ? ":" + f.line : "");
						li.appendChild(loc);
					}
					ul.appendChild(li);
				}
				div.appendChild(ul);
			}
			head.setAttribute("aria-expanded", expanded ? "true" : "false");
			activatable(head, "button", () => {
				state.selectedRow = expanded ? null : { name: g.top_frame, count: g.count, index };
				renderReport();
				syncModelContext();
			});
			list.appendChild(div);
		});
		if (groups.length === 0) {
			const n = doc.createElement("div");
			n.className = "notice";
			n.textContent = "No goroutines.";
			list.appendChild(n);
		}
		el.report.appendChild(list);
	}

	function renderDrill() {
		const d = state.drill;
		el.drill.hidden = !d || state.mode === "groups";
		el.drill.textContent = "";
		if (!d || state.mode === "groups") return;
		const fn = d.function || {};
		const unit = d.unit || state.unit;
		const head = doc.createElement("div");
		head.className = "drill-head";
		head.appendChild(nameNode(fn.name));
		if (!d.loading) {
			const m = doc.createElement("span");
			m.className = "muted";
			m.textContent = "flat " + formatValue(fn.flat, unit) + ", cum " + formatValue(fn.cum, unit);
			head.appendChild(m);
			head.appendChild(explainButton("row", {
				row: { name: fn.name, flat: fn.flat, cum: fn.cum, flat_pct: pctOf(fn.flat, d.total), cum_pct: pctOf(fn.cum, d.total) },
			}));
		} else {
			const m = doc.createElement("span");
			m.className = "muted";
			m.textContent = "Loading...";
			head.appendChild(m);
		}
		const close = doc.createElement("button");
		close.type = "button";
		close.setAttribute("aria-label", "Close drill-down");
		close.textContent = "x";
		close.addEventListener("click", () => {
			state.drill = null;
			state.selectedRow = null;
			renderReport();
			renderDrill();
			syncModelContext();
		});
		head.appendChild(close);
		el.drill.appendChild(head);
		if (d.loading) return;
		const cols = doc.createElement("div");
		cols.className = "drill-cols";
		cols.appendChild(edgeList("callers", d.callers, "caller", fn, unit));
		cols.appendChild(edgeList("callees", d.callees, "callee", fn, unit));
		el.drill.appendChild(cols);
	}

	function edgeList(title, edges, direction, fn, unit) {
		const col = doc.createElement("div");
		const h = doc.createElement("h4");
		h.textContent = title;
		col.appendChild(h);
		const list = Array.isArray(edges) ? edges : [];
		if (list.length === 0) {
			const n = doc.createElement("div");
			n.className = "muted";
			n.textContent = "none";
			col.appendChild(n);
		}
		for (const e of list) {
			const row = doc.createElement("div");
			row.className = "edge";
			const n = nameNode(e.name, "edge-name");
			n.title = "Drill into " + e.name;
			activatable(n, "button", () => {
				state.selectedRow = { name: e.name };
				renderReport();
				loadDrill(e.name);
				syncModelContext();
			});
			row.appendChild(n);
			row.appendChild(cell("weight", formatValue(e.weight, unit)));
			row.appendChild(explainButton("edge", { edge: { direction, function: fn, name: e.name, weight: e.weight } }));
			col.appendChild(row);
		}
		return col;
	}

	// wiring

	const markInteraction = () => {
		state.lastInteraction = Date.now();
	};
	doc.addEventListener("pointerdown", markInteraction, true);
	doc.addEventListener("click", markInteraction, true);
	doc.addEventListener("keydown", markInteraction, true);
	doc.addEventListener("input", markInteraction, true);

	el.captures.addEventListener("click", (e) => {
		const b = e.target.closest("button[data-kind]");
		if (!b) return;
		capture(b.dataset.kind, b.dataset.seconds ? Number(b.dataset.seconds) : undefined);
	});
	el.clearBase.addEventListener("click", () => setBase(state.baseId));
	el.bannerDismiss.addEventListener("click", () => {
		state.agentBanner = null;
		renderBanner();
	});
	el.bannerShow.addEventListener("click", () => {
		const b = state.agentBanner;
		state.agentBanner = null;
		renderBanner();
		if (b && b.apply) b.apply();
	});
	el.sampleType.addEventListener("change", () => {
		state.sampleType = el.sampleType.value || null;
		loadReport();
	});
	el.sort.addEventListener("change", () => {
		state.sort = el.sort.value === "cum" ? "cum" : "flat";
		if (state.mode === "diff") {
			renderAll();
			syncModelContext();
		} else {
			loadReport();
		}
	});
	el.filter.addEventListener("input", () => {
		clearTimeout(filterTimer);
		filterTimer = setTimeout(() => applyFocus(el.filter.value), 400);
	});
	el.filter.addEventListener("keydown", (e) => {
		if (e.key === "Enter") {
			e.preventDefault();
			clearTimeout(filterTimer);
			applyFocus(el.filter.value);
		}
	});
	el.modes.addEventListener("click", (e) => {
		const b = e.target.closest("button[data-mode]");
		if (!b || !availableModes().includes(b.dataset.mode) || b.dataset.mode === state.mode) return;
		state.mode = b.dataset.mode;
		state.selectedRow = null;
		state.drill = null;
		loadReport();
	});
	el.explainProfile.addEventListener("click", () => {
		const r = state.report || {};
		let top = [];
		if (r.view === "groups") {
			const total = Number(r.total) || 0;
			top = (r.groups || []).map((g) => ({ name: g.top_frame, flat: g.count, flat_pct: pctOf(g.count, total), cum: g.count, cum_pct: pctOf(g.count, total) }));
		} else if (r.view === "diff") {
			top = (r.rows || []).map((row) => ({ name: row.name, flat: row.flat, flat_pct: pctOf(row.flat, r.total), cum: row.cum, cum_pct: pctOf(row.cum, r.total) }));
		} else {
			top = r.rows || [];
		}
		explain("profile", { top });
	});

	renderAll();

	bridge.request("ui/initialize", {
		appInfo: { name: "pprof-explorer", version: "0.1.0" },
		appCapabilities: {},
		protocolVersion: PROTOCOL_VERSION,
	}).then((result) => {
		bridge.notify("ui/notifications/initialized", {});
		applyTheme(result && result.hostContext && result.hostContext.theme);
		refreshList().then(syncModelContext);
	}).catch((err) => {
		setError("Initialize failed: " + err.message);
	});
}

if (typeof document !== "undefined") {
	mountExplorer(document, window);
}
