import type { Profile, Sample } from "pprof-format";
import {
	MAX_FUNCTION_NAME_LENGTH,
	normalizeString,
	safeRegex,
	trimPath,
} from "./sanitize.js";

export const DEFAULT_LIMIT = 50;
export const MAX_LIMIT = 200;
export const MAX_GROUP_FRAMES = 16;
export const MAX_EDGES = 100;

export interface SampleTypeInfo {
	type: string;
	unit: string;
}

export interface ResolvedSampleType extends SampleTypeInfo {
	index: number;
}

export interface FunctionRow {
	name: string;
	file: string;
	line: number;
	flat: number;
	cum: number;
}

export interface TopRow extends FunctionRow {
	flat_pct: number;
	cum_pct: number;
}

export interface TopOptions {
	sampleType?: string;
	sort?: "flat" | "cum";
	focus?: string;
	limit?: number;
}

export interface TopReport {
	sample_type: string;
	unit: string;
	sort: "flat" | "cum";
	focus: string | null;
	total: number;
	function_count: number;
	rows: TopRow[];
	truncated: boolean;
}

export interface Edge {
	name: string;
	weight: number;
}

export interface CallersCalleesReport {
	sample_type: string;
	unit: string;
	function: FunctionRow;
	callers: Edge[];
	callees: Edge[];
	total: number;
	truncated: boolean;
}

export interface DiffRow {
	name: string;
	file: string;
	line: number;
	base_flat: number;
	flat: number;
	delta_flat: number;
	base_cum: number;
	cum: number;
	delta_cum: number;
}

export interface DiffReport {
	sample_type: string;
	unit: string;
	base_total: number;
	total: number;
	rows: DiffRow[];
	truncated: boolean;
}

export interface Frame {
	name: string;
	file: string;
	line: number;
}

export interface GoroutineGroup {
	count: number;
	top_frame: string;
	frames: Frame[];
	frame_count: number;
	labels: Record<string, string>;
}

export interface GoroutineGroupsReport {
	sample_type: string;
	unit: string;
	total: number;
	group_count: number;
	groups: GoroutineGroup[];
	truncated: boolean;
}

const TWO_POW_63 = 1n << 63n;
const TWO_POW_64 = 1n << 64n;

/**
 * Converts a pprof-format numeric field to a JavaScript number. Values are
 * decoded as bigint only when they need more than four varint bytes; a
 * bigint at or above 2^63 is the unsigned encoding of a negative int64.
 */
export function toNumber(value: number | bigint): number {
	if (typeof value === "number") {
		return value;
	}
	return Number(value >= TWO_POW_63 ? value - TWO_POW_64 : value);
}

// Raw string table lookup. Aggregation keys use raw strings so that two
// functions whose names differ only past the display cap stay distinct;
// strings are normalized when rows, edges, and frames are emitted.
function rawStr(profile: Profile, index: number | bigint): string {
	return profile.stringTable.strings[toNumber(index)] ?? "";
}

function str(profile: Profile, index: number | bigint): string {
	return normalizeString(rawStr(profile, index));
}

function displayName(raw: string): string {
	return normalizeString(raw, MAX_FUNCTION_NAME_LENGTH);
}

function displayFrame(frame: Frame): Frame {
	return {
		name: displayName(frame.name),
		file: trimPath(frame.file),
		line: frame.line,
	};
}

/** Lists the profile's sample types in declaration order. */
export function sampleTypes(profile: Profile): SampleTypeInfo[] {
	return profile.sampleType.map((st) => ({
		type: str(profile, st.type),
		unit: str(profile, st.unit),
	}));
}

/**
 * Picks the sample type to aggregate. A requested name must appear in the
 * profile. Otherwise the profile's defaultSampleType is used when it names a
 * known type, else the last declared sample type.
 */
