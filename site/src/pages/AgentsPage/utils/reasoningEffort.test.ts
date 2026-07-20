import { describe, expect, it } from "vitest";
import { formatReasoningEffort, pickReasoningEffort } from "./reasoningEffort";

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
