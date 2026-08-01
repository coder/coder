import { afterEach, describe, expect, it, vi } from "vitest";
import {
	registerClientErrorSink,
	reportClientError,
} from "./clientErrorReporting";

describe("reportClientError", () => {
	afterEach(() => {
		// Unregister by installing a throwaway sink so tests stay isolated.
		registerClientErrorSink(() => {});
	});

	it("is a no-op when no sink is registered", () => {
		// Nothing to assert beyond "does not throw"; the module starts
		// without a sink in production until the telemetry chunk loads.
		expect(() => reportClientError(new Error("boom"))).not.toThrow();
	});

	it("forwards the error and context to the registered sink", () => {
		const sink = vi.fn();
		registerClientErrorSink(sink);

		const error = new Error("boom");
		reportClientError(error, { chatId: "chat-1" });

		expect(sink).toHaveBeenCalledWith(error, { chatId: "chat-1" });
	});

	it("truncates oversized context values", () => {
		const sink = vi.fn();
		registerClientErrorSink(sink);

		const oversized = "x".repeat(5000);
		reportClientError(new Error("boom"), { frameSnippet: oversized });

		const [, context] = sink.mock.calls[0];
		expect(context.frameSnippet).toHaveLength(2048 + "...[truncated]".length);
		expect(context.frameSnippet.endsWith("...[truncated]")).toBe(true);
	});

	it("swallows sink failures", () => {
		registerClientErrorSink(() => {
			throw new Error("sink exploded");
		});

		expect(() => reportClientError(new Error("boom"))).not.toThrow();
	});
});