export function resolveSampleType(
	profile: Profile,
	requested?: string,
): ResolvedSampleType {
	const types = sampleTypes(profile);
	if (types.length === 0) {
		throw new Error("profile declares no sample types");
	}
	if (requested !== undefined && requested !== "") {
		const index = types.findIndex((t) => t.type === requested);
		if (index === -1) {
			throw new Error(
				`unknown sample_type "${normalizeString(requested)}"; available: ${types
					.map((t) => t.type)
					.join(", ")}`,
			);
		}
		return { ...types[index], index };
	}
	const defaultName = str(profile, profile.defaultSampleType);
	const defaultIndex =
		defaultName === "" ? -1 : types.findIndex((t) => t.type === defaultName);
	const index = defaultIndex === -1 ? types.length - 1 : defaultIndex;
	return { ...types[index], index };
}

/** Sums every sample type over all samples, keyed by sample type name. */
export function totals(profile: Profile): Record<string, number> {
	const out: Record<string, number> = {};
	const types = sampleTypes(profile);
	for (const t of types) {
		out[t.type] = 0;
	}
	for (const sample of profile.sample) {
		for (let i = 0; i < types.length; i++) {
			out[types[i].type] += toNumber(sample.value[i] ?? 0);
		}
	}
	return out;
}

interface FunctionStats {
	name: string;
	file: string;
	startLine: number;
	flat: number;
	cum: number;
	// Weight of flat attribution per source line, used to pick the line shown.
	leafLines: Map<number, number>;
}

interface Aggregate {
	total: number;
	functions: Map<string, FunctionStats>;
	// callers.get(callee).get(caller) and callees.get(caller).get(callee).
	callers: Map<string, Map<string, number>>;
	callees: Map<string, Map<string, number>>;
}

interface FrameTables {
	locations: Map<number, Frame[]>;
	// Function start line by raw function name.
	startLines: Map<string, number>;
}

// Resolves every location once into its frames in caller-to-callee order:
// Location.line lists the innermost inlined callee first, so the list is
// reversed here. Names and files are raw string table entries.
function buildFrameTables(profile: Profile): FrameTables {
	const functions = new Map<number, Frame>();
	const startLines = new Map<string, number>();
	for (const fn of profile.function) {
		const name = rawStr(profile, fn.name);
		functions.set(toNumber(fn.id), {
			name,
			file: rawStr(profile, fn.filename),
			line: toNumber(fn.startLine),
		});
		if (!startLines.has(name)) {
			startLines.set(name, toNumber(fn.startLine));
		}
	}
	const locations = new Map<number, Frame[]>();
	for (const loc of profile.location) {
		const frames: Frame[] = [];
		for (let i = loc.line.length - 1; i >= 0; i--) {
			const line = loc.line[i];
			const fn = functions.get(toNumber(line.functionId));
			const lineNo = toNumber(line.line);
			frames.push({
				name: fn?.name ?? `0x${toNumber(loc.address).toString(16)}`,
				file: fn?.file ?? "",
				line: lineNo !== 0 ? lineNo : (fn?.line ?? 0),
			});
		}
		if (frames.length === 0) {
			frames.push({
				name: `0x${toNumber(loc.address).toString(16)}`,
				file: "",
				line: 0,
			});
		}
		locations.set(toNumber(loc.id), frames);
	}
	return { locations, startLines };
}

/**
 * Flattens a sample's stack into frames ordered root first, leaf last.
 * Sample.locationId lists the leaf first, so locations are walked from the
 * end; within a location the inlined callee chain is already caller first.
 */
function sampleFrames(sample: Sample, tables: FrameTables): Frame[] {
	const frames: Frame[] = [];
	for (let i = sample.locationId.length - 1; i >= 0; i--) {
		const locFrames = tables.locations.get(toNumber(sample.locationId[i]));
		if (locFrames !== undefined) {
			frames.push(...locFrames);
		}
	}
	return frames;
}

