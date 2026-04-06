import type * as TypesGen from "#/api/typesGenerated";
import { asRecord, asString } from "../ChatElements/runtimeTypeUtils";
import { appendTextBlock } from "./blockUtils";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
	ParsedToolCall,
	ParsedToolResult,
	RenderBlock,
} from "./types";

/** Concatenate text chunks, skipping whitespace-only values. */
const appendText = (current: string, next: string): string => {
	if (!next.trim()) {
		return current;
	}
	return `${current}${next}`;
};

const isSubagentToolName = (name: string): boolean =>
	name === "spawn_agent" ||
	name === "spawn_computer_use_agent" ||
	name === "wait_agent" ||
	name === "message_agent";

const isCompletedSubagentResult = (
	toolName: string,
	result: unknown,
): boolean => {
	if (!isSubagentToolName(toolName)) {
		return false;
	}
	const typedResult = asRecord(result);
	if (!typedResult) {
		return false;
	}
	const status = asString(
		typedResult.status ?? typedResult.subagent_status,
	).toLowerCase();
	return status === "completed" || status === "reported";
};

type ToolResultErrorBlock = {
	readonly is_error?: unknown;
	readonly error?: unknown;
};

export const parseToolResultIsError = (
	toolName: string,
	block: ToolResultErrorBlock,
	result: unknown,
): boolean => {
	if (typeof block.is_error === "boolean") {
		return block.is_error;
	}
	if (!block.error) {
		return false;
	}
	// Some providers include generic error metadata even on successful
	// subagent completions.
	return !isCompletedSubagentResult(toolName, result);
};

const emptyParsedMessageContent = (): ParsedMessageContent => ({
	markdown: "",
	reasoning: "",
	toolCalls: [],
	toolResults: [],
	tools: [],
	blocks: [],
	sources: [],
});

export const ensureToolBlock = (
	blocks: RenderBlock[],
	id: string,
): RenderBlock[] => {
	if (blocks.some((block) => block.type === "tool" && block.id === id)) {
		return blocks;
	}
	return [...blocks, { type: "tool", id }];
};

export const mergeTools = (
	calls: ParsedToolCall[],
	results: ParsedToolResult[],
): MergedTool[] => {
	const resultById = new Map(results.map((r) => [r.id, r]));
	const seen = new Set<string>();
	const merged: MergedTool[] = [];

	for (const call of calls) {
		seen.add(call.id);
		const result = resultById.get(call.id);
		// Extract model_intent from the tool call args if present.
		const callArgs = call.args as Record<string, unknown> | undefined;
		const modelIntent =
			typeof callArgs?.model_intent === "string"
				? callArgs.model_intent
				: undefined;
		merged.push({
			id: call.id,
			name: call.name,
			args: call.args,
			result: result?.result,
			isError: result?.isError ?? false,
			status: result ? (result.isError ? "error" : "completed") : "completed",
			mcpServerConfigId: call.mcpServerConfigId || result?.mcpServerConfigId,
			modelIntent,
		});
	}

	for (const result of results) {
		if (!seen.has(result.id)) {
			merged.push({
				id: result.id,
				name: result.name,
				result: result.result,
				isError: result.isError,
				status: result.isError ? "error" : "completed",
				mcpServerConfigId: result.mcpServerConfigId,
			});
		}
	}

	return merged;
};

export const parseMessageContent = (
	content: readonly TypesGen.ChatMessagePart[] | undefined,
): ParsedMessageContent => {
	if (!content || content.length === 0) {
		return emptyParsedMessageContent();
	}

	const parsed = emptyParsedMessageContent();
	for (const [index, part] of content.entries()) {
		switch (part.type) {
			case "text": {
				parsed.markdown = appendText(parsed.markdown, part.text);
				parsed.blocks = appendTextBlock(parsed.blocks, "response", part.text);
				break;
			}
			case "reasoning": {
				parsed.reasoning = appendText(parsed.reasoning, part.text);
				parsed.blocks = appendTextBlock(parsed.blocks, "thinking", part.text);
				break;
			}
			case "tool-call": {
				// Provider-executed tool calls (e.g. web_search) are
				// handled by the provider itself — hide them from the
				// tool card UI and let the sources component render
				// their results.
				if (part.provider_executed) {
					break;
				}
				const id = part.tool_call_id || `tool-call-${index}`;
				parsed.toolCalls.push({
					id,
					name: part.tool_name || "Tool",
					args: part.args,
					mcpServerConfigId: part.mcp_server_config_id,
				});
				parsed.blocks = ensureToolBlock(parsed.blocks, id);
				break;
			}
			case "file-reference": {
				parsed.blocks.push(part);
				break;
			}
			case "tool-result": {
				// Skip synthetic results for provider-executed tools.
				if (part.provider_executed) {
					break;
				}
				const id = part.tool_call_id || `tool-result-${index}`;
				const name = part.tool_name || "Tool";
				parsed.toolResults.push({
					id,
					name,
					result: part.result,
					isError: parseToolResultIsError(name, part, part.result),
					mcpServerConfigId: part.mcp_server_config_id,
				});
				parsed.blocks = ensureToolBlock(parsed.blocks, id);
				break;
			}
			case "file": {
				if (part.data || part.file_id) {
					parsed.blocks = [...parsed.blocks, part];
				}
				break;
			}
			case "source": {
				if (part.url) {
					const source = { url: part.url, title: part.title || part.url };
					// Still populate the flat list for backward compat.
					if (!parsed.sources.some((s) => s.url === part.url)) {
						parsed.sources.push(source);
					}
					// Group consecutive sources into a single
					// inline block at this position.
					const lastBlock = parsed.blocks[parsed.blocks.length - 1];
					if (
						lastBlock &&
						lastBlock.type === "sources" &&
						!lastBlock.sources.some((s) => s.url === part.url)
					) {
						lastBlock.sources.push(source);
					} else if (!lastBlock || lastBlock.type !== "sources") {
						parsed.blocks.push({
							type: "sources",
							sources: [source],
						});
					}
				}
				break;
			}
			case "context-file": {
				// Context files are metadata for the context indicator;
				// they are not rendered in the conversation timeline.
				break;
			}
			case "skill": {
				// Skill parts are metadata for the context indicator;
				// they are not rendered in the conversation timeline.
				break;
			}
			default: {
				const _exhaustive: never = part;
				break;
			}
		}
	}
	return parsed;
};

