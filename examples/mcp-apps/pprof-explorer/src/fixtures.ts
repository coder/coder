// Synthetic pprof profiles for tests, built with pprof-format and passed
// through encode/decode so field types match what the decoder produces.
import {
	Function as PprofFunction,
	Label,
	Line,
	Location,
	Profile,
	Sample,
	StringTable,
	ValueType,
} from "pprof-format";

export interface FrameSpec {
	fn: string;
	file?: string;
	line?: number;
	/** Function.startLine; only the first spec seen for a function applies. */
	startLine?: number;
}

export interface SampleSpec {
	/** Locations leaf first; each location lists its lines innermost first. */
	stack: FrameSpec[][];
	values: Array<number | bigint>;
	labels?: Record<string, string>;
}

export interface ProfileSpec {
	sampleTypes: Array<[type: string, unit: string]>;
	defaultSampleType?: string;
	samples: SampleSpec[];
}

/** Builds a Profile and round-trips it through the wire encoding. */
export function buildProfile(spec: ProfileSpec): Profile {
	const st = new StringTable();
	const functions = new Map<string, PprofFunction>();
	const locations = new Map<string, Location>();

	const functionId = (frame: FrameSpec): number => {
		const key = `${frame.fn}|${frame.file ?? ""}`;
		let fn = functions.get(key);
		if (fn === undefined) {
			fn = new PprofFunction({
				id: functions.size + 1,
				name: st.dedup(frame.fn),
				systemName: st.dedup(frame.fn),
				filename: st.dedup(frame.file ?? ""),
				startLine: frame.startLine ?? 0,
			});
			functions.set(key, fn);
		}
		return Number(fn.id);
	};
	const locationId = (lines: FrameSpec[]): number => {
		const key = lines.map((l) => `${l.fn}|${l.file ?? ""}|${l.line ?? 0}`).join("\n");
		let loc = locations.get(key);
		if (loc === undefined) {
			loc = new Location({
				id: locations.size + 1,
				address: 0x1000 + locations.size,
				line: lines.map(
					(l) => new Line({ functionId: functionId(l), line: l.line ?? 0 }),
				),
			});
			locations.set(key, loc);
		}
		return Number(loc.id);
	};

	const sampleType = spec.sampleTypes.map(
		([type, unit]) =>
			new ValueType({ type: st.dedup(type), unit: st.dedup(unit) }),
	);
	const sample = spec.samples.map(
		(s) =>
			new Sample({
				locationId: s.stack.map(locationId),
				value: s.values,
				label: Object.entries(s.labels ?? {}).map(
					([k, v]) => new Label({ key: st.dedup(k), str: st.dedup(v) }),
				),
			}),
	);
	const profile = new Profile({
		sampleType,
		sample,
		location: [...locations.values()],
		function: [...functions.values()],
		stringTable: st,
		defaultSampleType:
			spec.defaultSampleType !== undefined ? st.dedup(spec.defaultSampleType) : 0,
	});
	return Profile.decode(profile.encode());
}

export const MIB = 1024 * 1024;

const HEAP_TYPES: ProfileSpec["sampleTypes"] = [
	["alloc_objects", "count"],
	["alloc_space", "bytes"],
	["inuse_objects", "count"],
	["inuse_space", "bytes"],
];

const runtimeMain: FrameSpec = {
	fn: "runtime.main",
	file: "/usr/local/go/src/runtime/proc.go",
	line: 250,
};
const mainMain = (line: number): FrameSpec => ({
	fn: "main.main",
	file: "/home/u/src/pprof-lab/main.go",
	line,
	startLine: 8,
});
const run: FrameSpec = {
	fn: "lab.(*heapGrowth).run",
	file: "/home/u/src/pprof-lab/lab/heap_growth.go",
	line: 30,
	startLine: 25,
};
const retainBlob = (line: number): FrameSpec => ({
	fn: "lab.(*heapGrowth).retainBlob",
	file: "/home/u/src/pprof-lab/lab/heap_growth.go",
	line,
	startLine: 38,
});
const newRecord: FrameSpec = {
	fn: "lab.newRecord",
	file: "/home/u/src/pprof-lab/lab/record.go",
	line: 12,
};
const recurse = (line: number): FrameSpec => ({
	fn: "main.recurse",
	file: "/home/u/src/pprof-lab/main.go",
	line,
});
const helper: FrameSpec = {
	fn: "main.helper",
	file: "/home/u/src/pprof-lab/main.go",
	line: 20,
};

/** alloc_space for the helper sample; needs more than four varint bytes. */
export const BIG_ALLOC_SPACE = 2n ** 40n;

/**
 * Heap fixture. inuse_space totals 65 MiB:
 * - 48 MiB in newRecord inlined into retainBlob:41 (one location, two lines)
 * - 12 MiB in retainBlob:45
 * - 4 MiB in a recursive main.recurse stack
 * - 1 MiB in main.helper, with an alloc_space value above 2^32
 */
export function heapProfile(): Profile {
	return buildProfile({
		sampleTypes: HEAP_TYPES,
		defaultSampleType: "inuse_space",
		samples: [
			{
				stack: [[newRecord, retainBlob(41)], [run], [mainMain(10)], [runtimeMain]],
				values: [1000, 48 * MIB, 1000, 48 * MIB],
			},
			{
				stack: [[retainBlob(45)], [run], [mainMain(10)], [runtimeMain]],
				values: [200, 12 * MIB, 200, 12 * MIB],
			},
			{
				stack: [[recurse(5)], [recurse(7)], [mainMain(12)], [runtimeMain]],
				values: [10, 4 * MIB, 10, 4 * MIB],
			},
			{
				stack: [[helper], [mainMain(14)], [runtimeMain]],
				values: [1, BIG_ALLOC_SPACE, 1, MIB],
			},
		],
	});
}

