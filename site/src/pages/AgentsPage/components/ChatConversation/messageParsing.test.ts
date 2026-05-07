import { describe, expect, it } from "vitest";
import type { ChatMessage, ChatMessagePart } from "#/api/typesGenerated";
import { getSubagentDescriptor } from "../ChatElements/tools/subagentDescriptor";
import {
	buildSubagentMaps,
	getEditableUserMessagePayload,
	mergeTools,
	parseMessageContent,
	parseMessagesWithMergedTools,
	parseToolResultIsError,
} from "./messageParsing";

describe("parseToolResultIsError", () => {
	it("returns the boolean is_error when present", () => {
		expect(parseToolResultIsError("tool", { is_error: true }, null)).toBe(true);
		expect(parseToolResultIsError("tool", { is_error: false }, null)).toBe(
			false,
		);
	});

	it("returns false when no error indicator is present", () => {
		expect(parseToolResultIsError("tool", {}, null)).toBe(false);
	});

	it("returns true when error field is present for non-subagent tools", () => {
		expect(
			parseToolResultIsError("some_tool", { error: "something" }, null),
		).toBe(true);
	});

	it("returns false for completed subagent tools, including legacy spawn_subagent, even with error field", () => {
		expect(
			parseToolResultIsError(
				"spawn_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"wait_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"message_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"spawn_computer_use_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"spawn_subagent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"close_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
	});
});

describe("getEditableUserMessagePayload", () => {
	it("keeps only editable stored attachments", () => {
		const cases = [
			{
				message: {
					id: 1,
					chat_id: "chat-1",
					created_at: "2026-04-21T00:00:00.000Z",
					role: "user",
					content: [
						{ type: "text", text: "Please edit this draft." },
						{ type: "file", media_type: "image/png", file_id: "image-file" },
						{
							type: "file",
							media_type: "application/json",
							file_id: "json-file",
							name: "report.json",
						},
						{
							type: "file",
							media_type: "application/pdf",
							file_id: "pdf-file",
							name: "manual.pdf",
						},
						{
							type: "file",
							media_type: "application/zip",
							file_id: "zip-file",
							name: "archive.zip",
						},
					],
				} satisfies ChatMessage,
				want: {
					text: "Please edit this draft.",
					fileBlocks: [
						{ type: "file", media_type: "image/png", file_id: "image-file" },
						{
							type: "file",
							media_type: "application/json",
							file_id: "json-file",
							name: "report.json",
						},
						{
							type: "file",
							media_type: "application/pdf",
							file_id: "pdf-file",
							name: "manual.pdf",
						},
					],
				},
			},
			{
				message: {
					id: 2,
					chat_id: "chat-1",
					created_at: "2026-04-21T00:00:00.000Z",
					role: "user",
					content: [
						{ type: "text", text: "Share the archive instead." },
						{
							type: "file",
							media_type: "application/zip",
							file_id: "zip-file",
							name: "archive.zip",
						},
					],
				} satisfies ChatMessage,
				want: {
					text: "Share the archive instead.",
					fileBlocks: undefined,
				},
			},
		];

		for (const { message, want } of cases) {
			expect(getEditableUserMessagePayload(message)).toEqual(want);
		}
	});
});

describe("parseMessageContent", () => {
	it("returns empty result for undefined content", () => {
		const result = parseMessageContent(undefined);
		expect(result.markdown).toBe("");
		expect(result.blocks).toEqual([]);
	});

	it("returns empty result for an empty array", () => {
		const result = parseMessageContent([]);
		expect(result.markdown).toBe("");
		expect(result.blocks).toEqual([]);
		expect(result.toolCalls).toEqual([]);
		expect(result.toolResults).toEqual([]);
	});

	it("parses a single text block", () => {
		const result = parseMessageContent([{ type: "text", text: "Hello" }]);
		expect(result.markdown).toBe("Hello");
		expect(result.blocks).toEqual([{ type: "response", text: "Hello" }]);
	});

	it("merges multiple text blocks into a single response block", () => {
		const result = parseMessageContent([
			{ type: "text", text: "Line one" },
			{ type: "text", text: "Line two" },
		]);
		expect(result.markdown).toBe("Line oneLine two");
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Line oneLine two",
		});
	});

	it("normalizes list markers split across text blocks", () => {
		// LLMs stream list markers and item content as separate text
		// blocks. Both paths (streaming and completed) must concatenate
		// them directly so the marker and content stay on the same line.
		const result = parseMessageContent([
			{ type: "text", text: "Intro\n\n- " },
			{ type: "text", text: "First item" },
			{ type: "text", text: "\n- " },
			{ type: "text", text: "Second item" },
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Intro\n\n- First item\n- Second item",
		});
	});

	it("parses a reasoning block", () => {
		const result = parseMessageContent([
			{ type: "reasoning", text: "Let me think..." },
		]);
		expect(result.reasoning).toBe("Let me think...");
		expect(result.blocks).toEqual([
			{ type: "thinking", text: "Let me think..." },
		]);
	});

	it("parses a tool_use / tool-call block", () => {
		const result = parseMessageContent([
			{
				type: "tool-call",
				tool_name: "bash",
				tool_call_id: "call-1",
				args: { command: "ls" },
			},
		]);
		expect(result.toolCalls).toHaveLength(1);
		expect(result.toolCalls[0]).toEqual({
			id: "call-1",
			name: "bash",
			args: { command: "ls" },
		});
		expect(result.blocks).toEqual([{ type: "tool", id: "call-1" }]);
	});

	it("parses a tool-result block", () => {
		const result = parseMessageContent([
			{
				type: "tool-result",
				tool_name: "bash",
				tool_call_id: "call-1",
				result: { output: "file.txt" },
			},
		]);
		expect(result.toolResults).toHaveLength(1);
		expect(result.toolResults[0]).toEqual({
			id: "call-1",
			name: "bash",
			result: { output: "file.txt" },
			isError: false,
		});
		expect(result.blocks).toEqual([{ type: "tool", id: "call-1" }]);
	});

	it("handles interleaved text and tool blocks in correct order", () => {
		const result = parseMessageContent([
			{ type: "text", text: "Starting..." },
			{
				type: "tool-call",
				tool_name: "bash",
				tool_call_id: "call-1",
				args: {},
			},
			{
				type: "tool-result",
				tool_name: "bash",
				tool_call_id: "call-1",
				result: { output: "ok" },
			},
			{ type: "text", text: "Done!" },
		]);
		expect(result.blocks).toHaveLength(3);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Starting...",
		});
		expect(result.blocks[1]).toEqual({ type: "tool", id: "call-1" });
		// The second text block creates a new response block after the
		// tool block.
		expect(result.blocks[2]).toEqual({ type: "response", text: "Done!" });
	});

	it("generates fallback IDs when tool_call_id is missing", () => {
		const result = parseMessageContent([
			{ type: "tool-call", tool_name: "run" },
		]);
		expect(result.toolCalls[0].id).toBe("tool-call-0");
	});

	it("extracts fileId from a file block with file_id", () => {
		const result = parseMessageContent([
			{
				type: "file",
				media_type: "image/png",
				file_id: "abc-123-def",
			},
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "file",
			media_type: "image/png",
			data: undefined,
			file_id: "abc-123-def",
		});
	});

	it("parses a file block without file_id (backward compat)", () => {
		const result = parseMessageContent([
			{
				type: "file",
				media_type: "image/png",
				data: "iVBORw0KGgo=",
			},
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "file",
			media_type: "image/png",
			data: "iVBORw0KGgo=",
			file_id: undefined,
		});
	});

	it("skips file parts without data or file_id", () => {
		const result = parseMessageContent([
			{
				type: "file",
				media_type: "image/png",
			},
		]);
		expect(result.blocks).toHaveLength(0);
	});

	it("parses a file-reference block into blocks", () => {
		const result = parseMessageContent([
			{
				type: "file-reference",
				file_name: "src/main.go",
				start_line: 10,
				end_line: 15,
				content: "some added code lines",
			},
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "file-reference",
			file_name: "src/main.go",
			start_line: 10,
			end_line: 15,
			content: "some added code lines",
		});
	});

	it("does not affect markdown when file-reference blocks are present", () => {
		const result = parseMessageContent([
			{ type: "text", text: "Hello" },
			{
				type: "file-reference",
				file_name: "a.go",
				start_line: 1,
				end_line: 2,
				content: "nit code content",
			},
		]);
		expect(result.markdown).toBe("Hello");
		expect(result.blocks).toHaveLength(2);
		expect(result.blocks[0]).toEqual({ type: "response", text: "Hello" });
		expect(result.blocks[1]).toEqual({
			type: "file-reference",
			file_name: "a.go",
			start_line: 1,
			end_line: 2,
			content: "nit code content",
		});
	});

	it("skips provider_executed tool-call parts", () => {
		const result = parseMessageContent([
			{
				type: "tool-call",
				tool_name: "web_search",
				tool_call_id: "tc-1",
				provider_executed: true,
			},
		]);
		expect(result.toolCalls).toEqual([]);
		expect(result.blocks.some((b) => b.type === "tool")).toBe(false);
	});

	it("skips provider_executed tool-result parts", () => {
		const result = parseMessageContent([
			{
				type: "tool-result",
				tool_name: "web_search",
				tool_call_id: "tc-1",
				provider_executed: true,
				result: { output: "results" },
			},
		]);
		expect(result.toolResults).toEqual([]);
		expect(result.blocks.some((b) => b.type === "tool")).toBe(false);
	});

	it("parses a source part into a sources block", () => {
		const result = parseMessageContent([
			{ type: "source", url: "https://example.com", title: "Example" },
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "sources",
			sources: [{ url: "https://example.com", title: "Example" }],
		});
		expect(result.sources).toEqual([
			{ url: "https://example.com", title: "Example" },
		]);
	});

	it("groups multiple consecutive sources into a single sources block", () => {
		const result = parseMessageContent([
			{ type: "source", url: "https://example.com", title: "Example" },
			{ type: "source", url: "https://other.com", title: "Other" },
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "sources",
			sources: [
				{ url: "https://example.com", title: "Example" },
				{ url: "https://other.com", title: "Other" },
			],
		});
		expect(result.sources).toHaveLength(2);
	});
});

