import { describe, expect, it } from "vitest";
import type { LiveStatusModel } from "./liveStatusModel";
import { shouldShowGenericThinking } from "./streamingActivity";
import type { MergedTool, StreamState } from "./types";

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
	}
};

const streamState = (blocks: StreamState["blocks"]): StreamState => ({
	blocks,
	toolCalls: {},
	toolResults: {},
	sources: [],
});

const tool = (id: string, status: MergedTool["status"]): MergedTool => ({
	id,
	name: "read_file",
	isError: false,
	status,
});

describe("shouldShowGenericThinking", () => {
	it("shows for starting", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("starting"),
				streamState: null,
				streamTools: [],
			}),
		).toBe(true);
	});

	it("shows for streaming with no readable blocks or running tools", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				streamState: null,
				streamTools: [],
			}),
		).toBe(true);
	});

	it("hides for streaming with a running tool", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				streamState: streamState([{ type: "tool", id: "read-1" }]),
				streamTools: [tool("read-1", "running")],
			}),
		).toBe(false);
	});

	// The block renders the waiting placeholder, so a shimmer beside it would
	// show two progress indicators for one tool.
	it("hides for streaming with a tool block whose tool has not arrived", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				streamState: streamState([{ type: "tool", id: "unresolved-1" }]),
				streamTools: [],
			}),
		).toBe(false);
	});

	it("shows after tools complete but before readable output", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				streamState: streamState([{ type: "tool", id: "read-1" }]),
				streamTools: [tool("read-1", "completed")],
			}),
		).toBe(true);
	});

	it("hides when response text is visible", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				streamState: streamState([{ type: "response", text: "hello" }]),
				streamTools: [],
			}),
		).toBe(false);
	});

	it("hides when reasoning is visible", () => {
		expect(
			shouldShowGenericThinking({
				liveStatus: liveStatus("streaming"),
				streamState: streamState([{ type: "thinking", text: "thinking" }]),
				streamTools: [],
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
				streamState: null,
				streamTools: [],
			}),
		).toBe(false);
	});
});