/** Heap fixture after growth: retainBlob:45 doubles and helper disappears. */
export function heapProfileAfter(): Profile {
	return buildProfile({
		sampleTypes: HEAP_TYPES,
		defaultSampleType: "inuse_space",
		samples: [
			{
				stack: [[newRecord, retainBlob(41)], [run], [mainMain(10)], [runtimeMain]],
				values: [1000, 48 * MIB, 1000, 48 * MIB],
			},
			{
				stack: [[retainBlob(45)], [run], [mainMain(10)], [runtimeMain]],
				values: [400, 24 * MIB, 400, 24 * MIB],
			},
			{
				stack: [[recurse(5)], [recurse(7)], [mainMain(12)], [runtimeMain]],
				values: [10, 4 * MIB, 10, 4 * MIB],
			},
			{
				stack: [[{ fn: "main.newcomer", file: "/home/u/src/pprof-lab/main.go", line: 30 }], [mainMain(16)], [runtimeMain]],
				values: [5, 2 * MIB, 5, 2 * MIB],
			},
		],
	});
}

const gopark: FrameSpec = {
	fn: "runtime.gopark",
	file: "/usr/local/go/src/runtime/proc.go",
	line: 424,
};
const chanrecv: FrameSpec = {
	fn: "runtime.chanrecv",
	file: "/usr/local/go/src/runtime/chan.go",
	line: 639,
};
const parkedWorker: FrameSpec = {
	fn: "lab.(*goroutineLeak).parkedWorker",
	file: "/home/u/src/pprof-lab/lab/goroutine_leak.go",
	line: 52,
};

/**
 * Goroutine fixture: 500 parked workers split into two samples with the same
 * stack and different label sets (300 and 200), 3 goroutines in main.main,
 * and one 20-frame stack.
 */
export function goroutineProfile(): Profile {
	const deep: FrameSpec[][] = [];
	for (let i = 0; i < 20; i++) {
		deep.push([{ fn: `main.deep${i}`, file: "/home/u/src/pprof-lab/deep.go", line: i + 1 }]);
	}
	return buildProfile({
		sampleTypes: [["goroutine", "count"]],
		samples: [
			{
				stack: [[gopark], [chanrecv], [parkedWorker]],
				values: [300],
				labels: { scenario: "goroutine-leak" },
			},
			{
				stack: [[gopark], [chanrecv], [parkedWorker]],
				values: [200],
				labels: { scenario: "goroutine-leak", batch: "second" },
			},
			{
				stack: [[gopark], [{ fn: "runtime.selectgo", file: "/usr/local/go/src/runtime/select.go", line: 327 }], [mainMain(40)]],
				values: [3],
			},
			{
				stack: deep,
				values: [1],
			},
		],
	});
}

/** CPU fixture with samples/count and cpu/nanoseconds. */
export function cpuProfile(): Profile {
	return buildProfile({
		sampleTypes: [
			["samples", "count"],
			["cpu", "nanoseconds"],
		],
		samples: [
			{
				stack: [[{ fn: "lab.(*cpuBurn).hotLoop", file: "/home/u/src/pprof-lab/lab/cpu_burn.go", line: 18 }], [mainMain(50)], [runtimeMain]],
				values: [480, 4_800_000_000],
			},
			{
				stack: [[{ fn: "runtime.mallocgc", file: "/usr/local/go/src/runtime/malloc.go", line: 1000 }], [mainMain(52)], [runtimeMain]],
				values: [20, 200_000_000],
			},
		],
	});
}

function longName(prefix: string, i: number): string {
	return `${prefix}${String(i).padStart(4, "0")}.`.padEnd(160, "x");
}

/**
 * Heap fixture sized to overflow the result budget: 400 leaf functions with
 * 160-character names and files, all reached through one hub function that
 * 150 long-named callers reach.
 */
export function bigHeapProfile(): Profile {
	const hub: FrameSpec = { fn: "hub.Dispatch", file: "/x/hub/hub.go", line: 1 };
	const samples: SampleSpec[] = [];
	for (let i = 0; i < 400; i++) {
		const leaf: FrameSpec = {
			fn: longName("leaf", i),
			file: `/very/long/path/${longName("dir", i)}/file.go`,
			line: i,
		};
		const caller: FrameSpec = {
			fn: longName("caller", i % 150),
			file: "/x/callers.go",
			line: i,
		};
		samples.push({
			stack: [[leaf], [hub], [caller], [runtimeMain]],
			values: [1, 4096 * (i + 1), 1, 4096 * (i + 1)],
		});
	}
	return buildProfile({
		sampleTypes: HEAP_TYPES,
		defaultSampleType: "inuse_space",
		samples,
	});
}

/** Goroutine fixture with 300 distinct 20-frame stacks of long names. */
export function bigGoroutineProfile(): Profile {
	const samples: SampleSpec[] = [];
	for (let i = 0; i < 300; i++) {
		const stack: FrameSpec[][] = [];
		for (let d = 0; d < 20; d++) {
			stack.push([
				{
					fn: longName(`g${i}.frame${d}.`, d),
					file: `/very/long/path/${longName("dir", i)}/file.go`,
					line: d,
				},
			]);
		}
		samples.push({
			stack,
			values: [i + 1],
			labels: { scenario: longName("scenario", i) },
		});
	}
	return buildProfile({
		sampleTypes: [["goroutine", "count"]],
		samples,
	});
}
