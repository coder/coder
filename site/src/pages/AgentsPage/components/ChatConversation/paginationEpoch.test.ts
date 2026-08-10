import { describe, expect, it } from "vitest";
import {
	applySupersededWrites,
	createPaginationEpochManager,
	type PaginationCacheWrite,
} from "./paginationEpoch";

type TestWrite = PaginationCacheWrite & { id: string };

const upsert = (id: string): TestWrite => ({ kind: "upsert", id });
const replace = (id: string): TestWrite => ({ kind: "replace", id });

describe("applySupersededWrites", () => {
	it("returns all writes when there is no replacement", () => {
		expect(applySupersededWrites([upsert("a"), upsert("b")])).toEqual([
			upsert("a"),
			upsert("b"),
		]);
	});

	it("keeps an empty buffer empty", () => {
		expect(applySupersededWrites([])).toEqual([]);
	});

	it("drops every write before the last replacement", () => {
		expect(
			applySupersededWrites([
				upsert("a"),
				upsert("b"),
				replace("r1"),
				upsert("c"),
			]),
		).toEqual([replace("r1"), upsert("c")]);
	});

	it("keeps only the last of multiple replacements", () => {
		expect(
			applySupersededWrites([upsert("a"), replace("r1"), replace("r2")]),
		).toEqual([replace("r2")]);
	});

	it("preserves order for writes after the replacement", () => {
		expect(
			applySupersededWrites([
				replace("r1"),
				upsert("a"),
				upsert("b"),
				replace("r2"),
				upsert("c"),
			]),
		).toEqual([replace("r2"), upsert("c")]);
	});
});

describe("createPaginationEpochManager", () => {
	it("buffers writes while open and replays them on close", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		const generation = manager.open("chat-1");
		manager.record("chat-1", upsert("a"));
		manager.record("chat-1", upsert("b"));
		expect(manager.close("chat-1", generation)).toEqual([
			upsert("a"),
			upsert("b"),
		]);
	});

	it("ignores records when no epoch is open", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		manager.record("chat-1", upsert("dropped"));
		const generation = manager.open("chat-1");
		expect(manager.close("chat-1", generation)).toEqual([]);
	});

	it("replays only when the last participant closes", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		const first = manager.open("chat-1");
		const second = manager.open("chat-1");
		expect(second).toBe(first);
		manager.record("chat-1", upsert("a"));
		expect(manager.close("chat-1", first)).toBeUndefined();
		manager.record("chat-1", upsert("b"));
		expect(manager.close("chat-1", second)).toEqual([upsert("a"), upsert("b")]);
	});

	it("applies supersession when replaying", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		const generation = manager.open("chat-1");
		manager.record("chat-1", upsert("a"));
		manager.record("chat-1", replace("r"));
		manager.record("chat-1", upsert("b"));
		expect(manager.close("chat-1", generation)).toEqual([
			replace("r"),
			upsert("b"),
		]);
	});

	it("treats a close with a stale generation as a no-op", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		const generation = manager.open("chat-1");
		expect(manager.close("chat-1", generation + 1)).toBeUndefined();
		expect(manager.close("chat-1", generation)).toEqual([]);
	});

	it("keeps epochs for different chats independent", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		const chat1 = manager.open("chat-1");
		const chat2 = manager.open("chat-2");
		manager.record("chat-1", upsert("one"));
		manager.record("chat-2", upsert("two"));
		expect(manager.close("chat-2", chat2)).toEqual([upsert("two")]);
		expect(manager.close("chat-1", chat1)).toEqual([upsert("one")]);
	});

	it("returns undefined when closing a chat that has no epoch", () => {
		const manager = createPaginationEpochManager<TestWrite>();
		expect(manager.close("chat-1", 1)).toBeUndefined();
	});
});
