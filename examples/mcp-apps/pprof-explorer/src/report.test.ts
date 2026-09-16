import assert from "node:assert/strict";
import { test } from "node:test";
import {
	BIG_ALLOC_SPACE,
	MIB,
	bigGoroutineProfile,
	buildProfile,
	goroutineProfile,
	heapProfile,
	heapProfileAfter,
} from "./fixtures.js";
import {
	MAX_GROUP_FRAMES,
	MAX_LIMIT,
	callersCallees,
	clampLimit,
	diff,
	formatPct,
	formatValue,
	goroutineGroups,
	resolveSampleType,
	sampleTypes,
	toNumber,
	top,
	totals,
} from "./report.js";
import {
	MAX_FUNCTION_NAME_LENGTH,
	MAX_STRING_LENGTH,
	normalizeString,
	safeRegex,
	trimPath,
} from "./sanitize.js";

const RETAIN = "lab.(*heapGrowth).retainBlob";
const NEW_RECORD = "lab.newRecord";
const RUN = "lab.(*heapGrowth).run";

test("sampleTypes and resolveSampleType", () => {
	const profile = heapProfile();
	assert.deepEqual(
		sampleTypes(profile).map((t) => `${t.type}/${t.unit}`),
		[
			"alloc_objects/count",
			"alloc_space/bytes",
			"inuse_objects/count",
			"inuse_space/bytes",
		],
	);
	// defaultSampleType is honoured.
	assert.deepEqual(resolveSampleType(profile), {
		index: 3,
		type: "inuse_space",
		unit: "bytes",
	});
	// A requested name is validated against the list.
	assert.equal(resolveSampleType(profile, "alloc_objects").index, 0);
	assert.throws(
		() => resolveSampleType(profile, "nope"),
		/unknown sample_type "nope"; available: alloc_objects/,
	);
	// Without defaultSampleType the last declared type wins.
	const noDefault = buildProfile({
		sampleTypes: [
			["a", "count"],
			["b", "bytes"],
		],
		samples: [],
	});
	assert.equal(resolveSampleType(noDefault).type, "b");
});

test("bigint values are converted once and totals sum every type", () => {
	const profile = heapProfile();
	const helperSample = profile.sample[3];
	assert.equal(typeof helperSample.value[1], "bigint");
	assert.equal(toNumber(helperSample.value[1]), Number(BIG_ALLOC_SPACE));
	assert.equal(toNumber(7), 7);
	// Unsigned encoding of a negative int64 maps back to a negative number.
	assert.equal(toNumber(2n ** 64n - 5n), -5);
	const sums = totals(profile);
	assert.equal(sums.inuse_space, 65 * MIB);
	assert.equal(sums.alloc_space, 64 * MIB + Number(BIG_ALLOC_SPACE));
	assert.equal(sums.inuse_objects, 1211);
});

test("top attributes flat to the innermost inlined frame and cum once per function", () => {
	const report = top(heapProfile());
	assert.equal(report.sample_type, "inuse_space");
	assert.equal(report.unit, "bytes");
	assert.equal(report.sort, "flat");
	assert.equal(report.focus, null);
	assert.equal(report.total, 65 * MIB);
	assert.equal(report.truncated, false);
	const byName = new Map(report.rows.map((r) => [r.name, r]));

	// Inlined callee at Line[0] receives the flat weight, not the location's
	// outer function.
	const rec = byName.get(NEW_RECORD);
	assert.ok(rec);
	assert.equal(rec.flat, 48 * MIB);
	assert.equal(rec.cum, 48 * MIB);
	assert.equal(rec.file, "lab/record.go");
	assert.equal(rec.line, 12);

	const retain = byName.get(RETAIN);
	assert.ok(retain);
	assert.equal(retain.flat, 12 * MIB);
	assert.equal(retain.cum, 60 * MIB);
	assert.equal(retain.file, "lab/heap_growth.go");
	// Line is the one that carried the flat weight, not the inlining site.
	assert.equal(retain.line, 45);

	// Recursion: cum counted once per sample.
	const recurse = byName.get("main.recurse");
	assert.ok(recurse);
	assert.equal(recurse.flat, 4 * MIB);
	assert.equal(recurse.cum, 4 * MIB);

	const main = byName.get("main.main");
	assert.ok(main);
	assert.equal(main.flat, 0);
	assert.equal(main.cum, 65 * MIB);
	assert.equal(main.cum_pct, 100);
	// Never a leaf: the line is the function's start line, not a call site.
	assert.equal(main.line, 8);
	assert.equal(byName.get(RUN)?.line, 25);

	// Ordering and percentages.
	assert.deepEqual(
		report.rows.slice(0, 4).map((r) => r.name),
		[NEW_RECORD, RETAIN, "main.recurse", "main.helper"],
	);
	assert.equal(rec.flat_pct, Math.round((48 / 65) * 10000) / 100);
	assert.equal(retain.cum_pct, Math.round((60 / 65) * 10000) / 100);
});