function aggregate(profile: Profile, sampleIndex: number): Aggregate {
	const tables = buildFrameTables(profile);
	const agg: Aggregate = {
		total: 0,
		functions: new Map(),
		callers: new Map(),
		callees: new Map(),
	};
	const statsFor = (frame: Frame): FunctionStats => {
		let stats = agg.functions.get(frame.name);
		if (stats === undefined) {
			stats = {
				name: frame.name,
				file: frame.file,
				startLine: tables.startLines.get(frame.name) ?? 0,
				flat: 0,
				cum: 0,
				leafLines: new Map(),
			};
			agg.functions.set(frame.name, stats);
		}
		return stats;
	};
	const addEdge = (
		map: Map<string, Map<string, number>>,
		from: string,
		to: string,
		weight: number,
	) => {
		let inner = map.get(from);
		if (inner === undefined) {
			inner = new Map();
			map.set(from, inner);
		}
		inner.set(to, (inner.get(to) ?? 0) + weight);
	};

	for (const sample of profile.sample) {
		const value = toNumber(sample.value[sampleIndex] ?? 0);
		if (value === 0) {
			continue;
		}
		agg.total += value;
		const frames = sampleFrames(sample, tables);
		if (frames.length === 0) {
			continue;
		}
		const seenFunctions = new Set<string>();
		const seenEdges = new Set<string>();
		let parent: Frame | undefined;
		for (const frame of frames) {
			const stats = statsFor(frame);
			if (!seenFunctions.has(frame.name)) {
				seenFunctions.add(frame.name);
				stats.cum += value;
			}
			if (parent !== undefined && parent.name !== frame.name) {
				const key = `${parent.name}\u0000${frame.name}`;
				if (!seenEdges.has(key)) {
					seenEdges.add(key);
					addEdge(agg.callees, parent.name, frame.name, value);
					addEdge(agg.callers, frame.name, parent.name, value);
				}
			}
			parent = frame;
		}
		const leaf = frames[frames.length - 1];
		const leafStats = statsFor(leaf);
		leafStats.flat += value;
		leafStats.leafLines.set(
			leaf.line,
			(leafStats.leafLines.get(leaf.line) ?? 0) + value,
		);
	}
	return agg;
}

// Emits a display row: the line is the source line that received the most
// flat weight, or the function's start line when it never appears as a leaf.
function functionRow(stats: FunctionStats): FunctionRow {
	let line = stats.startLine;
	let best = -1;
	for (const [candidate, weight] of stats.leafLines) {
		if (weight > best) {
			best = weight;
			line = candidate;
		}
	}
	return {
		name: displayName(stats.name),
		file: trimPath(stats.file),
		line,
		flat: stats.flat,
		cum: stats.cum,
	};
}

function pct(value: number, total: number): number {
	if (total === 0) {
		return 0;
	}
	return Math.round((value / total) * 10000) / 100;
}

/** Clamps a requested row limit into [1, MAX_LIMIT], defaulting to DEFAULT_LIMIT. */
export function clampLimit(
	limit: number | undefined,
	fallback = DEFAULT_LIMIT,
): number {
	if (limit === undefined || !Number.isFinite(limit)) {
		return fallback;
	}
	return Math.min(MAX_LIMIT, Math.max(1, Math.floor(limit)));
}

export function top(profile: Profile, options: TopOptions = {}): TopReport {
	const st = resolveSampleType(profile, options.sampleType);
	const sort = options.sort ?? "flat";
	const limit = clampLimit(options.limit);
	const focus =
		options.focus !== undefined && options.focus !== ""
			? safeRegex(options.focus)
			: undefined;
	const agg = aggregate(profile, st.index);
	let rows = [...agg.functions.values()].map(functionRow);
	if (focus !== undefined) {
		rows = rows.filter((r) => focus.test(r.name) || focus.test(r.file));
	}
	rows.sort(
		sort === "cum"
			? (a, b) => b.cum - a.cum || b.flat - a.flat || cmp(a.name, b.name)
			: (a, b) => b.flat - a.flat || b.cum - a.cum || cmp(a.name, b.name),
	);
	const truncated = rows.length > limit;
	return {
		sample_type: st.type,
		unit: st.unit,
		sort,
		focus: focus === undefined ? null : focus.source,
		total: agg.total,
		function_count: rows.length,
		rows: rows.slice(0, limit).map((r) => ({
			...r,
			flat_pct: pct(r.flat, agg.total),
			cum_pct: pct(r.cum, agg.total),
		})),
		truncated,
	};
}

