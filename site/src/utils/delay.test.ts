import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { delay } from "./delay";

describe("delay", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it("resolves after the given time", async () => {
		let resolved = false;
		void delay(1000).then(() => {
			resolved = true;
		});

		await vi.advanceTimersByTimeAsync(999);
		expect(resolved).toBe(false);

		await vi.advanceTimersByTimeAsync(1);
		expect(resolved).toBe(true);
	});

	it("resolves early when the signal aborts", async () => {
		const controller = new AbortController();
		let resolved = false;
		void delay(60_000, controller.signal).then(() => {
			resolved = true;
		});

		controller.abort();
		await vi.advanceTimersByTimeAsync(0);
		expect(resolved).toBe(true);
	});

	it("resolves immediately for an already-aborted signal", async () => {
		let resolved = false;
		void delay(60_000, AbortSignal.abort()).then(() => {
			resolved = true;
		});

		await vi.advanceTimersByTimeAsync(0);
		expect(resolved).toBe(true);
	});
});