test("top sorts by cum, honours focus, limit and truncated", () => {
	const profile = heapProfile();
	const byCum = top(profile, { sort: "cum" });
	assert.equal(byCum.sort, "cum");
	assert.deepEqual(byCum.rows.slice(0, 2).map((r) => r.name).sort(), [
		"main.main",
		"runtime.main",
	]);
	// run and retainBlob tie on cum; the higher flat value breaks the tie.
	assert.deepEqual(byCum.rows.slice(2, 4).map((r) => r.name), [RETAIN, RUN]);

	const focused = top(profile, { focus: "heapGrowth" });
	assert.equal(focused.focus, "heapGrowth");
	assert.deepEqual(
		focused.rows.map((r) => r.name).sort(),
		[RETAIN, RUN].sort(),
	);
	assert.equal(focused.function_count, 2);
	// Focus also matches the trimmed file path.
	assert.equal(top(profile, { focus: "record\\.go" }).rows[0].name, NEW_RECORD);
	// Focus rows keep percentages relative to the whole profile.
	assert.equal(focused.total, 65 * MIB);

	const limited = top(profile, { limit: 2 });
	assert.equal(limited.rows.length, 2);
	assert.equal(limited.truncated, true);
	assert.equal(limited.function_count, 7);

	assert.equal(clampLimit(undefined), 50);
	assert.equal(clampLimit(0), 1);
	assert.equal(clampLimit(10_000), MAX_LIMIT);
	assert.equal(top(profile, { limit: 10_000 }).rows.length, 7);

	assert.throws(() => top(profile, { focus: "(a+)+" }), /quantifiers applied to a group/);
	assert.throws(() => top(profile, { focus: "(" }), /Invalid focus pattern/);
	assert.equal(top(profile, { sampleType: "inuse_objects" }).total, 1211);
});

test("callersCallees weights edges by sample value and dedupes per sample", () => {
	const report = callersCallees(heapProfile(), RETAIN);
	assert.equal(report.function.name, RETAIN);
	assert.equal(report.function.flat, 12 * MIB);
	assert.equal(report.function.cum, 60 * MIB);
	assert.deepEqual(report.callers, [{ name: RUN, weight: 60 * MIB }]);
	assert.deepEqual(report.callees, [{ name: NEW_RECORD, weight: 48 * MIB }]);
	assert.equal(report.total, 65 * MIB);

	const main = callersCallees(heapProfile(), "main.main", "inuse_objects");
	assert.equal(main.sample_type, "inuse_objects");
	assert.deepEqual(main.callers, [{ name: "runtime.main", weight: 1211 }]);
	assert.deepEqual(
		main.callees,
		[
			{ name: RUN, weight: 1200 },
			{ name: "main.recurse", weight: 10 },
			{ name: "main.helper", weight: 1 },
		],
	);
	// The recursive frame produces no self edge.
	const recurse = callersCallees(heapProfile(), "main.recurse");
	assert.deepEqual(recurse.callers, [{ name: "main.main", weight: 4 * MIB }]);
	assert.deepEqual(recurse.callees, []);

	assert.throws(
		() => callersCallees(heapProfile(), "does.not.Exist"),
		/function "does.not.Exist" not found/,
	);
});