describe("mergeTools", () => {
	it("merges tool calls with matching results", () => {
		const merged = mergeTools(
			[{ id: "1", name: "bash", args: { cmd: "ls" } }],
			[{ id: "1", name: "bash", result: "ok", isError: false }],
		);
		expect(merged).toHaveLength(1);
		expect(merged[0]).toEqual({
			id: "1",
			name: "bash",
			args: { cmd: "ls" },
			result: "ok",
			isError: false,
			status: "completed",
		});
	});

	it("includes orphaned results that have no matching call", () => {
		const merged = mergeTools(
			[],
			[{ id: "1", name: "bash", result: "output", isError: false }],
		);
		expect(merged).toHaveLength(1);
		expect(merged[0].id).toBe("1");
		expect(merged[0].status).toBe("completed");
	});

	it("marks error results with error status", () => {
		const merged = mergeTools(
			[{ id: "1", name: "bash" }],
			[{ id: "1", name: "bash", result: "fail", isError: true }],
		);
		expect(merged[0].isError).toBe(true);
		expect(merged[0].status).toBe("error");
	});

	it("returns completed for calls without results", () => {
		const merged = mergeTools([{ id: "1", name: "bash" }], []);
		expect(merged).toHaveLength(1);
		expect(merged[0].status).toBe("completed");
	});
});

