import { describe, expect, it } from "vitest";
import type { AIBridgeThread } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockAIBridgeThread,
} from "#/testHelpers/entities";
import {
	classifyThreadSearch,
	countSessionSearchResults,
	matchesNetworkCallSearch,
	splitMatchSegments,
	windowAroundFirstMatch,
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

describe("countSessionSearchResults", () => {
	it("sums occurrences across prompt, tool, input, and network detail", () => {
		expect(
			countSessionSearchResults(
				[MockAIBridgeThread],
				MockAIBridgeSessionNetworkCalls,
				"path",
			),
		).toBe(1);
	});

	it("counts multiple occurrences in one field", () => {
		const repeated: AIBridgeThread = {
			...MockAIBridgeThread,
			prompt: "relay relay relay",
		};
		expect(countSessionSearchResults([repeated], [], "relay")).toBe(3);
	});

	it("counts occurrences inside longer words", () => {
		const compound: AIBridgeThread = {
			...MockAIBridgeThread,
			prompt: "vercel-relay and vercelrelay",
		};
		expect(countSessionSearchResults([compound], [], "relay")).toBe(2);
	});

	it("counts network call detail occurrences", () => {
		expect(
			countSessionSearchResults(
				[],
				MockAIBridgeSessionNetworkCalls,
				"github.com",
			),
		).toBe(2);
	});

	it("returns zero for an empty query", () => {
		expect(
			countSessionSearchResults(
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

describe("windowAroundFirstMatch", () => {
	it("returns null when the text fits inside the window", () => {
		expect(windowAroundFirstMatch("short relay", "relay", 100)).toBeNull();
	});

	it("returns null when there is no match", () => {
		const text = "a".repeat(100);
		expect(windowAroundFirstMatch(text, "relay", 10)).toBeNull();
	});

	it("clamps to the start for an early match", () => {
		const text = `relay ${"a".repeat(100)}`;
		expect(windowAroundFirstMatch(text, "relay", 20)).toEqual({
			start: 0,
			end: 20,
		});
	});

	it("centers the window on a deep match", () => {
		const text = `${"a".repeat(50)}relay${"b".repeat(50)}`;
		const window = windowAroundFirstMatch(text, "relay", 20);
		expect(window).not.toBeNull();
		// The match must sit inside the window, roughly centered.
		expect(window!.start).toBeLessThanOrEqual(50);
		expect(window!.end).toBeGreaterThanOrEqual(55);
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