test("diff reports signed deltas, one-sided functions, and rejects mismatched types", () => {
	const report = diff(heapProfile(), heapProfileAfter());
	assert.equal(report.sample_type, "inuse_space");
	assert.equal(report.base_total, 65 * MIB);
	assert.equal(report.total, 78 * MIB);
	assert.equal(report.truncated, false);
	const byName = new Map(report.rows.map((r) => [r.name, r]));

	// Sorted by |delta_flat|.
	assert.deepEqual(
		report.rows.slice(0, 3).map((r) => r.name),
		[RETAIN, "main.newcomer", "main.helper"],
	);
	const retain = byName.get(RETAIN);
	assert.ok(retain);
	assert.equal(retain.base_flat, 12 * MIB);
	assert.equal(retain.flat, 24 * MIB);
	assert.equal(retain.delta_flat, 12 * MIB);
	assert.equal(retain.base_cum, 60 * MIB);
	assert.equal(retain.cum, 72 * MIB);
	assert.equal(retain.delta_cum, 12 * MIB);

	// Present only in current.
	const newcomer = byName.get("main.newcomer");
	assert.ok(newcomer);
	assert.equal(newcomer.base_flat, 0);
	assert.equal(newcomer.delta_flat, 2 * MIB);
	assert.equal(newcomer.file, "pprof-lab/main.go");
	// Present only in base.
	const helper = byName.get("main.helper");
	assert.ok(helper);
	assert.equal(helper.flat, 0);
	assert.equal(helper.delta_flat, -MIB);
	assert.equal(helper.delta_cum, -MIB);
	// Unchanged functions have zero deltas and sort last.
	assert.equal(byName.get(NEW_RECORD)?.delta_flat, 0);

	assert.equal(diff(heapProfile(), heapProfileAfter(), undefined, 2).truncated, true);
	assert.equal(
		diff(heapProfile(), heapProfileAfter(), "inuse_objects").rows[0].delta_flat,
		200,
	);
	assert.throws(
		() => diff(heapProfile(), goroutineProfile()),
		/sample types differ/,
	);
	assert.throws(() => diff(heapProfile(), heapProfileAfter(), "bogus"), /unknown sample_type/);
});

test("goroutineGroups groups by full stack and label set, caps frames", () => {
	const report = goroutineGroups(goroutineProfile());
	assert.equal(report.sample_type, "goroutine");
	assert.equal(report.total, 504);
	assert.equal(report.group_count, 4);
	assert.equal(report.truncated, false);

	// Same stack, different label sets: two groups with their own counts.
	const [leak, leakBatch, main, deep] = report.groups;
	assert.equal(leak.count, 300);
	assert.equal(leak.top_frame, "runtime.gopark");
	assert.deepEqual(
		leak.frames.map((f) => f.name),
		["runtime.gopark", "runtime.chanrecv", "lab.(*goroutineLeak).parkedWorker"],
	);
	assert.deepEqual(leak.frames[2], {
		name: "lab.(*goroutineLeak).parkedWorker",
		file: "lab/goroutine_leak.go",
		line: 52,
	});
	assert.deepEqual(leak.labels, { scenario: "goroutine-leak" });
	assert.equal(leak.frame_count, 3);
	assert.equal(leakBatch.count, 200);
	assert.deepEqual(leakBatch.frames, leak.frames);
	assert.deepEqual(leakBatch.labels, { batch: "second", scenario: "goroutine-leak" });

	assert.equal(main.count, 3);
	assert.deepEqual(main.labels, {});
	assert.equal(main.frames[2].name, "main.main");

	assert.equal(deep.count, 1);
	assert.equal(deep.frames.length, MAX_GROUP_FRAMES);
	assert.equal(deep.frame_count, 20);
	assert.equal(deep.frames[0].name, "main.deep0");

	const limited = goroutineGroups(goroutineProfile(), 1);
	assert.equal(limited.groups.length, 1);
	assert.equal(limited.truncated, true);

	const big = goroutineGroups(bigGoroutineProfile(), 25);
	assert.equal(big.group_count, 300);
	assert.equal(big.groups.length, 25);
	assert.equal(big.groups[0].count, 300);
});

