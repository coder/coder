import assert from "node:assert/strict";
import { test } from "node:test";
import {
	EXPLAIN_MAX_CHARS,
	MODEL_CONTEXT_MAX_CHARS,
	buildExplainMessage,
	buildModelContext,
	computeDiffRows,
	compileFocus,
	describeAgentResult,
	formatPct,
	formatSigned,
	formatValue,
	inferToolFromArgs,
	softWrapName,
} from "./explorer.js";

const MiB = 1024 * 1024;

const snapshot = {
	profile_id: "p2",
	kind: "heap",
	captured_at: "2026-09-16T12:03:11.000Z",
	sample_type: "inuse_space",
	unit: "bytes",
	total: 77.3 * MiB,
};
const base = { profile_id: "p1", kind: "heap", captured_at: "2026-09-16T12:01:02.000Z" };
const row = {
	name: "lab.(*heapGrowth).retainBlob",
	file: "lab/heap_growth.go",
	line: 41,
	flat: 48 * MiB,
	flat_pct: 62.1,
	cum: 48 * MiB,
	cum_pct: 62.1,
};

const DATA_HEADER = "Profile data (verbatim from the profiled process):";

function assertOrdered(text, ...needles) {
	let last = -1;
	for (const needle of needles) {
		const idx = text.indexOf(needle);
		assert.ok(idx >= 0, `missing "${needle}" in:\n${text}`);
		assert.ok(idx > last, `"${needle}" out of order in:\n${text}`);
		last = idx;
	}
}

test("formatValue bytes", () => {
	assert.equal(formatValue(0, "bytes"), "0 B");
	assert.equal(formatValue(512, "bytes"), "512 B");
	assert.equal(formatValue(1024, "bytes"), "1.0 KiB");
	assert.equal(formatValue(48 * MiB, "bytes"), "48.0 MiB");
	assert.equal(formatValue(2.5 * 1024 * MiB, "bytes"), "2.50 GiB");
	assert.equal(formatValue(-1024, "bytes"), "-1.0 KiB");
	assert.equal(formatValue(undefined, "bytes"), "0 B");
});

test("formatValue nanoseconds", () => {
	assert.equal(formatValue(42, "nanoseconds"), "42 ns");
	assert.equal(formatValue(1500, "nanoseconds"), "1.5 us");
	assert.equal(formatValue(12_500_000, "nanoseconds"), "12.5 ms");
	assert.equal(formatValue(1_200_000_000, "nanoseconds"), "1.20 s");
});

test("formatValue count and unknown units", () => {
	assert.equal(formatValue(500, "count"), "500");
	assert.equal(formatValue(12345, "count"), "12,345");
	assert.equal(formatValue(-7, "count"), "-7");
	assert.equal(formatValue(3, "widgets"), "3 widgets");
});

test("formatPct and formatSigned", () => {
	assert.equal(formatPct(62.123), "62.1%");
	assert.equal(formatPct(0), "0.0%");
	assert.equal(formatPct(undefined), "0.0%");
	assert.equal(formatSigned(38 * MiB, "bytes"), "+38.0 MiB");
	assert.equal(formatSigned(-2048, "bytes"), "-2.0 KiB");
	assert.equal(formatSigned(0, "count"), "0");
});

test("softWrapName splits after . and / and before (", () => {
	assert.deepEqual(softWrapName("lab.(*heapGrowth).retainBlob"), ["lab.", "(*heapGrowth).", "retainBlob"]);
	assert.deepEqual(softWrapName("github.com/coder/x.Foo"), ["github.", "com/", "coder/", "x.", "Foo"]);
	assert.deepEqual(softWrapName("main"), ["main"]);
	assert.deepEqual(softWrapName("runtime.gopark"), ["runtime.", "gopark"]);
	assert.deepEqual(softWrapName("trailing."), ["trailing."]);
	assert.deepEqual(softWrapName(""), [""]);
	const name = "a/b.(*T).m(x).n";
	assert.equal(softWrapName(name).join(""), name);
});

test("compileFocus rejects bad and hostile patterns", () => {
	assert.equal(compileFocus(""), null);
	assert.equal(compileFocus("("), null);
	assert.equal(compileFocus("(a+)+"), null);
	assert.equal(compileFocus("(ab)*"), null);
	assert.equal(compileFocus("(?:ab){2}"), null);
	assert.equal(compileFocus("a+b+c+d+"), null);
	assert.equal(compileFocus("a{1,3}b*c?d+"), null);
	assert.equal(compileFocus("(?=a)b"), null);
	assert.equal(compileFocus("(?!a)b"), null);
	assert.equal(compileFocus("(?<=a)b"), null);
	assert.equal(compileFocus("(?<!a)b"), null);
	assert.equal(compileFocus("(a)\\1"), null);
	assert.equal(compileFocus("(?<n>a)\\k<n>"), null);
	assert.equal(compileFocus("x".repeat(129)), null);
	assert.ok(compileFocus("main\\.").test("main.run"));
	assert.ok(compileFocus("a+b+c+").test("abc"));
	assert.ok(compileFocus("(?:lab|main)\\.").test("lab.x"));
	assert.ok(compileFocus("[+*?]{2}").test("a++"));
	assert.ok(compileFocus("\\+\\*").test("+*"));
});