function cmp(a: string, b: string): number {
	return a < b ? -1 : a > b ? 1 : 0;
}

function edgeList(map: Map<string, number> | undefined): Edge[] {
	if (map === undefined) {
		return [];
	}
	return [...map.entries()]
		.map(([name, weight]) => ({ name: displayName(name), weight }))
		.sort((a, b) => b.weight - a.weight || cmp(a.name, b.name));
}

/**
 * Reports one function (matched by exact raw name) with its callers and
 * callees weighted by the sample value flowing along each edge. Throws when
 * the function is not present in the profile.
 */
export function callersCallees(
	profile: Profile,
	fn: string,
	sampleType?: string,
): CallersCalleesReport {
	const st = resolveSampleType(profile, sampleType);
	const agg = aggregate(profile, st.index);
	const stats = agg.functions.get(fn);
	if (stats === undefined) {
		throw new Error(
			`function "${normalizeString(fn)}" not found in profile (names must match exactly; use top with a focus pattern to find it)`,
		);
	}
	const callers = edgeList(agg.callers.get(fn));
	const callees = edgeList(agg.callees.get(fn));
	return {
		sample_type: st.type,
		unit: st.unit,
		function: functionRow(stats),
		callers: callers.slice(0, MAX_EDGES),
		callees: callees.slice(0, MAX_EDGES),
		total: agg.total,
		truncated: callers.length > MAX_EDGES || callees.length > MAX_EDGES,
	};
}

/**
 * Compares two profiles with identical sample type lists. Functions present
 * on only one side get zero on the other, so the delta is the signed value.
 * Rows are ordered by absolute flat delta, then absolute cum delta.
 */
export function diff(
	base: Profile,
	current: Profile,
	sampleType?: string,
	limit?: number,
): DiffReport {
	const baseTypes = sampleTypes(base);
	const currentTypes = sampleTypes(current);
	const same =
		baseTypes.length === currentTypes.length &&
		baseTypes.every(
			(t, i) =>
				t.type === currentTypes[i].type && t.unit === currentTypes[i].unit,
		);
	if (!same) {
		throw new Error(
			`sample types differ: base has [${describeTypes(baseTypes)}], current has [${describeTypes(currentTypes)}]`,
		);
	}
	const st = resolveSampleType(current, sampleType);
	const max = clampLimit(limit);
	const baseAgg = aggregate(base, st.index);
	const currentAgg = aggregate(current, st.index);
	const names = new Set([
		...baseAgg.functions.keys(),
		...currentAgg.functions.keys(),
	]);
	const rows: DiffRow[] = [];
	for (const name of names) {
		const b = baseAgg.functions.get(name);
		const c = currentAgg.functions.get(name);
		const row = functionRow((c ?? b) as FunctionStats);
		rows.push({
			name: row.name,
			file: row.file,
			line: row.line,
			base_flat: b?.flat ?? 0,
			flat: c?.flat ?? 0,
			delta_flat: (c?.flat ?? 0) - (b?.flat ?? 0),
			base_cum: b?.cum ?? 0,
			cum: c?.cum ?? 0,
			delta_cum: (c?.cum ?? 0) - (b?.cum ?? 0),
		});
	}
	rows.sort(
		(a, b) =>
			Math.abs(b.delta_flat) - Math.abs(a.delta_flat) ||
			Math.abs(b.delta_cum) - Math.abs(a.delta_cum) ||
			cmp(a.name, b.name),
	);
	return {
		sample_type: st.type,
		unit: st.unit,
		base_total: baseAgg.total,
		total: currentAgg.total,
		rows: rows.slice(0, max),
		truncated: rows.length > max,
	};
}

function describeTypes(types: SampleTypeInfo[]): string {
	return types.map((t) => `${t.type}/${t.unit}`).join(", ");
}

/**
 * Groups samples by their full frame sequence plus their label set (the same
 * identity pprof uses for tag focus), so goroutines with identical stacks but
 * different labels are reported as separate groups. Frames are listed leaf
 * first and capped at MAX_GROUP_FRAMES; frame_count is the uncapped depth.
 */
