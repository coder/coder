import { describe, expect, it } from "vitest";
import type { AIBridgeThread } from "#/api/typesGenerated";
import { MockAIBridgeSessionNetworkCalls } from "#/testHelpers/entities";
import {
	matchesNetworkCallSearch,
	matchesThreadSearch,
	matchesThreadToolQuery,
} from "./sessionSearch";

const mockThread: AIBridgeThread = {
	id: "thread-1",
	prompt: "Summarize the project structure",
	model: "claude-opus-4-6",
	provider: "anthropic",
	credential_kind: "centralized",
	credential_hint: "sk-a...efgh",
	started_at: "2026-03-09T09:28:15.000Z",
	ended_at: "2026-03-09T09:28:47.000Z",
	token_usage: {
		input_tokens: 1240,
		output_tokens: 320,
		cache_read_input_tokens: 900,
		cache_write_input_tokens: 140,
		metadata: {},
	},
	agentic_actions: [
		{
			model: "claude-opus-4-6",
			token_usage: {
				input_tokens: 620,
				output_tokens: 160,
				cache_read_input_tokens: 450,
				cache_write_input_tokens: 70,
				metadata: {},
			},
			thinking: [],
			tool_calls: [
				{
					id: "tool-1",
					interception_id: "interception-1",
					provider_response_id: "resp-1",
					server_url: "http://localhost:3000/mcp",
					tool: "list_directory",
					injected: false,
					input: JSON.stringify({ path: "." }),
					metadata: {},
					created_at: "2026-03-09T09:28:20.000Z",
				},
			],
		},
	],
};

describe("matchesThreadSearch", () => {
	it("matches prompt text case-insensitively", () => {
		expect(matchesThreadSearch(mockThread, "PROJECT")).toBe(true);
		expect(matchesThreadSearch(mockThread, "summarize")).toBe(true);
	});

	it("matches tool names", () => {
		expect(matchesThreadSearch(mockThread, "list_directory")).toBe(true);
	});

	it("matches tool input JSON", () => {
		expect(matchesThreadSearch(mockThread, ". ")).toBe(true);
		expect(matchesThreadSearch(mockThread, "path")).toBe(true);
	});

	it("does not match model, provider, or unrelated text", () => {
		expect(matchesThreadSearch(mockThread, "claude-opus")).toBe(false);
		expect(matchesThreadSearch(mockThread, "anthropic")).toBe(false);
	});

	it("an empty or whitespace query matches everything", () => {
		expect(matchesThreadSearch(mockThread, "")).toBe(true);
		expect(matchesThreadSearch(mockThread, "   ")).toBe(true);
	});

	it("does not match a thread with no prompt when query is specific", () => {
		const noPrompt: AIBridgeThread = { ...mockThread, prompt: undefined };
		expect(matchesThreadSearch(noPrompt, "structure")).toBe(false);
	});
});

describe("matchesThreadToolQuery", () => {
	it("matches tool names", () => {
		expect(matchesThreadToolQuery(mockThread, "list_directory")).toBe(true);
	});

	it("matches tool input JSON", () => {
		expect(matchesThreadToolQuery(mockThread, "path")).toBe(true);
	});

	it("does not match prompt text alone", () => {
		expect(matchesThreadToolQuery(mockThread, "summarize")).toBe(false);
	});

	it("returns false for an empty query", () => {
		expect(matchesThreadToolQuery(mockThread, "")).toBe(false);
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
