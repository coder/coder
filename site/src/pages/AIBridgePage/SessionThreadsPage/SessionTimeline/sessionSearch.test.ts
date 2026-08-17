import { describe, expect, it } from "vitest";
import type { AIBridgeThread } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockAIBridgeThread,
} from "#/testHelpers/entities";
import {
	classifyThreadSearch,
	countOccurrences,
	countSessionSearchResults,
	matchesNetworkCallSearch,
	matchesThreadSearch,
	matchesThreadToolSearch,
	splitMatchSegments,
	windowAroundFirstMatch,
} from "./sessionSearch";

describe("matchesThreadSearch", () => {
	it("matches prompt text case-insensitively", () => {
		expect(matchesThreadSearch(MockAIBridgeThread, "PROJECT")).toBe(true);
		expect(matchesThreadSearch(MockAIBridgeThread, "summarize")).toBe(true);
	});

	it("matches tool names", () => {
		expect(matchesThreadSearch(MockAIBridgeThread, "list_directory")).toBe(
			true,
		);
	});

	it("matches tool input JSON", () => {
		expect(matchesThreadSearch(MockAIBridgeThread, ". ")).toBe(true);
		expect(matchesThreadSearch(MockAIBridgeThread, "path")).toBe(true);
	});

	it("does not match model, provider, or unrelated text", () => {
		expect(matchesThreadSearch(MockAIBridgeThread, "claude-opus")).toBe(false);
		expect(matchesThreadSearch(MockAIBridgeThread, "anthropic")).toBe(false);
	});

	it("an empty or whitespace query matches everything", () => {
		expect(matchesThreadSearch(MockAIBridgeThread, "")).toBe(true);
		expect(matchesThreadSearch(MockAIBridgeThread, "   ")).toBe(true);
	});

	it("does not match a thread with no prompt when query is specific", () => {
		const noPrompt: AIBridgeThread = {
			...MockAIBridgeThread,
			prompt: undefined,
		};
		expect(matchesThreadSearch(noPrompt, "structure")).toBe(false);
	});
});

describe("classifyThreadSearch", () => {
	it("reports the prompt axis only for a prompt match", () => {
		expect(classifyThreadSearch(MockAIBridgeThread, "summarize")).toEqual({
			promptMatch: true,
			toolMatch: false,
		});
	});

	it("reports the tool axis only for a tool match", () => {
		expect(classifyThreadSearch(MockAIBridgeThread, "list_directory")).toEqual({
			promptMatch: false,
			toolMatch: true,
		});
	});

	it("reports both axes when both match", () => {
		// "st" appears in the prompt ("structure") and the tool name
		// ("list_directory").
		expect(classifyThreadSearch(MockAIBridgeThread, "st")).toEqual({
			promptMatch: true,
			toolMatch: true,
		});
	});

	it("keeps the thread visible but reports no tool match for an empty query", () => {
		expect(classifyThreadSearch(MockAIBridgeThread, "")).toEqual({
			promptMatch: true,
			toolMatch: false,
		});
	});
});

describe("matchesThreadToolSearch", () => {
	it("matches tool names", () => {
		expect(matchesThreadToolSearch(MockAIBridgeThread, "list_directory")).toBe(
			true,
		);
	});

	it("matches tool input JSON", () => {
		expect(matchesThreadToolSearch(MockAIBridgeThread, "path")).toBe(true);
	});

	it("matches tool names case-insensitively", () => {
		const upperTool: AIBridgeThread = {
			...MockAIBridgeThread,
			agentic_actions: MockAIBridgeThread.agentic_actions.map((a) => ({
				...a,
				tool_calls: a.tool_calls.map((c) => ({
					...c,
					tool: c.tool.toUpperCase(),
				})),
			})),
		};
		expect(matchesThreadToolSearch(upperTool, "list_directory")).toBe(true);
	});

	it("matches tool input case-insensitively", () => {
		const upperInput: AIBridgeThread = {
			...MockAIBridgeThread,
			agentic_actions: MockAIBridgeThread.agentic_actions.map((a) => ({
				...a,
				tool_calls: a.tool_calls.map((c) => ({
					...c,
					input: c.input.toUpperCase(),
				})),
			})),
		};
		expect(matchesThreadToolSearch(upperInput, "path")).toBe(true);
	});

	it("does not match prompt text alone", () => {
		expect(matchesThreadToolSearch(MockAIBridgeThread, "summarize")).toBe(
			false,
		);
	});

	it("returns false for an empty query", () => {
		expect(matchesThreadToolSearch(MockAIBridgeThread, "")).toBe(false);
	});
});

describe("countOccurrences", () => {
	it("counts every non-overlapping occurrence case-insensitively", () => {
		expect(countOccurrences("a relay, a RELAY, and a relay", "relay")).toBe(3);
	});

	it("counts occurrences inside longer words", () => {
		expect(countOccurrences("vercel-relay and vercelrelay", "relay")).toBe(2);
	});

	it("returns zero for an empty query or no match", () => {
		expect(countOccurrences("anything", "")).toBe(0);
		expect(countOccurrences("anything", "nope")).toBe(0);
	});
});

describe("countSessionSearchResults", () => {
	it("sums occurrences across prompt, tool, input, and network detail", () => {
		// prompt "Summarize the project structure" has no "path"; the tool
		// input JSON.stringify({ path: "." }) has one, and the network call
		// detail https://api.github.com/repos/coder/coder has none.
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
		expect(windowAroundFirstMatch("a".repeat(100), "relay", 10)).toBeNull();
	});

	it("clamps to the start for an early match", () => {
		expect(
			windowAroundFirstMatch("relay " + "a".repeat(100), "relay", 20),
		).toEqual({ start: 0, end: 20 });
	});

	it("centers the window on a deep match", () => {
		const text = "a".repeat(50) + "relay" + "b".repeat(50);
		const window = windowAroundFirstMatch(text, "relay", 20);
		expect(window).not.toBeNull();
		// The match must sit inside the window, roughly centered.
		expect(window!.start).toBeLessThanOrEqual(50);
		expect(window!.end).toBeGreaterThanOrEqual(55);
	});
});

describe("matchesNetworkCallSearch", () => {
	it("matches the destination detail case-insensitively", () => {
		const call = MockAIBridgeSessionNetworkCalls[0];
		expect(matchesNetworkCallSearch(call, "api.github.com")).toBe(true);
		expect(matchesNetworkCallSearch(call, "API.GITHUB.COM")).toBe(true);
	});

	it("matches an uppercase detail with a lowercase query", () => {
		const upperDetail = {
			...MockAIBridgeSessionNetworkCalls[0],
			detail: MockAIBridgeSessionNetworkCalls[0].detail.toUpperCase(),
		};
		expect(matchesNetworkCallSearch(upperDetail, "api.github.com")).toBe(true);
	});

	it("does not match method, proto, or matched rule", () => {
		const call = MockAIBridgeSessionNetworkCalls[0];
		expect(matchesNetworkCallSearch(call, "POST")).toBe(false);
		expect(matchesNetworkCallSearch(call, "allow api.github.com")).toBe(false);
	});

	it("an empty query matches everything", () => {
		const call = MockAIBridgeSessionNetworkCalls[0];
		expect(matchesNetworkCallSearch(call, "")).toBe(true);
	});
});