test("computeDiffRows filters and sorts by absolute delta", () => {
	const rows = [
		{ name: "main.a", file: "cmd/a.go", delta_flat: 10, delta_cum: -50 },
		{ name: "main.b", file: "cmd/b.go", delta_flat: -30, delta_cum: 5 },
		{ name: "lab.c", file: "lab/c.go", delta_flat: 20, delta_cum: 1 },
	];
	const flat = computeDiffRows(rows, { sort: "flat" });
	assert.deepEqual(flat.rows.map((r) => r.name), ["main.b", "lab.c", "main.a"]);
	assert.equal(flat.maxAbs, 30);
	assert.equal(flat.key, "delta_flat");
	const cum = computeDiffRows(rows, { sort: "cum", focus: "^main\\." });
	assert.deepEqual(cum.rows.map((r) => r.name), ["main.a", "main.b"]);
	assert.equal(cum.maxAbs, 50);
	const byFile = computeDiffRows(rows, { sort: "flat", focus: "^lab/" });
	assert.deepEqual(byFile.rows.map((r) => r.name), ["lab.c"]);
	assert.deepEqual(computeDiffRows(undefined, {}).rows, []);
});

test("buildExplainMessage row", () => {
	const text = buildExplainMessage("row", { snapshot, base, row });
	assert.ok(text.startsWith('Explain why the function "lab.(*heapGrowth).retainBlob" accounts for 62.1% of in-use heap in p2.'));
	assertOrdered(
		text,
		"Explain why",
		DATA_HEADER,
		"snapshot p2, heap, inuse_space, captured 12:03:11Z, total 77.3 MiB",
		"base p1 (heap, 12:01:02Z)",
		"row: lab.(*heapGrowth).retainBlob",
		"flat 48.0 MiB (62.1%), cum 48.0 MiB (62.1%)",
		"lab/heap_growth.go:41",
		'Use the pprof explorer tools on profile_id "p2" (top, callers_callees, diff_profiles against "p1")',
		"holding the memory",
	);
	assert.ok(text.endsWith("what to check next."));
	assert.ok(text.length <= EXPLAIN_MAX_CHARS);
});

test("buildExplainMessage row without base omits diff tool", () => {
	const text = buildExplainMessage("row", { snapshot, row });
	assert.ok(!text.includes("base p1"));
	assert.ok(!text.includes("diff_profiles"));
	assert.ok(text.includes('profile_id "p2" (top, callers_callees)'));
});

test("buildExplainMessage diffRow", () => {
	const diffRow = {
		name: "lab.(*heapGrowth).retainBlob",
		base_flat: 10 * MiB,
		flat: 48 * MiB,
		delta_flat: 38 * MiB,
		base_cum: 12 * MiB,
		cum: 60 * MiB,
		delta_cum: 48 * MiB,
	};
	const text = buildExplainMessage("diffRow", { snapshot, base, row: diffRow });
	assert.ok(text.startsWith('Explain why the function "lab.(*heapGrowth).retainBlob" changed by +38.0 MiB of in-use heap between p1 and p2.'));
	assertOrdered(
		text,
		DATA_HEADER,
		"flat: base 10.0 MiB, current 48.0 MiB, delta +38.0 MiB",
		"cum: base 12.0 MiB, current 60.0 MiB, delta +48.0 MiB",
		'diff_profiles against "p1"',
		"what changed between the two snapshots",
	);
});

test("buildExplainMessage group caps frames at 8 and lists labels", () => {
	const frames = Array.from({ length: 12 }, (_, i) => ({ name: "frame" + i, file: "f.go", line: i + 1 }));
	const group = { count: 500, top_frame: "lab.(*goroutineLeak).parkedWorker", frames, labels: { scenario: "goroutine-leak" } };
	const gsnap = { profile_id: "p3", kind: "goroutine", captured_at: snapshot.captured_at, sample_type: "goroutine", unit: "count", total: 512 };
	const text = buildExplainMessage("group", { snapshot: gsnap, group });
	assert.ok(text.startsWith('Explain what 500 goroutines at the function "lab.(*goroutineLeak).parkedWorker" in p3 are doing and whether they are leaked.'));
	assertOrdered(text, DATA_HEADER, "group: 500 goroutines (97.7% of 512)", "labels: scenario=goroutine-leak", "frame0 f.go:1", "frame7 f.go:8", "(+4 more frames)", "goroutine_groups", "whether this is a leak");
	assert.ok(!text.includes("frame8"));
});

