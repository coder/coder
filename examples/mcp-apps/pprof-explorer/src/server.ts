import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import {
	RESOURCE_MIME_TYPE,
	registerAppResource,
	registerAppTool,
} from "@modelcontextprotocol/ext-apps/server";
import { hostHeaderValidation, toNodeHandler } from "@modelcontextprotocol/node";
import {
	type CallToolResult,
	McpServer,
	createMcpHandler,
} from "@modelcontextprotocol/server";
import { z } from "zod";
import { fetchProfile, fetchTargetStatus } from "./fetch.js";
import {
	type DiffRow,
	type Edge,
	type GoroutineGroup,
	MAX_LIMIT,
	type TopRow,
	callersCallees,
	clampLimit,
	diff,
	formatPct,
	formatValue,
	goroutineGroups,
	resolveSampleType,
	sampleTypes,
	top,
	totals,
} from "./report.js";
import { normalizeString, MAX_FUNCTION_NAME_LENGTH } from "./sanitize.js";
import {
	PROFILE_KINDS,
	type ProfileKind,
	type Snapshot,
	SnapshotStore,
	serializeCpuCapture,
} from "./store.js";

export const EXPLORER_URI = "ui://pprof/explorer";
export const SCRIPT_PLACEHOLDER = "<!-- EXPLORER_SCRIPT -->";
export const RESULT_BUDGET_BYTES = 56 * 1024;
const TEXT_ROWS = 10;
const DEFAULT_TARGET = "http://127.0.0.1:6060";

/**
 * Validates PPROF_TARGET: an absolute http or https URL with a host. The
 * returned string has no trailing slash. Throws on anything else.
 */
export function parseTarget(raw: string): string {
	let url: URL;
	try {
		url = new URL(raw);
	} catch {
		throw new Error(`PPROF_TARGET "${raw}" is not a valid URL`);
	}
	if (url.protocol !== "http:" && url.protocol !== "https:") {
		throw new Error(`PPROF_TARGET "${raw}" must use http or https`);
	}
	if (url.hostname === "") {
		throw new Error(`PPROF_TARGET "${raw}" has no host`);
	}
	if (url.search !== "" || url.hash !== "") {
		throw new Error(`PPROF_TARGET "${raw}" must not carry a query or fragment`);
	}
	return url.toString().replace(/\/+$/, "");
}

export interface ServerOptions {
	target: string;
	store?: SnapshotStore;
	/** Directory holding explorer.html and explorer.js. */
	viewDir?: string;
}

// Module-level store so every stateless request shares the same snapshots.
const sharedStore = new SnapshotStore();
const defaultViewDir = dirname(fileURLToPath(import.meta.url));

// Tools carry _meta.ui so hosts render the explorer resource alongside
// results. visibility "model" lets the agent call the tool, "app" lets the
// view call it through the host proxy.
const uiMeta = {
	ui: { resourceUri: EXPLORER_URI, visibility: ["model", "app"] as const },
};
const appOnlyMeta = {
	ui: { resourceUri: EXPLORER_URI, visibility: ["app"] as const },
};

const profileIdSchema = z
	.string()
	.regex(/^p\d+$/, "profile ids look like p1, p2, ...");
const sampleTypeSchema = z
	.string()
	.max(64)
	.optional()
	.describe(
		"Sample type name from the snapshot's sample_types, for example inuse_space or alloc_objects. Defaults to the profile's default sample type.",
	);
const limitSchema = z
	.number()
	.int()
	.min(1)
	.max(MAX_LIMIT)
	.optional()
	.describe(`Maximum rows to return (default 50, max ${MAX_LIMIT}).`);

function textResult(
	text: string,
	structuredContent: Record<string, unknown>,
): CallToolResult {
	return { content: [{ type: "text", text }], structuredContent };
}

function errorResult(
	message: string,
	extra: Record<string, unknown> = {},
): CallToolResult {
	return {
		content: [{ type: "text", text: message }],
		structuredContent: { view: "error", message, ...extra },
		isError: true,
	};
}

