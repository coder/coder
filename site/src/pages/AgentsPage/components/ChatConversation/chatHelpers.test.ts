import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatCompactionMessage,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import type { ModelSelectorOption } from "../ChatElements";
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

const buildOption = (
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
		expect(extractContextUsageFromMessage(MockChatMessage)).toBeNull();
	});

	it("returns usage when input_tokens is present", () => {
		const msg = { ...MockChatMessage, usage: { input_tokens: 100 } };
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.inputTokens).toBe(100);
		expect(result!.usedTokens).toBe(100);
	});

	it("returns usage when output_tokens is present", () => {
		const msg = { ...MockChatMessage, usage: { output_tokens: 50 } };
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.outputTokens).toBe(50);
		expect(result!.usedTokens).toBe(50);
	});

	it("sums all token components into usedTokens", () => {
		const msg = {
			...MockChatMessage,
			usage: {
				input_tokens: 10,
				output_tokens: 20,
				reasoning_tokens: 5,
				cache_creation_tokens: 3,
				cache_read_tokens: 2,
			},
		};
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
		const msg = { ...MockChatMessage, usage: { context_limit: 128000 } };
		const result = extractContextUsageFromMessage(msg);
		expect(result).not.toBeNull();
		expect(result!.contextLimitTokens).toBe(128000);
	});

	it("returns usage with only contextLimitTokens and no usedTokens", () => {
		const msg = { ...MockChatMessage, usage: { context_limit: 4096 } };
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
	const compactionSummaryMessage: TypesGen.ChatMessage = {
		...MockChatMessage,
		id: 2,
		role: "tool",
		content: [{ type: "tool-result", tool_name: "chat_summarized" }],
	};

	it("uses the compacted estimate after reloading persisted messages", () => {
		const messages: TypesGen.ChatMessage[] = JSON.parse(
			JSON.stringify([
				{ ...MockChatMessage, id: 1, usage: { input_tokens: 90000 } },
				MockChatCompactionMessage,
			]),
		);
		expect(getLatestContextUsage(messages)).toEqual({
			usedTokens: 12000,
			contextLimitTokens: 100000,
			estimated: true,
		});
		expect(getLatestContextUsage(messages, 200000)).toEqual({
			usedTokens: 12000,
			contextLimitTokens: 200000,
			estimated: true,
		});
	});

	it("replaces the estimate with newer measured usage", () => {
		const result = getLatestContextUsage(
			[
				MockChatCompactionMessage,
				{
					...MockChatMessage,
					id: 4,
					usage: { input_tokens: 15000, context_limit: 200000 },
				},
			],
			100000,
		);
		expect(result?.usedTokens).toBe(15000);
		expect(result?.contextLimitTokens).toBe(200000);
		expect(result?.estimated).toBeUndefined();
	});

	it.each([
		undefined,
		null,
		[],
		{},
		{ estimated_context_tokens: 0, context_limit_tokens: 100000 },
		{ estimated_context_tokens: -1, context_limit_tokens: 100000 },
		{ estimated_context_tokens: 1.5, context_limit_tokens: 100000 },
		{ estimated_context_tokens: "12000", context_limit_tokens: 100000 },
		{ estimated_context_tokens: 12000, context_limit_tokens: 0 },
		{ estimated_context_tokens: 12000 },
	])(
		"does not reuse pre-compaction usage for invalid metadata %j",
		(result) => {
			const message: TypesGen.ChatMessage = JSON.parse(
				JSON.stringify({
					...MockChatCompactionMessage,
					content: [
						{ type: "tool-result", tool_name: "chat_summarized", result },
					],
				}),
			);
			expect(
				getLatestContextUsage([
					{ ...MockChatMessage, usage: { input_tokens: 90000 } },
					message,
				]),
			).toBeNull();
		},
	);

	it.each<TypesGen.ChatMessagePart>([
		{ type: "tool-call", tool_name: "chat_summarized" },
		{
			type: "tool-result",
			tool_name: "chat_summarized",
			is_error: true,
			result: MockChatCompactionMessage.content?.find(
				(part) => part.type === "tool-result",
			)?.result,
		},
		{ type: "tool-call", tool_name: "chat_cleared" },
	])("stops at a boundary without successful summary metadata: %j", (part) => {
		expect(
			getLatestContextUsage([
				MockChatCompactionMessage,
				{ ...MockChatMessage, content: [part] },
			]),
		).toBeNull();
	});

	it("has no usage for an empty chat", () => {
		expect(getLatestContextUsage([], 200000)).toBeNull();
	});

	it("returns usage from the newest usage-bearing message", () => {
		const messages = [
			{ ...MockChatMessage, id: 1, usage: { input_tokens: 100 } },
			{ ...MockChatMessage, id: 2 },
			{ ...MockChatMessage, id: 3, usage: { input_tokens: 300 } },
		];
		const result = getLatestContextUsage(messages);
		expect(result?.inputTokens).toBe(300);
	});

	it("returns null when a compaction summary is newer than usage", () => {
		const messages = [
			{ ...MockChatMessage, id: 1, usage: { input_tokens: 100 } },
			compactionSummaryMessage,
		];
		expect(getLatestContextUsage(messages)).toBeNull();
	});

	it("returns null when a context clear is newer than usage", () => {
		const messages = [
			{ ...MockChatMessage, id: 1, usage: { input_tokens: 100 } },
			{
				...MockChatMessage,
				id: 2,
				role: "tool" as const,
				content: [{ type: "tool-result" as const, tool_name: "chat_cleared" }],
			},
		];
		expect(getLatestContextUsage(messages)).toBeNull();
	});

	it("returns null when no messages have usage data", () => {
		const messages = [MockChatMessage, { ...MockChatMessage, id: 2 }];
		expect(getLatestContextUsage(messages)).toBeNull();
	});

	it("returns usage when it is newer than a compaction summary", () => {
		const messages = [
			compactionSummaryMessage,
			{ ...MockChatMessage, id: 3, usage: { input_tokens: 300 } },
		];
		const result = getLatestContextUsage(messages);
		expect(result?.inputTokens).toBe(300);
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
		buildOption("openai:gpt-4", "openai", "gpt-4"),
		buildOption("anthropic:claude-3", "anthropic", "claude-3"),
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
	const buildAgent = (id: string): TypesGen.WorkspaceAgent =>
		({ id, name: `agent-${id}` }) as TypesGen.WorkspaceAgent;

	const buildWorkspace = (
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
		const ws = buildWorkspace([]);
		expect(getWorkspaceAgent(ws, "agent-1")).toBeUndefined();
	});

	it("returns the matching agent by id", () => {
		const ws = buildWorkspace([buildAgent("a1"), buildAgent("a2")]);
		expect(getWorkspaceAgent(ws, "a2")).toEqual(
			expect.objectContaining({ id: "a2" }),
		);
	});

	it("returns undefined when workspaceAgentId does not match", () => {
		const ws = buildWorkspace([buildAgent("a1"), buildAgent("a2")]);
		expect(getWorkspaceAgent(ws, "no-match")).toBeUndefined();
	});

	it("returns undefined when workspaceAgentId is undefined", () => {
		const ws = buildWorkspace([buildAgent("a1")]);
		expect(getWorkspaceAgent(ws, undefined)).toBeUndefined();
	});

	it("collects agents from multiple resources", () => {
		const ws = {
			latest_build: {
				resources: [
					{ agents: [buildAgent("r1-a1")] },
					{ agents: [buildAgent("r2-a1")] },
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
				resources: [{ agents: undefined }, { agents: [buildAgent("a1")] }],
			},
		} as unknown as TypesGen.Workspace;
		expect(getWorkspaceAgent(ws, "a1")).toEqual(
			expect.objectContaining({ id: "a1" }),
		);
	});
});