test("buildExplainMessage edge names both ends", () => {
	const fn = { name: "lab.(*heapGrowth).retainBlob", flat: 48 * MiB, cum: 48 * MiB };
	const caller = buildExplainMessage("edge", { snapshot, edge: { direction: "caller", function: fn, name: "lab.(*heapGrowth).run", weight: 48 * MiB } });
	assert.ok(caller.startsWith('Explain why the function "lab.(*heapGrowth).run" calls into "lab.(*heapGrowth).retainBlob" for 48.0 MiB (62.1%) of in-use heap in p2.'));
	assert.ok(caller.includes("edge: lab.(*heapGrowth).run -> lab.(*heapGrowth).retainBlob, weight 48.0 MiB (62.1%)"));
	const callee = buildExplainMessage("edge", { snapshot, edge: { direction: "callee", function: fn, name: "runtime.makeslice", weight: 40 * MiB } });
	assert.ok(callee.startsWith('Explain why the function "lab.(*heapGrowth).retainBlob" calls into "runtime.makeslice" for 40.0 MiB (51.7%)'));
	assert.ok(callee.includes("function: lab.(*heapGrowth).retainBlob, flat 48.0 MiB (62.1%), cum 48.0 MiB (62.1%)"));
});

test("buildExplainMessage profile inlines the top 3 rows", () => {
	const top = Array.from({ length: 5 }, (_, i) => ({ name: "fn" + i, flat: (5 - i) * MiB, flat_pct: (5 - i) * 10, cum: (5 - i) * MiB, cum_pct: (5 - i) * 10 }));
	const text = buildExplainMessage("profile", { snapshot, base, top });
	assert.ok(text.startsWith("Assess the heap profile p2 (inuse_space) and point out anything pathological."));
	assertOrdered(text, DATA_HEADER, "top rows by flat:", "fn0 flat 5.0 MiB (50.0%)", "fn2 flat 3.0 MiB (30.0%)", "main hotspots");
	assert.ok(!text.includes("fn3"));
});

test("buildExplainMessage respects the 1536 cap with a hostile function name", () => {
	const hostile = "x".repeat(5000) + ".(*T).method";
	const text = buildExplainMessage("row", { snapshot, base, row: { ...row, name: hostile, file: "y".repeat(3000) } });
	assert.ok(text.length <= EXPLAIN_MAX_CHARS, `length ${text.length}`);
	assert.ok(text.startsWith('Explain why the function "xxx'));
	assert.ok(text.includes('..." accounts for 62.1%'), "clipped name stays quoted");
	assert.ok(text.includes(DATA_HEADER));
	assert.ok(text.includes('Use the pprof explorer tools on profile_id "p2"'));
	assert.ok(text.includes('diff_profiles against "p1"'));
	assert.ok(text.endsWith("what to check next."));
});

test("buildExplainMessage truncates the data block and keeps the instruction", () => {
	const frames = Array.from({ length: 8 }, (_, i) => ({ name: "f" + i + "." + "z".repeat(190), file: "path/" + "q".repeat(150) + ".go", line: 1 }));
	const group = { count: 3, top_frame: frames[0].name, frames, labels: {} };
	const gsnap = { profile_id: "p3", kind: "goroutine", captured_at: snapshot.captured_at, sample_type: "goroutine", unit: "count", total: 3 };
	const text = buildExplainMessage("group", { snapshot: gsnap, group });
	assert.ok(text.length <= EXPLAIN_MAX_CHARS, `length ${text.length}`);
	assert.ok(text.startsWith('Explain what 3 goroutines at the function "f0.'));
	assert.ok(text.includes("[truncated]"));
	assertOrdered(text, DATA_HEADER, "[truncated]", 'Use the pprof explorer tools on profile_id "p3"');
	assert.ok(text.endsWith("what to check next."));
});

test("buildExplainMessage never exceeds the cap even for garbage input", () => {
	for (const kind of ["row", "diffRow", "group", "edge", "profile", "unknown"]) {
		const text = buildExplainMessage(kind, {});
		assert.ok(text.length <= EXPLAIN_MAX_CHARS);
		assert.ok(text.includes("pprof explorer tools"));
	}
});