describe("parseMessagesWithMergedTools — killedBySignal annotation", () => {
	const msg = (
		id: number,
		role: "assistant" | "user",
		parts: ChatMessagePart[],
	): ChatMessage => ({
		id,
		chat_id: "chat-1",
		created_at: new Date().toISOString(),
		role,
		content: parts,
	});

	// The generated types use Record<string, string> for args/result,
	// but real tool data contains booleans and nulls. We widen the
	// parameter types and cast back to ChatMessagePart.
	const toolCall = (
		id: string,
		name: string,
		args: Record<string, string>,
	): ChatMessagePart => ({
		type: "tool-call" as const,
		tool_call_id: id,
		tool_name: name,
		args,
	});
	const toolResult = (
		id: string,
		name: string,
		result: Record<string, unknown>,
		isError = false,
	): ChatMessagePart => ({
		type: "tool-result" as const,
		tool_call_id: id,
		tool_name: name,
		// The generated type uses Record<string, string> but real
		// tool results contain booleans and nulls.
		result: result as Record<string, string>,
		is_error: isError,
	});

	it("annotates execute tool with killedBySignal from a later process_signal", () => {
		const PID = "abc-123";
		const parsed = parseMessagesWithMergedTools([
			msg(1, "assistant", [
				toolCall("tc1", "execute", { command: "make build" }),
			]),
			msg(2, "assistant", [
				toolResult("tc1", "execute", {
					success: true,
					output: "",
					background_process_id: PID,
				}),
				toolCall("tc2", "process_signal", {
					process_id: PID,
					signal: "kill",
				}),
			]),
			msg(3, "assistant", [
				toolResult("tc2", "process_signal", {
					success: true,
					message: `signal "kill" sent to process ${PID}`,
				}),
			]),
		]);

		const executeTool = parsed
			.flatMap((e) => e.parsed.tools)
			.find((t) => t.name === "execute");
		expect(executeTool?.killedBySignal).toBe("kill");
	});

	it("does not annotate when process_signal failed", () => {
		const PID = "abc-123";
		const parsed = parseMessagesWithMergedTools([
			msg(1, "assistant", [
				toolCall("tc1", "execute", { command: "make build" }),
			]),
			msg(2, "assistant", [
				toolResult("tc1", "execute", {
					success: true,
					output: "",
					background_process_id: PID,
				}),
				toolCall("tc2", "process_signal", {
					process_id: PID,
					signal: "kill",
				}),
			]),
			msg(3, "assistant", [
				toolResult("tc2", "process_signal", {
					success: false,
					error: "process not found",
				}),
			]),
		]);

		const executeTool = parsed
			.flatMap((e) => e.parsed.tools)
			.find((t) => t.name === "execute");
		expect(executeTool?.killedBySignal).toBeUndefined();
	});

	it("annotates process_output via args.process_id", () => {
		const PID = "def-456";
		const parsed = parseMessagesWithMergedTools([
			msg(1, "assistant", [
				toolCall("tc1", "process_output", { process_id: PID }),
			]),
			msg(2, "assistant", [
				toolResult("tc1", "process_output", {
					output: "some output",
					exit_code: null,
				}),
				toolCall("tc2", "process_signal", {
					process_id: PID,
					signal: "terminate",
				}),
			]),
			msg(3, "assistant", [
				toolResult("tc2", "process_signal", {
					success: true,
					message: "signal sent",
				}),
			]),
		]);

		const procOut = parsed
			.flatMap((e) => e.parsed.tools)
			.find((t) => t.name === "process_output");
		expect(procOut?.killedBySignal).toBe("terminate");
	});
});