function errorMessage(err: unknown): string {
	return err instanceof Error ? err.message : String(err);
}

function resultBytes(result: CallToolResult): number {
	return Buffer.byteLength(JSON.stringify(result), "utf8");
}

/**
 * Builds a result from a list of items, shrinking the list until the
 * serialized result fits RESULT_BUDGET_BYTES. The builder receives the items
 * to include and whether anything was cut (from the list passed here or by
 * the report that produced it).
 */
function budgeted<T>(
	items: T[],
	alreadyTruncated: boolean,
	build: (items: T[], truncated: boolean) => CallToolResult,
): CallToolResult {
	let count = items.length;
	let result = build(items, alreadyTruncated);
	while (resultBytes(result) > RESULT_BUDGET_BYTES && count > 0) {
		count = Math.floor(count * 0.75);
		result = build(items.slice(0, count), true);
	}
	return result;
}

function describeSnapshot(s: Snapshot): string {
	const label = s.label ? ` "${s.label}"` : "";
	return `${s.id} (${s.kind}${label}, captured ${s.capturedAt.toISOString()})`;
}

function snapshotSummary(s: Snapshot): Record<string, unknown> {
	const types = sampleTypes(s.profile);
	return {
		profile_id: s.id,
		kind: s.kind,
		captured_at: s.capturedAt.toISOString(),
		label: s.label ?? null,
		sample_types: types,
		default_sample_type: resolveSampleType(s.profile).type,
		totals: totals(s.profile),
	};
}

function formatTotals(s: Snapshot): string {
	const types = sampleTypes(s.profile);
	const sums = totals(s.profile);
	return types
		.map((t) => `${t.type} ${formatValue(sums[t.type] ?? 0, t.unit)}`)
		.join(", ");
}

function topRowsText(rows: TopRow[], unit: string): string {
	if (rows.length === 0) {
		return "  (no rows)";
	}
	return rows
		.slice(0, TEXT_ROWS)
		.map(
			(r) =>
				`  ${formatValue(r.flat, unit)} ${formatPct(r.flat_pct)}  ${formatValue(r.cum, unit)} ${formatPct(r.cum_pct)}  ${r.name}${r.file ? ` (${r.file}:${r.line})` : ""}`,
		)
		.join("\n");
}

function edgesText(edges: Edge[], unit: string, total: number): string {
	if (edges.length === 0) {
		return "  (none)";
	}
	return edges
		.slice(0, TEXT_ROWS)
		.map(
			(e) =>
				`  ${formatValue(e.weight, unit)} ${formatPct(pctOf(e.weight, total))}  ${e.name}`,
		)
		.join("\n");
}

function pctOf(value: number, total: number): number {
	return total === 0 ? 0 : (value / total) * 100;
}

function signed(value: number, unit: string): string {
	return value > 0 ? `+${formatValue(value, unit)}` : formatValue(value, unit);
}

function diffRowsText(rows: DiffRow[], unit: string): string {
	if (rows.length === 0) {
		return "  (no differences)";
	}
	return rows
		.slice(0, TEXT_ROWS)
		.map(
			(r) =>
				`  flat ${signed(r.delta_flat, unit)} (${formatValue(r.base_flat, unit)} -> ${formatValue(r.flat, unit)})  cum ${signed(r.delta_cum, unit)}  ${r.name}`,
		)
		.join("\n");
}

function groupsText(groups: GoroutineGroup[]): string {
	if (groups.length === 0) {
		return "  (no goroutines)";
	}
	return groups
		.slice(0, TEXT_ROWS)
		.map((g) => {
			const labels = Object.entries(g.labels)
				.map(([k, v]) => `${k}=${v}`)
				.join(" ");
			const stack = g.frames
				.slice(0, 4)
				.map((f) => f.name)
				.join(" <- ");
			const more = g.frame_count > 4 ? ` <- ... (${g.frame_count} frames)` : "";
			return `  ${g.count}  ${stack}${more}${labels ? `  [${labels}]` : ""}`;
		})
		.join("\n");
}