export function goroutineGroups(
	profile: Profile,
	limit?: number,
): GoroutineGroupsReport {
	const st = resolveSampleType(profile);
	const max = clampLimit(limit, 25);
	const tables = buildFrameTables(profile);
	interface Group {
		count: number;
		frames: Frame[];
		labels: Array<[string, string]>;
	}
	const groups = new Map<string, Group>();
	let total = 0;
	for (const sample of profile.sample) {
		const count = toNumber(sample.value[st.index] ?? 0);
		if (count === 0) {
			continue;
		}
		total += count;
		const frames = sampleFrames(sample, tables).reverse();
		const labels = sampleLabels(profile, sample);
		const key = [
			...frames.map((f) => `${f.name}|${f.file}|${f.line}`),
			"labels",
			...labels.map(([k, v]) => `${k}=${v}`),
		].join("\n");
		let group = groups.get(key);
		if (group === undefined) {
			group = { count: 0, frames, labels };
			groups.set(key, group);
		}
		group.count += count;
	}
	const sorted = [...groups.values()].sort(
		(a, b) =>
			b.count - a.count ||
			cmp(a.frames[0]?.name ?? "", b.frames[0]?.name ?? ""),
	);
	return {
		sample_type: st.type,
		unit: st.unit,
		total,
		group_count: sorted.length,
		groups: sorted.slice(0, max).map((g) => ({
			count: g.count,
			top_frame: displayName(g.frames[0]?.name ?? ""),
			frames: g.frames.slice(0, MAX_GROUP_FRAMES).map(displayFrame),
			frame_count: g.frames.length,
			labels: Object.fromEntries(
				g.labels.map(([k, v]) => [normalizeString(k), normalizeString(v)]),
			),
		})),
		truncated: sorted.length > max,
	};
}

// Returns a sample's labels as raw key/value pairs sorted by key then value.
// Numeric labels render as "<num> <unit>".
function sampleLabels(profile: Profile, sample: Sample): Array<[string, string]> {
	const pairs: Array<[string, string]> = [];
	for (const label of sample.label) {
		const k = rawStr(profile, label.key);
		if (k === "") {
			continue;
		}
		const v =
			toNumber(label.str) !== 0
				? rawStr(profile, label.str)
				: `${toNumber(label.num)}${labelUnit(profile, label.numUnit)}`;
		pairs.push([k, v]);
	}
	return pairs.sort((a, b) => cmp(a[0], b[0]) || cmp(a[1], b[1]));
}

function labelUnit(profile: Profile, unit: number | bigint): string {
	const name = rawStr(profile, unit);
	return name === "" ? "" : ` ${name}`;
}

/**
 * Formats a sample value for text output. Bytes scale to KiB/MiB/GiB,
 * nanoseconds to us/ms/s, counts print as integers, and other units are
 * appended verbatim.
 */
export function formatValue(value: number, unit: string): string {
	const sign = value < 0 ? "-" : "";
	const abs = Math.abs(value);
	switch (unit) {
		case "bytes": {
			const steps = ["B", "KiB", "MiB", "GiB", "TiB"];
			let v = abs;
			let i = 0;
			while (v >= 1024 && i < steps.length - 1) {
				v /= 1024;
				i++;
			}
			return `${sign}${i === 0 ? v.toFixed(0) : v.toFixed(1)} ${steps[i]}`;
		}
		case "nanoseconds": {
			if (abs >= 1e9) {
				return `${sign}${(abs / 1e9).toFixed(2)}s`;
			}
			if (abs >= 1e6) {
				return `${sign}${(abs / 1e6).toFixed(2)}ms`;
			}
			if (abs >= 1e3) {
				return `${sign}${(abs / 1e3).toFixed(1)}us`;
			}
			return `${sign}${abs.toFixed(0)}ns`;
		}
		case "count":
		case "":
			return `${sign}${abs.toFixed(0)}`;
		default:
			return `${sign}${abs} ${unit}`;
	}
}

/** Formats a percentage with one decimal, as pprof's top output does. */
export function formatPct(value: number): string {
	return `${value.toFixed(1)}%`;
}