const isEditableUserMessageFileBlock = (
	block: RenderBlock,
): block is TypesGen.ChatFilePart =>
	block.type === "file" &&
	(block.media_type.startsWith("image/") || block.media_type === "text/plain");

export const getEditableUserMessagePayload = (
	message: TypesGen.ChatMessage,
): {
	text: string;
	fileBlocks: readonly TypesGen.ChatMessagePart[] | undefined;
} => {
	const parsed = parseMessageContent(message.content);
	const fileBlocks = parsed.blocks.filter(isEditableUserMessageFileBlock);
	return {
		text: parsed.markdown || "",
		fileBlocks: fileBlocks.length > 0 ? fileBlocks : undefined,
	};
};

export const parseMessagesWithMergedTools = (
	messages: readonly TypesGen.ChatMessage[],
): ParsedMessageEntry[] => {
	const rawParsed = messages.map((message) => ({
		message,
		parsed: parseMessageContent(message.content),
	}));

	const globalToolResults = new Map<string, ParsedToolResult>();
	for (const { parsed } of rawParsed) {
		for (const result of parsed.toolResults) {
			globalToolResults.set(result.id, result);
		}
	}

	for (const { parsed } of rawParsed) {
		const resultById = new Map<string, ParsedToolResult>();
		for (const result of parsed.toolResults) {
			resultById.set(result.id, result);
		}
		for (const call of parsed.toolCalls) {
			if (!resultById.has(call.id)) {
				const global = globalToolResults.get(call.id);
				if (global) {
					resultById.set(global.id, global);
				}
			}
		}
		parsed.tools = mergeTools(
			parsed.toolCalls,
			Array.from(resultById.values()),
		);
	}

	// Annotate execute/process_output tools whose process was
	// later killed or terminated via process_signal.
	const signaledProcesses = new Map<string, "kill" | "terminate">();
	for (const { parsed } of rawParsed) {
		for (const tool of parsed.tools) {
			if (tool.name !== "process_signal") continue;
			const args = asRecord(tool.args);
			const result = asRecord(tool.result);
			if (!args || !result || !result.success) continue;
			const pid = asString(args.process_id);
			const sig = asString(args.signal);
			if (pid && (sig === "kill" || sig === "terminate"))
				signaledProcesses.set(pid, sig);
		}
	}
	if (signaledProcesses.size > 0) {
		for (const { parsed } of rawParsed) {
			for (const tool of parsed.tools) {
				if (tool.name !== "execute" && tool.name !== "process_output") continue;
				const rec = asRecord(tool.result);
				const args = asRecord(tool.args);
				const pid =
					(rec ? asString(rec.background_process_id) : "") ||
					(rec ? asString(rec.process_id) : "") ||
					(args ? asString(args.process_id) : "");
				const sig = pid ? signaledProcesses.get(pid) : undefined;
				if (sig) tool.killedBySignal = sig;
			}
		}
	}

	return rawParsed;
};

export const buildSubagentTitles = (
	parsedMessages: readonly ParsedMessageEntry[],
): Map<string, string> => {
	const map = new Map<string, string>();
	for (const { parsed } of parsedMessages) {
		for (const tool of parsed.tools) {
			if (
				tool.name !== "spawn_agent" &&
				tool.name !== "spawn_computer_use_agent"
			) {
				continue;
			}
			const rec = asRecord(tool.result);
			if (!rec) {
				continue;
			}
			const chatId = asString(rec.chat_id);
			const title = asString(rec.title);
			if (chatId && title) {
				map.set(chatId, title);
			}
		}
	}
	return map;
};

export const buildComputerUseSubagentIds = (
	parsedMessages: readonly ParsedMessageEntry[],
): Set<string> => {
	const ids = new Set<string>();
	for (const { parsed } of parsedMessages) {
		for (const tool of parsed.tools) {
			if (tool.name !== "spawn_computer_use_agent") {
				continue;
			}
			const rec = asRecord(tool.result);
			if (!rec) {
				continue;
			}
			const chatId = asString(rec.chat_id);
			if (chatId) {
				ids.add(chatId);
			}
		}
	}
	return ids;
};
