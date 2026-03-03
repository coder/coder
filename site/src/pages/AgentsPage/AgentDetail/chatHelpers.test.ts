import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { describe, expect, it } from "vitest";
import {
	extractContextUsageFromMessage,
	getLatestContextUsage,
	getParentChatID,
	getWorkspaceAgent,
	resolveModelFromChatConfig,
} from "./chatHelpers";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Minimal ChatMessage factory – only required fields. */
const makeMessage = (
	overrides: Partial<TypesGen.ChatMessage> = {},
): TypesGen.ChatMessage =>
	({
		id: 1,
		chat_id: "chat-1",
		created_at: "2025-01-01T00:00:00Z",
		role: "assistant",
		...overrides,
	}) as TypesGen.ChatMessage;

const makeOption = (
	id: string,
	provider: string,
	model: string,
): ModelSelectorOption => ({
	id,
	provider,
	model,
	displayName: `${provider}/${model}`,
});

// ---------------------------------------------------------------------------
// extractContextUsageFromMessage
// ---------------------------------------------------------------------------

describe("extractContextUsageFromMessage", () => {
	it("returns null when the message has no usage fields", () => {
		expect(extractContextUsageFromMessage(makeMessage())).toBeNull();
	});

	it("returns usage when input_tokens is present", () => {
		const msg = makeMessage({ usage: { input_tokens: 100 } });
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.inputTokens).toBe(100);
		expect(result!.usedTokens).toBe(100);
	});

	it("returns usage when output_tokens is present", () => {
		const msg = makeMessage({ usage: { output_tokens: 50 } });
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.outputTokens).toBe(50);
		expect(result!.usedTokens).toBe(50);
	});

	it("sums all token components into usedTokens", () => {
		const msg = makeMessage({
			usage: {
				input_tokens: 10,
				output_tokens: 20,
				reasoning_tokens: 5,
				cache_creation_tokens: 3,
				cache_read_tokens: 2,
			},
		});
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.usedTokens).toBe(10 + 20 + 5 + 3 + 2);
		expect(result!.inputTokens).toBe(10);
		expect(result!.outputTokens).toBe(20);
		expect(result!.reasoningTokens).toBe(5);
		expect(result!.cacheCreationTokens).toBe(3);
		expect(result!.cacheReadTokens).toBe(2);
	});

	it("includes contextLimitTokens when context_limit is set", () => {
		const msg = makeMessage({ usage: { context_limit: 128000 } });
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.contextLimitTokens).toBe(128000);
	});

	it("returns usage with only contextLimitTokens and no usedTokens", () => {
		const msg = makeMessage({ usage: { context_limit: 4096 } });
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.usedTokens).toBeUndefined();
		expect(result!.contextLimitTokens).toBe(4096);
	});
});

// ---------------------------------------------------------------------------
// getLatestContextUsage
// ---------------------------------------------------------------------------

describe("getLatestContextUsage", () => {
	it("returns null for an empty message list", () => {
		expect(getLatestContextUsage([])).toBeNull();
	});

	it("returns null when no messages have usage data", () => {
		const messages = [makeMessage(), makeMessage({ id: 2 })];
		expect(getLatestContextUsage(messages)).toBeNull();
	});

	it("returns usage from the last message with usage data", () => {
		const messages = [
			makeMessage({ id: 1, usage: { input_tokens: 100 } }),
			makeMessage({ id: 2 }),
			makeMessage({ id: 3, usage: { input_tokens: 300 } }),
		];
		const result = getLatestContextUsage(messages);
		expect(result).not.toBeNull();
		expect(result!.inputTokens).toBe(300);
	});

	it("skips trailing messages without usage and finds the latest one", () => {
		const messages = [
			makeMessage({ id: 1, usage: { input_tokens: 50 } }),
			makeMessage({ id: 2, usage: { input_tokens: 200 } }),
			makeMessage({ id: 3 }),
		];
		const result = getLatestContextUsage(messages);
		expect(result).not.toBeNull();
		expect(result!.inputTokens).toBe(200);
	});
});

// ---------------------------------------------------------------------------
// getParentChatID
// ---------------------------------------------------------------------------

describe("getParentChatID", () => {
	it("returns undefined for undefined chat", () => {
		expect(getParentChatID(undefined)).toBeUndefined();
	});

	it("returns undefined when parent_chat_id is not present", () => {
		const chat = { id: "c1", title: "test" } as TypesGen.Chat;
		expect(getParentChatID(chat)).toBeUndefined();
	});

	it("returns the parent_chat_id when it is a non-empty string", () => {
		const chat = {
			id: "c1",
			title: "test",
			parent_chat_id: "parent-1",
		} as TypesGen.Chat;
		expect(getParentChatID(chat)).toBe("parent-1");
	});

	it("returns undefined when parent_chat_id is an empty string", () => {
		const chat = {
			id: "c1",
			title: "test",
			parent_chat_id: "",
		} as TypesGen.Chat;
		expect(getParentChatID(chat)).toBeUndefined();
	});

	it("returns undefined when parent_chat_id is only whitespace", () => {
		const chat = {
			id: "c1",
			title: "test",
			parent_chat_id: "   ",
		} as TypesGen.Chat;
		expect(getParentChatID(chat)).toBeUndefined();
	});
});