test("buildModelContext describes the selection compactly", () => {
	const profiles = [
		{ profile_id: "p1", kind: "heap" },
		{ profile_id: "p2", kind: "heap" },
	];
	const text = buildModelContext({
		profiles,
		selectedId: "p2",
		baseId: "p1",
		mode: "top",
		sampleType: "inuse_space",
		sort: "flat",
		focus: "main.",
		selectedRow: row,
		unit: "bytes",
	});
	assert.equal(
		text,
		'Viewing p2 (heap, inuse_space, sort flat, filter "main."); selected lab.(*heapGrowth).retainBlob flat 48.0 MiB (62.1%); base p1.',
	);
	assert.ok(text.length <= MODEL_CONTEXT_MAX_CHARS);
});

test("buildModelContext diff, groups, and empty variants", () => {
	const profiles = [
		{ profile_id: "p1", kind: "heap" },
		{ profile_id: "p2", kind: "heap" },
		{ profile_id: "p3", kind: "goroutine" },
	];
	const diff = buildModelContext({ profiles, selectedId: "p2", baseId: "p1", mode: "diff", sampleType: "inuse_space", sort: "cum", focus: "", selectedRow: { name: "lab.x", delta_flat: -2048 }, unit: "bytes" });
	assert.equal(diff, "Viewing diff p1 -> p2 (heap, inuse_space, sort cum); selected lab.x delta -2.0 KiB.");
	const groups = buildModelContext({ profiles, selectedId: "p3", baseId: "p1", mode: "groups", groupCount: 4, selectedRow: { name: "lab.parkedWorker", count: 500 } });
	assert.equal(groups, "Viewing p3 goroutine groups (goroutine, 4 groups); selected group lab.parkedWorker x500; base p1.");
	const empty = buildModelContext({ profiles, selectedId: null });
	assert.equal(empty, "pprof explorer: no snapshot selected; 3 snapshots available (p1 heap, p2 heap, p3 goroutine).");
	assert.equal(buildModelContext({ profiles: [], selectedId: null }), "pprof explorer: no snapshot selected; 0 snapshots available.");
});

test("buildModelContext stays under the cap with a hostile row name", () => {
	const text = buildModelContext({
		profiles: [{ profile_id: "p2", kind: "heap" }],
		selectedId: "p2",
		mode: "top",
		sampleType: "inuse_space",
		sort: "flat",
		focus: "f".repeat(500),
		selectedRow: { name: "n".repeat(5000), flat: 1, flat_pct: 1 },
		unit: "bytes",
	});
	assert.ok(text.length <= MODEL_CONTEXT_MAX_CHARS, `length ${text.length}`);
	assert.ok(text.startsWith("Viewing p2 (heap, inuse_space, sort flat, filter \"fff"));
});

test("describeAgentResult", () => {
	assert.equal(describeAgentResult({ view: "callers", profile_id: "p3", function: { name: "lab.retainBlob" } }), "callers of lab.retainBlob in p3");
	assert.equal(describeAgentResult({ view: "top", profile_id: "p2", sample_type: "inuse_space", sort: "cum", focus: "main." }), 'top of p2 (inuse_space, sort cum, filter "main.")');
	assert.equal(describeAgentResult({ view: "diff", profile_id: "p2", base_id: "p1" }), "diff p1 -> p2");
	assert.equal(describeAgentResult({ view: "groups", profile_id: "p3" }), "goroutine groups of p3");
	assert.equal(describeAgentResult(null), "");
});

test("inferToolFromArgs picks the tool to re-issue from the argument shape", () => {
	assert.equal(inferToolFromArgs({ kind: "heap" }), null);
	assert.equal(inferToolFromArgs({ base_id: "p1", profile_id: "p2" }), "diff_profiles");
	assert.equal(inferToolFromArgs({ profile_id: "p2", function: "main.run" }), "callers_callees");
	assert.equal(inferToolFromArgs({ profile_id: "p2", sort: "flat" }), "top");
	assert.equal(inferToolFromArgs({ profile_id: "p3" }, "goroutine"), "goroutine_groups");
	assert.equal(inferToolFromArgs({ profile_id: "p3", limit: 25 }, "goroutine"), "goroutine_groups");
	assert.equal(inferToolFromArgs({ profile_id: "p3", limit: 25 }, "goroutineleak"), "goroutine_groups");
	assert.equal(inferToolFromArgs({ profile_id: "p2", limit: 25 }, "heap"), "top");
	assert.equal(inferToolFromArgs({ profile_id: "p2" }, "heap"), "top");
	assert.equal(inferToolFromArgs({ profile_id: "p9", limit: 25 }, undefined), "top");
	assert.equal(inferToolFromArgs({}), "list_profiles");
});