const kindDescriptions: Record<ProfileKind, string> = {
	heap: "live heap (inuse_space/inuse_objects; alloc_* are cumulative)",
	allocs: "allocations since process start (alloc_space/alloc_objects)",
	goroutine: "all goroutine stacks",
	goroutineleak: "goroutines the runtime considers leaked (Go 1.25+)",
	profile: "CPU samples over `seconds`",
	block: "blocking events, cumulative since process start",
	mutex: "mutex contention at Unlock sites, cumulative since process start",
};

/**
 * Creates an McpServer with the pprof tools and the explorer resource. The
 * store defaults to the process-wide shared store; tests pass their own.
 */
export function buildServer(options: ServerOptions): McpServer {
	const target = options.target;
	const store = options.store ?? sharedStore;
	const viewDir = options.viewDir ?? defaultViewDir;
	const server = new McpServer({ name: "pprof-explorer", version: "0.1.0" });

	const lookup = (
		id: string,
	): { snapshot: Snapshot } | { error: CallToolResult } => {
		const snapshot = store.get(id);
		if (snapshot === undefined) {
			const known = store.list().map((s) => s.id);
			return {
				error: errorResult(
					`No profile with id ${id}. ${known.length > 0 ? `Known ids: ${known.join(", ")}.` : "Capture one with capture_profile first."}`,
					{ profile_id: id },
				),
			};
		}
		return { snapshot };
	};

	const listProfiles = (): CallToolResult => {
		const snapshots = store.list();
		return budgeted(snapshots, false, (kept, truncated) =>
			textResult(listText(kept), {
				view: "list",
				target,
				profiles: kept.map(snapshotSummary),
				truncated,
			}),
		);
	};

	const listText = (snapshots: Snapshot[]): string => {
		if (snapshots.length === 0) {
			return `No profiles captured from ${target}. Call capture_profile first.`;
		}
		const lines = snapshots.map(
			(s) => `${describeSnapshot(s)}: ${formatTotals(s)}`,
		);
		return `Profiles from ${target} (${snapshots.length}):\n${lines.join("\n")}`;
	};

	registerAppTool(
		server,
		"capture_profile",
		{
			title: "Capture profile",
			description: [
				`Fetch a Go pprof profile from the fixed target ${target} and store it as a snapshot (profile_id p1, p2, ...).`,
				"This is the first step of every investigation: capture, then inspect the snapshot with top, callers_callees, goroutine_groups, or diff_profiles using its profile_id.",
				"Kinds: " +
					PROFILE_KINDS.map((k) => `${k} = ${kindDescriptions[k]}`).join(
						"; ",
					) +
					".",
				"For leaks capture the same kind twice (before and after) and call diff_profiles. block and mutex profiles are cumulative since process start, so diff two captures to isolate a time window.",
				"The result includes the top 10 functions for the default sample type and, when the target exposes /api/status, a target_status line.",
			].join(" "),
			inputSchema: z.object({
				kind: z.enum(PROFILE_KINDS).describe("Which /debug/pprof endpoint to capture."),
				seconds: z
					.number()
					.int()
					.min(1)
					.max(30)
					.optional()
					.describe(
						"CPU sampling duration for kind=profile (1..30, default 5). Ignored for other kinds.",
					),
				gc: z
					.boolean()
					.optional()
					.describe(
						"For heap and allocs: run a GC before sampling so inuse_* reflects live memory (default true).",
					),
				label: z
					.string()
					.max(80)
					.optional()
					.describe("Free-form note attached to the snapshot, for example 'after heap-growth'."),
			}),
			_meta: uiMeta,
		},
		async ({ kind, seconds, gc, label }) => {
			// The status read follows the profile fetch inside the same closure so
			// a CPU capture queued behind another reports the status at its own
			// completion time.
			const capture = async () => {
				const profile = await fetchProfile(target, kind, { seconds, gc });
				const targetStatus = await fetchTargetStatus(target);
				return { profile, targetStatus };
			};
			let captured: Awaited<ReturnType<typeof capture>>;
			try {
				captured =
					kind === "profile"
						? await serializeCpuCapture(capture)
						: await capture();
			} catch (err) {
				return errorResult(
					`Capture of ${kind} from ${target} failed: ${errorMessage(err)}`,
					{ kind },
				);
			}
			const { targetStatus } = captured;
			const snapshot = store.add({
				kind,
				capturedAt: new Date(),
				label: label ? normalizeString(label) : undefined,
				target,
				profile: captured.profile,
			});
			const report = top(snapshot.profile, { limit: TEXT_ROWS });
			const summary = snapshotSummary(snapshot);
			const lines = [
				`Captured ${describeSnapshot(snapshot)} from ${target}.`,
				`Sample types: ${sampleTypes(snapshot.profile)
					.map((t) => `${t.type}/${t.unit}`)
					.join(", ")}; default ${report.sample_type}.`,
				`Totals: ${formatTotals(snapshot)}.`,
			];
			if (targetStatus !== undefined) {
				lines.push(
					`target_status (untrusted, reported by the target): ${targetStatus}`,
				);
			}
			lines.push(
				`Top ${Math.min(TEXT_ROWS, report.rows.length)} by flat (${report.sample_type}, flat flat% cum cum% function):`,
				topRowsText(report.rows, report.unit),
				`Next: top(profile_id="${snapshot.id}"), callers_callees, ${kind === "goroutine" || kind === "goroutineleak" ? "goroutine_groups, " : ""}or diff_profiles against an earlier ${kind} snapshot.`,
			);
			return budgeted(report.rows, report.truncated, (rows, truncated) =>
				textResult(lines.join("\n"), {
					view: "capture",
					...summary,
					target,
					unit: report.unit,
					total: report.total,
					top: rows,
					truncated,
					...(targetStatus !== undefined ? { target_status: targetStatus } : {}),
				}),
			);
		},
	);

	registerAppTool(
		server,
		"list_profiles",
		{
			title: "List profiles",
			description:
				"List captured snapshots with their profile_id, kind, capture time, label, and totals per sample type. Use the profile_id with top, callers_callees, goroutine_groups, or diff_profiles.",
			inputSchema: z.object({}),
			_meta: uiMeta,
		},
		async () => listProfiles(),
	);

	registerAppTool(
		server,
		"top",
		{
			title: "Top functions",
			description: [
				"Rank functions in a captured snapshot by flat (self) or cum (inclusive) value for one sample type, like `go tool pprof -top`.",
				"Capture a snapshot first and pass its profile_id. Use focus (a regular expression matched against function name or file) to narrow to a package, then callers_callees on a hot function to see who reaches it.",
			].join(" "),
			inputSchema: z.object({
				profile_id: profileIdSchema,
				sample_type: sampleTypeSchema,
				sort: z
					.enum(["flat", "cum"])
					.optional()
					.describe("Order rows by self value (flat, default) or inclusive value (cum)."),
				focus: z
					.string()
					.max(128)
					.optional()
					.describe("Regular expression; only functions whose name or file matches are returned."),
				limit: limitSchema,
			}),
			_meta: uiMeta,
		},
		async ({ profile_id, sample_type, sort, focus, limit }) => {
			const found = lookup(profile_id);
			if ("error" in found) {
				return found.error;
			}
			const { snapshot } = found;
			let report;
			try {
				report = top(snapshot.profile, {
					sampleType: sample_type,
					sort,
					focus,
					limit,
				});
			} catch (err) {
				return errorResult(`top failed for ${profile_id}: ${errorMessage(err)}`, {
					profile_id,
				});
			}
			const header = `${describeSnapshot(snapshot)} ${report.sample_type}, sort ${report.sort}${report.focus !== null ? `, focus /${report.focus}/` : ""}: total ${formatValue(report.total, report.unit)}, ${report.function_count} functions, showing ${Math.min(TEXT_ROWS, report.rows.length)} (flat flat% cum cum% function):`;
			return budgeted(report.rows, report.truncated, (rows, truncated) =>
				textResult(`${header}\n${topRowsText(rows, report.unit)}`, {
					view: "top",
					profile_id,
					kind: snapshot.kind,
					sample_type: report.sample_type,
					unit: report.unit,
					sort: report.sort,
					focus: report.focus,
					total: report.total,
					function_count: report.function_count,
					rows,
					truncated,
				}),
			);
		},
	);

	registerAppTool(
		server,
		"callers_callees",
		{
			title: "Callers and callees",
			description: [
				"Show one function's flat and cum values plus the functions that call it (callers) and that it calls (callees), weighted by the sample value flowing along each edge, like `pprof -peek`.",
				"Use it to find who allocates or spends time through a hot function found with top. The function name must match a top row exactly.",
			].join(" "),
			inputSchema: z.object({
				profile_id: profileIdSchema,
				function: z
					.string()
					.min(1)
					.max(MAX_FUNCTION_NAME_LENGTH)
					.describe("Exact function name as shown by top."),
				sample_type: sampleTypeSchema,
			}),
			_meta: uiMeta,
		},
		async ({ profile_id, function: fn, sample_type }) => {
			const found = lookup(profile_id);
			if ("error" in found) {
				return found.error;
			}
			const { snapshot } = found;
			let report;
			try {
				report = callersCallees(snapshot.profile, fn, sample_type);
			} catch (err) {
				return errorResult(
					`callers_callees failed for ${profile_id}: ${errorMessage(err)}`,
					{ profile_id, function: normalizeString(fn) },
				);
			}
			const f = report.function;
			const text = [
				`${describeSnapshot(snapshot)} ${report.sample_type}, total ${formatValue(report.total, report.unit)}`,
				`${f.name}${f.file ? ` (${f.file}:${f.line})` : ""}: flat ${formatValue(f.flat, report.unit)} ${formatPct(pctOf(f.flat, report.total))}, cum ${formatValue(f.cum, report.unit)} ${formatPct(pctOf(f.cum, report.total))}`,
				`Callers (${report.callers.length}):`,
				edgesText(report.callers, report.unit, report.total),
				`Callees (${report.callees.length}):`,
				edgesText(report.callees, report.unit, report.total),
			].join("\n");
			const edges = [
				...report.callers.map((e) => ({ side: "caller" as const, ...e })),
				...report.callees.map((e) => ({ side: "callee" as const, ...e })),
			];
			return budgeted(edges, report.truncated, (kept, truncated) =>
				textResult(text, {
					view: "callers",
					profile_id,
					kind: snapshot.kind,
					sample_type: report.sample_type,
					unit: report.unit,
					function: f,
					callers: kept.filter((e) => e.side === "caller").map(stripSide),
					callees: kept.filter((e) => e.side === "callee").map(stripSide),
					total: report.total,
					truncated,
				}),
			);
		},
	);

	registerAppTool(
		server,
		"diff_profiles",
		{
			title: "Diff profiles",
			description: [
				"Compare two snapshots of the same kind: for every function report base and current flat/cum and the signed deltas, sorted by absolute flat delta.",
				"This is how to find leaks and growth: capture before (base_id), reproduce, capture after (profile_id), then diff. It is also the way to isolate a time window in block and mutex profiles, which are cumulative since process start.",
			].join(" "),
			inputSchema: z.object({
				base_id: profileIdSchema.describe("Earlier snapshot."),
				profile_id: profileIdSchema.describe("Later snapshot."),
				sample_type: sampleTypeSchema,
				limit: limitSchema,
			}),
			_meta: uiMeta,
		},
		async ({ base_id, profile_id, sample_type, limit }) => {
			const baseFound = lookup(base_id);
			if ("error" in baseFound) {
				return baseFound.error;
			}
			const found = lookup(profile_id);
			if ("error" in found) {
				return found.error;
			}
			const base = baseFound.snapshot;
			const current = found.snapshot;
			if (base.kind !== current.kind) {
				return errorResult(
					`Cannot diff ${base_id} (${base.kind}) against ${profile_id} (${current.kind}): kinds must match.`,
					{ base_id, profile_id },
				);
			}
			let report;
			try {
				report = diff(base.profile, current.profile, sample_type, limit);
			} catch (err) {
				return errorResult(
					`diff_profiles failed for ${base_id} -> ${profile_id}: ${errorMessage(err)}`,
					{ base_id, profile_id },
				);
			}
			const header = `Diff ${base_id} -> ${profile_id} (${current.kind}, ${report.sample_type}): total ${formatValue(report.base_total, report.unit)} -> ${formatValue(report.total, report.unit)} (${signed(report.total - report.base_total, report.unit)}), showing ${Math.min(TEXT_ROWS, report.rows.length)} by |delta flat|:`;
			return budgeted(report.rows, report.truncated, (rows, truncated) =>
				textResult(`${header}\n${diffRowsText(rows, report.unit)}`, {
					view: "diff",
					base_id,
					profile_id,
					kind: current.kind,
					sample_type: report.sample_type,
					unit: report.unit,
					base_total: report.base_total,
					total: report.total,
					rows,
					truncated,
				}),
			);
		},
	);

	registerAppTool(
		server,
		"goroutine_groups",
		{
			title: "Goroutine groups",
			description: [
				"Group the goroutines in a goroutine or goroutineleak snapshot by identical stack and label set, most numerous first, reporting pprof labels per group (such as those set with pprof.Do).",
				"A large group of identical stacks is the signature of a goroutine leak; compare counts across two snapshots to confirm growth.",
			].join(" "),
			inputSchema: z.object({
				profile_id: profileIdSchema,
				limit: z
					.number()
					.int()
					.min(1)
					.max(MAX_LIMIT)
					.optional()
					.describe(`Maximum groups to return (default 25, max ${MAX_LIMIT}).`),
			}),
			_meta: uiMeta,
		},
		async ({ profile_id, limit }) => {
			const found = lookup(profile_id);
			if ("error" in found) {
				return found.error;
			}
			const { snapshot } = found;
			if (snapshot.kind !== "goroutine" && snapshot.kind !== "goroutineleak") {
				return errorResult(
					`${profile_id} is a ${snapshot.kind} profile; goroutine_groups needs a goroutine or goroutineleak snapshot.`,
					{ profile_id, kind: snapshot.kind },
				);
			}
			let report;
			try {
				report = goroutineGroups(snapshot.profile, clampLimit(limit, 25));
			} catch (err) {
				return errorResult(
					`goroutine_groups failed for ${profile_id}: ${errorMessage(err)}`,
					{ profile_id },
				);
			}
			const header = `${describeSnapshot(snapshot)}: ${report.total} goroutines in ${report.group_count} distinct stacks, showing ${Math.min(TEXT_ROWS, report.groups.length)} (count  leaf <- caller ... [labels]):`;
			return budgeted(report.groups, report.truncated, (groups, truncated) =>
				textResult(`${header}\n${groupsText(groups)}`, {
					view: "groups",
					profile_id,
					kind: snapshot.kind,
					sample_type: report.sample_type,
					unit: report.unit,
					total: report.total,
					group_count: report.group_count,
					groups,
					truncated,
				}),
			);
		},
	);

	registerAppTool(
		server,
		"drop_profile",
		{
			title: "Drop profile",
			description: "Remove a snapshot from the store.",
			inputSchema: z.object({ profile_id: profileIdSchema }),
			_meta: appOnlyMeta,
		},
		async ({ profile_id }) => {
			if (!store.drop(profile_id)) {
				return errorResult(`No profile with id ${profile_id}.`, { profile_id });
			}
			const snapshots = store.list();
			return budgeted(snapshots, false, (kept, truncated) =>
				textResult(`Dropped ${profile_id}.\n\n${listText(kept)}`, {
					view: "dropped",
					profile_id,
					target,
					profiles: kept.map(snapshotSummary),
					truncated,
				}),
			);
		},
	);

	registerAppResource(
		server,
		"pprof explorer",
		EXPLORER_URI,
		{
			description: "Interactive pprof snapshot explorer.",
			mimeType: RESOURCE_MIME_TYPE,
			_meta: { ui: { prefersBorder: true } },
		},
		async () => ({
			contents: [
				{
					uri: EXPLORER_URI,
					mimeType: RESOURCE_MIME_TYPE,
					text: await renderExplorer(viewDir),
					_meta: { ui: { prefersBorder: true } },
				},
			],
		}),
	);

	return server;
}

