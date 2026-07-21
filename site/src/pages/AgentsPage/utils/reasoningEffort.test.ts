import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	formatReasoningEffort,
	getReasoningEffortForModel,
	pickReasoningEffort,
	saveReasoningEffortForModel,
} from "./reasoningEffort";

describe("reasoning effort storage", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("stores the latest effort independently for each model", () => {
		saveReasoningEffortForModel("model-a", "high");
		saveReasoningEffortForModel("model-b", "medium");
		saveReasoningEffortForModel("model-a", "low");

		expect(getReasoningEffortForModel("model-a")).toBe("low");
		expect(getReasoningEffortForModel("model-b")).toBe("medium");
	});

	it("handles unavailable storage", () => {
		vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
			throw new DOMException("Storage unavailable", "SecurityError");
		});
		expect(getReasoningEffortForModel("model-a")).toBeUndefined();

		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("Storage full", "QuotaExceededError");
		});
		expect(() => saveReasoningEffortForModel("model-a", "high")).not.toThrow();
	});
});

describe("formatReasoningEffort", () => {
	it("formats xhigh", () => {
		expect(formatReasoningEffort("xhigh")).toBe("Xhigh");
	});
});

describe("pickReasoningEffort", () => {
	const efforts = ["low", "medium", "high"];

	it("keeps a selectable value", () => {
		expect(pickReasoningEffort("high", efforts, "medium")).toBe("high");
	});

	it("falls back to the default when the value is not exact", () => {
		expect(pickReasoningEffort(" High ", efforts, "medium")).toBe("medium");
	});

	it("falls back to the highest server-provided effort", () => {
		expect(pickReasoningEffort("xhigh", efforts, "max")).toBe("high");
	});

	it("returns undefined without selectable efforts", () => {
		expect(pickReasoningEffort("high", [], "medium")).toBeUndefined();
	});
});
