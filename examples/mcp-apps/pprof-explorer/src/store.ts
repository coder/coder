import type { Profile } from "pprof-format";

export const PROFILE_KINDS = [
	"heap",
	"allocs",
	"goroutine",
	"goroutineleak",
	"profile",
	"block",
	"mutex",
] as const;

export type ProfileKind = (typeof PROFILE_KINDS)[number];

export interface Snapshot {
	id: string;
	kind: ProfileKind;
	capturedAt: Date;
	label?: string;
	target: string;
	profile: Profile;
}

export const DEFAULT_CAPACITY = 32;

/**
 * In-memory snapshot store with least-recently-used eviction. Ids are
 * assigned sequentially as p1, p2, ... and are never reused within a
 * process. Reads through get() count as use.
 */
export class SnapshotStore {
	private readonly snapshots = new Map<string, Snapshot>();
	private nextId = 1;

	constructor(private readonly capacity = DEFAULT_CAPACITY) {
		if (capacity < 1) {
			throw new Error("snapshot store capacity must be at least 1");
		}
	}

	add(input: Omit<Snapshot, "id">): Snapshot {
		const snapshot: Snapshot = { ...input, id: `p${this.nextId++}` };
		this.snapshots.set(snapshot.id, snapshot);
		while (this.snapshots.size > this.capacity) {
			const oldest = this.snapshots.keys().next().value;
			if (oldest === undefined) {
				break;
			}
			this.snapshots.delete(oldest);
		}
		return snapshot;
	}

	get(id: string): Snapshot | undefined {
		const snapshot = this.snapshots.get(id);
		if (snapshot === undefined) {
			return undefined;
		}
		// Re-insert so Map iteration order reflects recency.
		this.snapshots.delete(id);
		this.snapshots.set(id, snapshot);
		return snapshot;
	}

	/** Returns snapshots in capture order without affecting recency. */
	list(): Snapshot[] {
		return [...this.snapshots.values()].sort(
			(a, b) => idNumber(a.id) - idNumber(b.id),
		);
	}

	drop(id: string): boolean {
		return this.snapshots.delete(id);
	}

	get size(): number {
		return this.snapshots.size;
	}
}

function idNumber(id: string): number {
	return Number(id.slice(1));
}

let cpuCaptureChain: Promise<unknown> = Promise.resolve();

/**
 * Runs fn after every previously queued CPU capture has settled. The Go
 * pprof handler returns 500 when two CPU profiles overlap, so captures are
 * executed one at a time regardless of which request started them.
 */
export function serializeCpuCapture<T>(fn: () => Promise<T>): Promise<T> {
	const run = cpuCaptureChain.then(fn, fn);
	cpuCaptureChain = run.then(
		() => undefined,
		() => undefined,
	);
	return run;
}