function stripSide<T extends { side: string }>(edge: T): Omit<T, "side"> {
	const { side: _side, ...rest } = edge;
	return rest;
}

/**
 * Reads explorer.html and explorer.js from viewDir and returns the page with
 * the script inlined as a module in place of SCRIPT_PLACEHOLDER. A literal
 * "</script" inside the script source is escaped so it cannot end the tag.
 */
export async function renderExplorer(viewDir: string): Promise<string> {
	const [html, js] = await Promise.all([
		readFile(join(viewDir, "explorer.html"), "utf8"),
		readFile(join(viewDir, "explorer.js"), "utf8"),
	]);
	if (!html.includes(SCRIPT_PLACEHOLDER)) {
		throw new Error(`explorer.html does not contain ${SCRIPT_PLACEHOLDER}`);
	}
	const safeJs = js.replace(/<\/script/gi, "<\\/script");
	return html.replace(
		SCRIPT_PLACEHOLDER,
		() => `<script type="module">\n${safeJs}\n</script>`,
	);
}

function main(): void {
	const port = Number(process.env.PORT ?? 3334);
	const host = process.env.HOST ?? "127.0.0.1";
	const target = parseTarget(process.env.PPROF_TARGET ?? DEFAULT_TARGET);
	const allowedHosts = allowedHostnames(host, process.env.ALLOWED_HOSTS);

	// The handler builds a fresh McpServer per request (stateless Streamable
	// HTTP); snapshots live in the module-level store.
	const mcp = toNodeHandler(createMcpHandler(() => buildServer({ target })));
	// Answers 403 itself when the Host header names a hostname outside the
	// allowlist, so a page on another origin cannot reach the server through a
	// rebound DNS name.
	const validateHost = hostHeaderValidation(allowedHosts);

	const httpServer = createServer((req, res) => {
		if (!validateHost(req, res)) {
			return;
		}
		const path = new URL(req.url ?? "/", "http://localhost").pathname;
		if (path !== "/mcp") {
			res.writeHead(404, { "content-type": "text/plain" });
			res.end("not found");
			return;
		}
		void mcp(req, res);
	});

	httpServer.listen(port, host, () => {
		console.log(
			`pprof-explorer MCP server listening on http://${host}:${port}/mcp, profiling ${target}, accepting Host ${allowedHosts.join(", ")}`,
		);
	});
}

const LOCALHOST_NAMES = ["localhost", "127.0.0.1", "[::1]"];
const WILDCARD_BINDS = new Set(["0.0.0.0", "::", "[::]"]);

/**
 * Builds the Host header allowlist: the loopback names, the bound host when
 * it is a concrete address, and any comma-separated extra names (for a
 * wildcard bind reached by container or LAN address). IPv6 literals are
 * bracketed as the validator expects.
 */
export function allowedHostnames(host: string, extra?: string): string[] {
	const names = new Set(LOCALHOST_NAMES);
	const add = (name: string) => {
		const trimmed = name.trim();
		if (trimmed === "") {
			return;
		}
		names.add(
			trimmed.includes(":") && !trimmed.startsWith("[")
				? `[${trimmed}]`
				: trimmed,
		);
	};
	if (!WILDCARD_BINDS.has(host)) {
		add(host);
	}
	for (const name of (extra ?? "").split(",")) {
		add(name);
	}
	return [...names];
}

// Only start listening when run directly; tests import buildServer.
if (
	process.argv[1] !== undefined &&
	import.meta.url === pathToFileURL(process.argv[1]).href
) {
	main();
}