describe("subagent transcript parsing", () => {
	const msg = (
		id: number,
		parts: ChatMessagePart[],
		role: "assistant" | "user" = "assistant",
	): ChatMessage => ({
		id,
		chat_id: "chat-1",
		created_at: new Date().toISOString(),
		role,
		content: parts,
	});

	const toolCall = (
		id: string,
		name: string,
		args: Record<string, unknown> = {},
	): ChatMessagePart => ({
		type: "tool-call",
		tool_call_id: id,
		tool_name: name,
		args: args as Record<string, string>,
	});

	const toolResult = (
		id: string,
		name: string,
		result: Record<string, unknown>,
	): ChatMessagePart => ({
		type: "tool-result",
		tool_call_id: id,
		tool_name: name,
		result: result as Record<string, string>,
	});

	const parseSubagents = (messages: readonly ChatMessage[]) => {
		const parsedMessages = parseMessagesWithMergedTools(messages);
		const { titles, variants } = buildSubagentMaps(parsedMessages);
		return {
			parsedMessages,
			titles,
			variants,
		};
	};

	it("keeps legacy spawn tool parsing intact", () => {
		const { parsedMessages, titles, variants } = parseSubagents([
			msg(1, [
				toolCall("legacy-general", "spawn_agent", {
					title: "Legacy general",
				}),
				toolResult("legacy-general", "spawn_agent", {
					chat_id: "legacy-general-child",
					title: "Legacy general",
					status: "completed",
				}),
			]),
			msg(2, [
				toolCall("legacy-explore", "spawn_explore_agent", {}),
				toolResult("legacy-explore", "spawn_explore_agent", {
					chat_id: "legacy-explore-child",
					status: "completed",
				}),
			]),
			msg(3, [
				toolCall("legacy-desktop", "spawn_computer_use_agent", {
					title: "Legacy desktop",
				}),
				toolResult("legacy-desktop", "spawn_computer_use_agent", {
					chat_id: "legacy-desktop-child",
					title: "Legacy desktop",
					status: "completed",
				}),
			]),
		]);

		expect(parsedMessages[0]?.parsed.tools[0]?.name).toBe("spawn_agent");
		expect(parsedMessages[1]?.parsed.tools[0]?.name).toBe(
			"spawn_explore_agent",
		);
		expect(parsedMessages[2]?.parsed.tools[0]?.name).toBe(
			"spawn_computer_use_agent",
		);
		expect(titles.get("legacy-general-child")).toBe("Legacy general");
		expect(variants.get("legacy-general-child")).toBe("general");
		expect(variants.get("legacy-explore-child")).toBe("explore");
		expect(variants.get("legacy-desktop-child")).toBe("computer_use");
	});

	it("keeps legacy spawn_subagent payload parsing intact", () => {
		const { titles, variants } = parseSubagents([
			msg(1, [
				toolCall("legacy-unified", "spawn_subagent", {
					subagent_type: "explore",
					title: "Legacy unified",
				}),
				toolResult("legacy-unified", "spawn_subagent", {
					chat_id: "legacy-unified-child",
					subagent_type: "explore",
					title: "Legacy unified",
					status: "completed",
				}),
			]),
		]);

		expect(titles.get("legacy-unified-child")).toBe("Legacy unified");
		expect(variants.get("legacy-unified-child")).toBe("explore");
	});

	it("parses spawn_agent variants from args and results", () => {
		const { titles, variants } = parseSubagents([
			msg(1, [
				toolCall("spawn-general", "spawn_agent", {
					type: "general",
					title: "General helper",
				}),
				toolResult("spawn-general", "spawn_agent", {
					chat_id: "spawn-general-child",
					type: "general",
					title: "General helper",
					status: "completed",
				}),
			]),
			msg(2, [
				toolCall("spawn-explore", "spawn_agent", {
					type: "explore",
				}),
				toolResult("spawn-explore", "spawn_agent", {
					chat_id: "spawn-explore-child",
					type: "explore",
					status: "completed",
				}),
			]),
			msg(3, [
				toolCall("spawn-desktop", "spawn_agent", {
					type: "computer_use",
					title: "Desktop helper",
				}),
				toolResult("spawn-desktop", "spawn_agent", {
					chat_id: "spawn-desktop-child",
					type: "computer_use",
					title: "Desktop helper",
					status: "completed",
				}),
			]),
		]);

		expect(titles.get("spawn-general-child")).toBe("General helper");
		expect(variants.get("spawn-general-child")).toBe("general");
		expect(variants.get("spawn-explore-child")).toBe("explore");
		expect(variants.get("spawn-desktop-child")).toBe("computer_use");
	});

	it("buildSubagentMaps merges mixed legacy and spawn_agent transcripts coherently", () => {
		const parsedMessages = parseMessagesWithMergedTools([
			msg(1, [toolCall("legacy", "spawn_agent", { title: "Legacy helper" })]),
			msg(2, [
				toolResult("legacy", "spawn_agent", {
					chat_id: "legacy-child",
					title: "Legacy helper",
					status: "completed",
				}),
			]),
			msg(3, [
				toolCall("unified", "spawn_agent", {
					type: "explore",
					title: "Unified helper",
				}),
			]),
			msg(4, [
				toolResult("unified", "spawn_agent", {
					chat_id: "unified-child",
					type: "explore",
					title: "Unified helper",
					status: "completed",
				}),
			]),
		]);
		const { titles, variants } = buildSubagentMaps(parsedMessages);

		expect(parsedMessages[0]?.parsed.tools[0]?.result).toMatchObject({
			chat_id: "legacy-child",
			title: "Legacy helper",
		});
		expect(parsedMessages[2]?.parsed.tools[0]?.result).toMatchObject({
			chat_id: "unified-child",
			title: "Unified helper",
			type: "explore",
		});
		expect(titles.get("legacy-child")).toBe("Legacy helper");
		expect(titles.get("unified-child")).toBe("Unified helper");
		expect(variants.get("legacy-child")).toBe("general");
		expect(variants.get("unified-child")).toBe("explore");
	});

	it("includes close_agent in the shared subagent parsing path", () => {
		const { variants } = parseSubagents([
			msg(1, [
				toolCall("close-tool", "close_agent", { chat_id: "closing-child" }),
				toolResult("close-tool", "close_agent", {
					chat_id: "closing-child",
					type: "explore",
					status: "completed",
				}),
			]),
		]);

		expect(variants.get("closing-child")).toBe("explore");
	});

	it("tracks computer-use variants for legacy and spawn_agent tools", () => {
		const { variants } = parseSubagents([
			msg(1, [
				toolCall("legacy-desktop", "spawn_computer_use_agent", {}),
				toolResult("legacy-desktop", "spawn_computer_use_agent", {
					chat_id: "legacy-desktop-child",
					status: "completed",
				}),
			]),
			msg(2, [
				toolCall("unified-desktop", "spawn_agent", {
					type: "computer_use",
				}),
				toolResult("unified-desktop", "spawn_agent", {
					chat_id: "unified-desktop-child",
					type: "computer_use",
					status: "completed",
				}),
			]),
		]);

		expect(variants.get("legacy-desktop-child")).toBe("computer_use");
		expect(variants.get("unified-desktop-child")).toBe("computer_use");
	});

	it("prefers lifecycle result type metadata when present", () => {
		const { variants } = parseSubagents([
			msg(1, [
				toolCall("wait-tool", "wait_agent", { chat_id: "wait-child" }),
				toolResult("wait-tool", "wait_agent", {
					chat_id: "wait-child",
					type: "explore",
					status: "completed",
				}),
			]),
			msg(2, [
				toolCall("message-tool", "message_agent", {
					chat_id: "message-child",
					message: "continue",
				}),
				toolResult("message-tool", "message_agent", {
					chat_id: "message-child",
					type: "computer_use",
					status: "completed",
				}),
			]),
			msg(3, [
				toolCall("close-tool", "close_agent", { chat_id: "close-child" }),
				toolResult("close-tool", "close_agent", {
					chat_id: "close-child",
					type: "general",
					status: "completed",
				}),
			]),
		]);

		expect(variants.get("wait-child")).toBe("explore");
		expect(variants.get("message-child")).toBe("computer_use");
		expect(variants.get("close-child")).toBe("general");
	});

	it("preserves lifecycle variants inferred from earlier spawn history", () => {
		const { titles, variants } = parseSubagents([
			msg(1, [
				toolCall("spawn-tool", "spawn_agent", {
					type: "explore",
					title: "Inspect repository",
				}),
				toolResult("spawn-tool", "spawn_agent", {
					chat_id: "history-child",
					type: "explore",
					title: "Inspect repository",
					status: "completed",
				}),
			]),
			msg(2, [
				toolCall("wait-tool", "wait_agent", { chat_id: "history-child" }),
				toolResult("wait-tool", "wait_agent", {
					chat_id: "history-child",
					status: "completed",
				}),
			]),
			msg(3, [
				toolCall("message-tool", "message_agent", {
					chat_id: "history-child",
					message: "keep going",
				}),
				toolResult("message-tool", "message_agent", {
					chat_id: "history-child",
					status: "completed",
				}),
			]),
			msg(4, [
				toolCall("close-tool", "close_agent", { chat_id: "history-child" }),
				toolResult("close-tool", "close_agent", {
					chat_id: "history-child",
					status: "completed",
				}),
			]),
		]);

		expect(titles.get("history-child")).toBe("Inspect repository");
		expect(variants.get("history-child")).toBe("explore");
	});
});

