import { describe, expect, it } from "vitest";
import type { LiveStatusModel } from "./liveStatusModel";
import { shouldShowGenericThinking } from "./streamingActivity";
import type { MergedTool, RenderBlock } from "./types";

const liveStatus = (phase: LiveStatusModel["phase"]): LiveStatusModel => {
	switch (phase) {
		case "idle":
			return { phase: "idle", hasAccumulatedOutput: false };
		case "starting":
			return { phase: "starting", hasAccumulatedOutput: false };
		case "streaming":
			return {
				phase: "streaming",
				hasAccumulatedOutput: false,
				hasRecentOutput: true,
			};
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

const quietStreaming: LiveStatusModel = {
	phase: "streaming",
	hasAccumulatedOutput: true,
	hasRecentOutput: false,
};

const tool = (status: MergedTool["status"]): MergedTool => ({
	id: status,
	name: "read_file",
	isError: false,
	status,
});

describe("shouldShowGenericThinking", () => {
	const response: RenderBlock = { type: "response", text: "Let me check." };

	type Case = [
		name: string,
		liveStatus: LiveStatusModel,
		blocks: RenderBlock[],
		tools: MergedTool[],
		shows: boolean,
	];
	const cases: Case[] = [
		["shows while starting", liveStatus("starting"), [], [], true],
		[
			"shows while streaming before any block",
			liveStatus("streaming"),
			[],
			[],
			true,
		],
		[
			"hides while response text keeps arriving",
			liveStatus("streaming"),
			[response],
			[],
			false,
		],
		[
			"shows once response text stops arriving",
			quietStreaming,
			[response],
			[],
			true,
		],
		[
			"hides while reasoning is the last block, even when quiet",
			quietStreaming,
			[{ type: "thinking", text: "thinking" }],
			[],
			false,
		],
		[
			"hides while a tool is running, even when response text is quiet",
			quietStreaming,
			[{ type: "tool", id: "running" }, response],
			[tool("running")],
			false,
		],
		[
			"shows when a completed tool follows earlier text",
			liveStatus("streaming"),
			[response, { type: "tool", id: "completed" }],
			[tool("completed")],
			true,
		],
		...(["idle", "retrying", "reconnecting", "failed"] as const).map(
			(phase): Case => [`hides for ${phase}`, liveStatus(phase), [], [], false],
		),
	];
	it.each(cases)("%s", (_name, status, blocks, tools, shows) => {
		expect(
			shouldShowGenericThinking({ liveStatus: status, blocks, tools }),
		).toBe(shows);
	});
});
