import { describe, expect, it } from "vitest";
import type { LiveStatusModel } from "./liveStatusModel";
import { shouldShowGenericThinking } from "./streamingActivity";
import type { MergedTool } from "./types";

const liveStatus = (phase: LiveStatusModel["phase"]): LiveStatusModel => {
	switch (phase) {
		case "idle":
			return { phase: "idle", hasAccumulatedOutput: false };
		case "starting":
			return { phase: "starting", hasAccumulatedOutput: false };
		case "streaming":
			return { phase: "streaming", hasAccumulatedOutput: false };
		case "retrying":
			return {
				phase: "retrying",
				hasAccumulatedOutput: false,
				attempt: 1,
				kind: "generic",
				title: "Retrying request",
				message: "Retrying",
			};
		case "reconnecting":
			return {
				phase: "reconnecting",
				hasAccumulatedOutput: false,
				attempt: 1,
				delayMs: 1000,
				retryingAt: "2026-03-10T00:00:01.000Z",
				title: "Reconnecting",
				message: "Reconnecting",
			};
		case "failed":
			return {
				phase: "failed",
				hasAccumulatedOutput: false,
				kind: "generic",
				title: "Failed",
				message: "Failed",
			};
		case "interrupting":
			return { phase: "interrupting", hasAccumulatedOutput: false };
	}
};

const tool = (status: MergedTool["status"]): MergedTool => ({
	id: status,
	name: "read_file",
	isError: false,
	status,
});

describe("shouldShowGenericThinking", () => {
	it("shows for starting", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("starting"),
				blocks: [],
				tools: [],
			}),
		).toBe(true);
	});

	it("shows for streaming with no readable blocks or running tools", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				blocks: [],
				tools: [],
			}),
		).toBe(true);
	});

	it("hides for streaming with a running tool", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				blocks: [{ type: "tool", id: "read-1" }],
				tools: [tool("running")],
			}),
		).toBe(false);
	});

	it("shows after tools complete but before readable output", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				blocks: [{ type: "tool", id: "read-1" }],
				tools: [tool("completed")],
			}),
		).toBe(true);
	});

	it("hides when response text is visible", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				blocks: [{ type: "response", text: "hello" }],
				tools: [],
			}),
		).toBe(false);
	});

	it("hides when reasoning is visible", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				blocks: [{ type: "thinking", text: "thinking" }],
				tools: [],
			}),
		).toBe(false);
	});

	it.each([
		"idle",
		"retrying",
		"reconnecting",
		"failed",
	] as const)("hides for %s", (phase) => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus(phase),
				blocks: [],
				tools: [],
			}),
		).toBe(false);
	});
});
