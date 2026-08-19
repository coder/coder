import { describe, expect, it } from "vitest";
import type { AIBridgeThread } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockAIBridgeThread,
} from "#/testHelpers/entities";
import {
	classifyThreadSearch,
	countSessionSearchMatches,
	matchesNetworkCallSearch,
	splitMatchSegments,
} from "./sessionSearch";

describe("classifyThreadSearch", () => {
	it.each([
		["summarize", { promptMatch: true, toolCalls: 0 }],
		["PROJECT", { promptMatch: true, toolCalls: 0 }],
		["list_directory", { promptMatch: false, toolCalls: 1 }],
		["path", { promptMatch: false, toolCalls: 1 }],
		["st", { promptMatch: true, toolCalls: 1 }],
		["claude-opus", { promptMatch: false, toolCalls: 0 }],
		["anthropic", { promptMatch: false, toolCalls: 0 }],
	])("classifies %s", (query, expected) => {
		const result = classifyThreadSearch(MockAIBridgeThread, query);
		expect(result.promptMatch).toBe(expected.promptMatch);
		expect(result.toolCallIds.size).toBe(expected.toolCalls);
	});

	it("matches tool names and input case-insensitively", () => {
		const upper: AIBridgeThread = {
			...MockAIBridgeThread,
			agentic_actions: MockAIBridgeThread.agentic_actions.map((a) => ({
				...a,
				tool_calls: a.tool_calls.map((c) => ({
					...c,
					tool: c.tool.toUpperCase(),
					input: c.input.toUpperCase(),
				})),
			})),
		};
		expect(classifyThreadSearch(upper, "list_directory").toolCallIds.size).toBe(
			1,
		);
		expect(classifyThreadSearch(upper, "path").toolCallIds.size).toBe(1);
	});

	it("does not match a thread with no prompt", () => {
		const noPrompt: AIBridgeThread = {
			...MockAIBridgeThread,
			prompt: undefined,
		};
		expect(classifyThreadSearch(noPrompt, "structure")).toEqual({
			promptMatch: false,
			toolCallIds: new Set<string>(),
		});
	});

	it("keeps the thread visible but reports no tool match for an empty query", () => {
		expect(classifyThreadSearch(MockAIBridgeThread, "")).toEqual({
			promptMatch: true,
			toolCallIds: new Set<string>(),
		});
	});
});

describe("countSessionSearchMatches", () => {
	it("counts each matching thread and network call once", () => {
		// "path" appears only in the tool input, so the thread matches once.
		// No network call detail contains it.
		expect(
			countSessionSearchMatches(
				[MockAIBridgeThread],
				MockAIBridgeSessionNetworkCalls,
				"path",
			),
		).toBe(1);
	});

	it("counts a thread once even when the query recurs within it", () => {
		const repeated: AIBridgeThread = {
			...MockAIBridgeThread,
			prompt: "relay relay relay",
		};
		expect(countSessionSearchMatches([repeated], [], "relay")).toBe(1);
	});

	it("counts each matching network call once", () => {
		// "github.com" appears in two network call details.
		expect(
			countSessionSearchMatches(
				[],
				MockAIBridgeSessionNetworkCalls,
				"github.com",
			),
		).toBe(2);
	});

	it("returns zero for an empty query", () => {
		expect(
			countSessionSearchMatches(
				[MockAIBridgeThread],
				MockAIBridgeSessionNetworkCalls,
				"",
			),
		).toBe(0);
	});
});

describe("splitMatchSegments", () => {
	it("marks each occurrence as a match segment", () => {
		expect(splitMatchSegments("a relay b RELAY", "relay")).toEqual([
			{ text: "a ", match: false },
			{ text: "relay", match: true },
			{ text: " b ", match: false },
			{ text: "RELAY", match: true },
		]);
	});

	it("yields one plain segment for an empty query", () => {
		expect(splitMatchSegments("plain text", "")).toEqual([
			{ text: "plain text", match: false },
		]);
	});

	it("yields one plain segment when nothing matches", () => {
		expect(splitMatchSegments("plain text", "nope")).toEqual([
			{ text: "plain text", match: false },
		]);
	});
});

describe("matchesNetworkCallSearch", () => {
	const call = MockAIBridgeSessionNetworkCalls[0];

	it.each([
		["api.github.com", true],
		["API.GITHUB.COM", true],
		["", true],
		["POST", false],
		["allow api.github.com", false],
	])("matches %s => %s", (query, expected) => {
		expect(matchesNetworkCallSearch(call, query)).toBe(expected);
	});

	it("matches an uppercase detail with a lowercase query", () => {
		const upperDetail = {
			...MockAIBridgeSessionNetworkCalls[0],
			detail: MockAIBridgeSessionNetworkCalls[0].detail.toUpperCase(),
		};
		expect(matchesNetworkCallSearch(upperDetail, "api.github.com")).toBe(true);
	});
});
