import type { ComponentType } from "react";
import * as React from "react";

vi.mock("react", async () => {
	const actual = await vi.importActual<typeof import("react")>("react");
	return {
		...actual,
		lazy: vi.fn(
			<P>(initializer: () => Promise<{ default: ComponentType<P> }>) =>
				initializer,
		),
	};
});

import { lazyWithRetry } from "./lazyWithRetry";

const toLazyInitializer = <P>(
	component: React.LazyExoticComponent<ComponentType<P>>,
): (() => Promise<{ default: ComponentType<P> }>) => {
	return component as unknown as () => Promise<{ default: ComponentType<P> }>;
};

afterEach(() => {
	vi.mocked(React.lazy).mockClear();
	vi.useRealTimers();
});

describe("lazyWithRetry", () => {
	it("resolves on first successful import", async () => {
		const module = {
			default: (() => null) as ComponentType,
		};
		const factory = vi.fn().mockResolvedValue(module);

		const lazyComponent = lazyWithRetry(factory);
		const wrappedFactory = toLazyInitializer(lazyComponent);

		await expect(wrappedFactory()).resolves.toBe(module);
		expect(factory).toHaveBeenCalledTimes(1);
		expect(vi.mocked(React.lazy)).toHaveBeenCalledTimes(1);
	});

	it("retries and resolves after transient failure", async () => {
		vi.useFakeTimers();

		const module = {
			default: (() => null) as ComponentType,
		};
		const factory = vi
			.fn<() => Promise<{ default: ComponentType }>>()
			.mockRejectedValueOnce(
				new Error("Failed to fetch dynamically imported module"),
			)
			.mockResolvedValueOnce(module);

		const wrappedFactory = toLazyInitializer(lazyWithRetry(factory));
		const result = wrappedFactory();

		expect(factory).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1000);
		await expect(result).resolves.toBe(module);
		expect(factory).toHaveBeenCalledTimes(2);
	});

	it("rejects after exhausting retries", async () => {
		vi.useFakeTimers();

		const error = new Error("Failed to fetch dynamically imported module");
		const factory = vi
			.fn<() => Promise<{ default: ComponentType }>>()
			.mockRejectedValue(error);

		const wrappedFactory = toLazyInitializer(lazyWithRetry(factory));
		const result = wrappedFactory();
		const rejection = expect(result).rejects.toThrow(error.message);

		expect(factory).toHaveBeenCalledTimes(1);

		await vi.advanceTimersByTimeAsync(999);
		expect(factory).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1);
		expect(factory).toHaveBeenCalledTimes(2);

		await vi.advanceTimersByTimeAsync(1999);
		expect(factory).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(1);
		expect(factory).toHaveBeenCalledTimes(3);

		await vi.advanceTimersByTimeAsync(3999);
		expect(factory).toHaveBeenCalledTimes(3);
		await vi.advanceTimersByTimeAsync(1);
		expect(factory).toHaveBeenCalledTimes(4);

		await rejection;
	});

	it("fails immediately for non-transient errors", async () => {
		vi.useFakeTimers();

		const error = new Error("Cannot find module './missing'");
		const factory = vi
			.fn<() => Promise<{ default: ComponentType }>>()
			.mockRejectedValue(error);

		const wrappedFactory = toLazyInitializer(lazyWithRetry(factory));

		await expect(wrappedFactory()).rejects.toThrow(error.message);
		expect(factory).toHaveBeenCalledTimes(1);
		expect(vi.getTimerCount()).toBe(0);
	});
});
