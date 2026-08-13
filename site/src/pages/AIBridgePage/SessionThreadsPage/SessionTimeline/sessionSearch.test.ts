import { describe, expect, it } from "vitest";
import type { AIBridgeThread } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockAIBridgeThread,
} from "#/testHelpers/entities";
import {
	matchesNetworkCallSearch,
	matchesThreadSearch,
	matchesThreadToolSearch,
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

describe("matchesNetworkCallSearch", () => {
	it("matches the destination detail case-insensitively", () => {
		const call = MockAIBridgeSessionNetworkCalls[0];
		expect(matchesNetworkCallSearch(call, "api.github.com")).toBe(true);
		expect(matchesNetworkCallSearch(call, "API.GITHUB.COM")).toBe(true);
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