// ---------------------------------------------------------------------------
// resolveModelFromChatConfig
// ---------------------------------------------------------------------------

describe("resolveModelFromChatConfig", () => {
	const options: ModelSelectorOption[] = [
		makeOption("openai:gpt-4", "openai", "gpt-4"),
		makeOption("anthropic:claude-3", "anthropic", "claude-3"),
	];

	it("returns empty string when no model options exist", () => {
		expect(resolveModelFromChatConfig({ model: "gpt-4" }, [])).toBe("");
	});

	it("returns first option when modelConfig is undefined", () => {
		expect(resolveModelFromChatConfig(undefined, options)).toBe("openai:gpt-4");
	});

	it("matches by exact model id", () => {
		const config = { model: "anthropic:claude-3" };
		expect(resolveModelFromChatConfig(config, options)).toBe(
			"anthropic:claude-3",
		);
	});

	it("matches by provider:model combined candidate", () => {
		// The model field alone doesn't match an option id, but
		// provider + model concatenated does.
		const config = { model: "gpt-4", provider: "openai" };
		expect(resolveModelFromChatConfig(config, options)).toBe("openai:gpt-4");
	});

	it("falls back to model field match on option.model property", () => {
		// Neither `model` nor `provider:model` match an option id,
		// so the function falls through to matching option.model.
		const altOptions: ModelSelectorOption[] = [
			makeOption("custom-id-1", "openai", "gpt-4"),
		];
		const config = { model: "gpt-4", provider: "openai" };
		expect(resolveModelFromChatConfig(config, altOptions)).toBe("custom-id-1");
	});

	it("falls back to model field match ignoring provider when provider is absent", () => {
		const altOptions: ModelSelectorOption[] = [
			makeOption("custom-id-1", "openai", "gpt-4"),
		];
		const config = { model: "gpt-4" };
		expect(resolveModelFromChatConfig(config, altOptions)).toBe("custom-id-1");
	});

	it("respects provider when matching on option.model", () => {
		const altOptions: ModelSelectorOption[] = [
			makeOption("id-a", "azure", "gpt-4"),
			makeOption("id-b", "openai", "gpt-4"),
		];
		const config = { model: "gpt-4", provider: "openai" };
		expect(resolveModelFromChatConfig(config, altOptions)).toBe("id-b");
	});

	it("returns first option when no match is found", () => {
		const config = { model: "unknown-model" };
		expect(resolveModelFromChatConfig(config, options)).toBe("openai:gpt-4");
	});

	it("returns first option when modelConfig is an empty object", () => {
		expect(resolveModelFromChatConfig({}, options)).toBe("openai:gpt-4");
	});
});

// ---------------------------------------------------------------------------
// getWorkspaceAgent
// ---------------------------------------------------------------------------

describe("getWorkspaceAgent", () => {
	const makeAgent = (id: string): TypesGen.WorkspaceAgent =>
		({ id, name: `agent-${id}` }) as TypesGen.WorkspaceAgent;

	const makeWorkspace = (
		agents: TypesGen.WorkspaceAgent[],
	): TypesGen.Workspace =>
		({
			latest_build: {
				resources: [{ agents }],
			},
		}) as unknown as TypesGen.Workspace;

	it("returns undefined when workspace is undefined", () => {
		expect(getWorkspaceAgent(undefined, "agent-1")).toBeUndefined();
	});

	it("returns undefined when there are no agents", () => {
		const ws = makeWorkspace([]);
		expect(getWorkspaceAgent(ws, "agent-1")).toBeUndefined();
	});

	it("returns the matching agent by id", () => {
		const ws = makeWorkspace([makeAgent("a1"), makeAgent("a2")]);
		expect(getWorkspaceAgent(ws, "a2")).toEqual(
			expect.objectContaining({ id: "a2" }),
		);
	});

	it("returns the first agent when workspaceAgentId does not match", () => {
		const ws = makeWorkspace([makeAgent("a1"), makeAgent("a2")]);
		expect(getWorkspaceAgent(ws, "no-match")).toEqual(
			expect.objectContaining({ id: "a1" }),
		);
	});

	it("returns the first agent when workspaceAgentId is undefined", () => {
		const ws = makeWorkspace([makeAgent("a1")]);
		expect(getWorkspaceAgent(ws, undefined)).toEqual(
			expect.objectContaining({ id: "a1" }),
		);
	});

	it("collects agents from multiple resources", () => {
		const ws = {
			latest_build: {
				resources: [
					{ agents: [makeAgent("r1-a1")] },
					{ agents: [makeAgent("r2-a1")] },
				],
			},
		} as unknown as TypesGen.Workspace;
		expect(getWorkspaceAgent(ws, "r2-a1")).toEqual(
			expect.objectContaining({ id: "r2-a1" }),
		);
	});

	it("handles resources with no agents array", () => {
		const ws = {
			latest_build: {
				resources: [{ agents: undefined }, { agents: [makeAgent("a1")] }],
			},
		} as unknown as TypesGen.Workspace;
		expect(getWorkspaceAgent(ws, "a1")).toEqual(
			expect.objectContaining({ id: "a1" }),
		);
	});
});
