import { describe, expect, it } from "vitest";
import { createDeferred } from "#/testHelpers/deferred";
import { runExclusiveQueueMutation } from "./chatQueueMutations";

describe("runExclusiveQueueMutation", () => {
	it("runs one queue-position mutation per chat at a time", async () => {
		const chatID = "chat-serialize-1";
		const order: string[] = [];
		const first = createDeferred<void>();
		const second = createDeferred<void>();

		const firstRun = runExclusiveQueueMutation(chatID, async () => {
			order.push("first:start");
			await first.promise;
			order.push("first:end");
		});
		const secondRun = runExclusiveQueueMutation(chatID, async () => {
			order.push("second:start");
			await second.promise;
			order.push("second:end");
		});

		// The send captures the queue head inside its critical section, so the
		// second mutation must not start until the first settles.
		expect(order).toEqual(["first:start"]);

		first.resolve();
		await firstRun;
		second.resolve();
		await secondRun;

		expect(order).toEqual([
			"first:start",
			"first:end",
			"second:start",
			"second:end",
		]);
	});

	it("keeps running after a predecessor rejects", async () => {
		const chatID = "chat-serialize-2";
		const failure = new Error("boom");

		const failing = runExclusiveQueueMutation(chatID, () =>
			Promise.reject(failure),
		);
		const following = runExclusiveQueueMutation(chatID, async () => "ok");

		await expect(failing).rejects.toBe(failure);
		await expect(following).resolves.toBe("ok");
	});

	it("does not serialize across chats", async () => {
		const started: string[] = [];
		const blocked = createDeferred<void>();

		const blocking = runExclusiveQueueMutation("chat-a", async () => {
			started.push("a");
			await blocked.promise;
		});
		const other = runExclusiveQueueMutation("chat-b", async () => {
			started.push("b");
		});

		await other;
		expect(started).toEqual(["a", "b"]);

		blocked.resolve();
		await blocking;
	});
});