test("string normalization applies at emit time to every profile-derived string", () => {
	const raw = `main.evil\u0000name\r\nwith${"x".repeat(600)}`;
	// Differs from raw only beyond the display cap; must stay a separate function.
	const rawTwin = `main.evil\u0000name\r\nwith${"x".repeat(600)}y`;
	const profile = buildProfile({
		sampleTypes: [["goroutine", "count"]],
		samples: [
			{
				stack: [[{ fn: raw, file: "/a/b\u0007/c.go", line: 1 }]],
				values: [2],
				labels: { "key\u001b": "val\nue" },
			},
			{
				stack: [[{ fn: rawTwin, file: "/a/b/c.go", line: 1 }]],
				values: [1],
			},
		],
	});
	const rows = top(profile).rows;
	assert.equal(rows.length, 2);
	assert.equal(rows[0].flat, 2);
	assert.equal(rows[1].flat, 1);
	const row = rows[0];
	assert.equal(row.name.length, MAX_FUNCTION_NAME_LENGTH);
	assert.ok(row.name.startsWith("main.evilnamewithxxx"));
	assert.ok(row.name.endsWith("..."));
	assert.equal(row.file, "b/c.go");
	// Exact-name lookup uses the raw name.
	assert.equal(callersCallees(profile, raw).function.flat, 2);
	const group = goroutineGroups(profile).groups[0];
	assert.deepEqual(group.labels, { key: "value" });
	assert.equal(group.top_frame, row.name);
	assert.equal(group.frames[0].file, "b/c.go");
});

test("normalizeString, trimPath and safeRegex", () => {
	assert.equal(normalizeString("a\u0000b\nc\u2028d"), "abcd");
	assert.equal(normalizeString(42), "");
	assert.equal(normalizeString("x".repeat(160)).length, 160);
	const long = normalizeString("y".repeat(161));
	assert.equal(long.length, MAX_STRING_LENGTH);
	assert.ok(long.endsWith("..."));
	assert.equal(normalizeString("z".repeat(20), 10), "zzzzzzz...");
	assert.equal(normalizeString("z".repeat(600), 512).length, 512);

	assert.equal(trimPath("/usr/local/go/src/runtime/malloc.go"), "runtime/malloc.go");
	assert.equal(trimPath("main.go"), "main.go");
	assert.equal(trimPath("/main.go"), "main.go");
	assert.equal(trimPath(""), "");

	// Accepted: plain substrings and simple patterns.
	assert.ok(safeRegex("main\\.").test("main.foo"));
	assert.ok(safeRegex("^lab\\..*Blob$").test("lab.(*heapGrowth).retainBlob"));
	assert.ok(safeRegex("[+*]+").test("a++"));
	assert.ok(safeRegex("a+b*c?").test("aab"));
	assert.ok(safeRegex("a{2,3}b+?").test("aab"));
	assert.ok(safeRegex("(ab|cd)e").test("cde"));
	assert.ok(safeRegex("x{oops").test("x{oops"));
	// Rejected by the bounded backtracking policy.
	assert.throws(() => safeRegex("x".repeat(129)), /longer than 128/);
	assert.throws(() => safeRegex("(a+)+"), /quantifiers applied to a group/);
	assert.throws(() => safeRegex("(ab)+c"), /quantifiers applied to a group/);
	assert.throws(() => safeRegex("(ab)?"), /quantifiers applied to a group/);
	assert.throws(() => safeRegex("(ab){2}"), /quantifiers applied to a group/);
	assert.throws(() => safeRegex("a+b+c+d+"), /more than 3 quantifiers/);
	assert.throws(() => safeRegex(".*a.*b.*c.*"), /more than 3 quantifiers/);
	assert.throws(() => safeRegex("(?=a)b"), /lookaround/);
	assert.throws(() => safeRegex("(?!a)b"), /lookaround/);
	assert.throws(() => safeRegex("(?<=a)b"), /lookaround/);
	assert.throws(() => safeRegex("(?<!a)b"), /lookaround/);
	assert.throws(() => safeRegex("(a)\\1"), /backreferences/);
	assert.throws(() => safeRegex("[unclosed"), /Invalid focus pattern/);
	assert.throws(() => safeRegex("(a+)+"), /plain substring/);
});

test("formatValue scales units for text output", () => {
	assert.equal(formatValue(512, "bytes"), "512 B");
	assert.equal(formatValue(48 * MIB, "bytes"), "48.0 MiB");
	assert.equal(formatValue(1536, "bytes"), "1.5 KiB");
	assert.equal(formatValue(-2 * MIB, "bytes"), "-2.0 MiB");
	assert.equal(formatValue(4_800_000_000, "nanoseconds"), "4.80s");
	assert.equal(formatValue(2_500_000, "nanoseconds"), "2.50ms");
	assert.equal(formatValue(1_500, "nanoseconds"), "1.5us");
	assert.equal(formatValue(42, "nanoseconds"), "42ns");
	assert.equal(formatValue(500, "count"), "500");
	assert.equal(formatValue(3, "widgets"), "3 widgets");
	assert.equal(formatPct(62.12), "62.1%");
});