describe("getSubagentDescriptor", () => {
	it("uses the inferred variant for lifecycle tools without explicit metadata", () => {
		const lifecycleTools = [
			{ name: "wait_agent", action: "wait" },
			{ name: "message_agent", action: "message" },
			{ name: "close_agent", action: "close" },
		] as const;

		for (const tool of lifecycleTools) {
			const descriptor = getSubagentDescriptor({
				name: tool.name,
				args: { chat_id: "desktop-child" },
				result: { chat_id: "desktop-child", status: "running" },
				inferredVariant: "computer_use",
			});

			expect(descriptor).toMatchObject({
				action: tool.action,
				variant: "computer_use",
				iconKind: "monitor",
				supportsDesktopAffordance: true,
			});
		}
	});

	it("falls back to the general lifecycle variant without an inference", () => {
		const lifecycleToolNames = [
			"wait_agent",
			"message_agent",
			"close_agent",
		] as const;

		for (const name of lifecycleToolNames) {
			const descriptor = getSubagentDescriptor({
				name,
				args: { chat_id: "general-child" },
				result: { chat_id: "general-child", status: "running" },
			});

			expect(descriptor).toMatchObject({
				variant: "general",
				iconKind: "bot",
				supportsDesktopAffordance: false,
			});
		}
	});
});
