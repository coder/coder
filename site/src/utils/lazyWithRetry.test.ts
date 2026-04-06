import { lazy } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { lazyWithRetry } from "./lazyWithRetry";

vi.mock("react", async () => {
	const actual = await vi.importActual<typeof import("react")>("react");
	return {
		...actual,
		lazy: vi.fn((factory: () => Promise<unknown>) => ({
			factory,
		})),
	};
});

const TestComponent = () => null;

const getWrappedFactory = (): (() => Promise<unknown>) => {
	const lazyMock = vi.mocked(lazy);
	const latestCall = lazyMock.mock.calls.at(-1);
	if (!latestCall) {
		throw new Error("Expected lazy to be called");
	}
	return latestCall[0];
};

const flushMicrotasks = async (): Promise<void> => {
	await Promise.resolve();
	await Promise.resolve();
};

describe("lazyWithRetry", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("resolves on first successful import", async () => {
		const factory = vi.fn().mockResolvedValue({ default: TestComponent });

		lazyWithRetry(factory);
		const wrappedFactory = getWrappedFactory();

		await expect(wrappedFactory()).resolves.toEqual({ default: TestComponent });
		expect(factory).toHaveBeenCalledTimes(1);
	});

	it("retries and resolves after transient failure", async () => {
		const chunkError = new Error("Failed to fetch dynamically imported module");
		const factory = vi
			.fn()
			.mockRejectedValueOnce(chunkError)
			.mockResolvedValueOnce({ default: TestComponent });

		lazyWithRetry(factory);
		const wrappedFactory = getWrappedFactory();

		const promise = wrappedFactory();
		expect(factory).toHaveBeenCalledTimes(1);

		await flushMicrotasks();
		vi.advanceTimersByTime(1000);
		await flushMicrotasks();

		await expect(promise).resolves.toEqual({ default: TestComponent });
		expect(factory).toHaveBeenCalledTimes(2);
	});

	it("rejects after exhausting retries", async () => {
		const chunkError = new Error("Failed to fetch dynamically imported module");
		const factory = vi.fn().mockRejectedValue(chunkError);

		lazyWithRetry(factory);
		const wrappedFactory = getWrappedFactory();

		const promise = wrappedFactory();
		expect(factory).toHaveBeenCalledTimes(1);

		await flushMicrotasks();
		vi.advanceTimersByTime(1000);
		await flushMicrotasks();
		expect(factory).toHaveBeenCalledTimes(2);

		vi.advanceTimersByTime(2000);
		await flushMicrotasks();
		expect(factory).toHaveBeenCalledTimes(3);

		vi.advanceTimersByTime(4000);
		await flushMicrotasks();

		await expect(promise).rejects.toBe(chunkError);
		expect(factory).toHaveBeenCalledTimes(4);
	});

	it("fails immediately for non-transient errors", async () => {
		const moduleError = new Error("Cannot find module './missing'");
		const factory = vi.fn().mockRejectedValue(moduleError);

		lazyWithRetry(factory);
		const wrappedFactory = getWrappedFactory();

		const promise = wrappedFactory();
		await expect(promise).rejects.toBe(moduleError);

		vi.advanceTimersByTime(7000);
		await flushMicrotasks();
		expect(factory).toHaveBeenCalledTimes(1);
	});
});
