import type * as TypesGen from "#/api/typesGenerated";
import { asNumber, asRecord, asString } from "../ChatElements/runtimeTypeUtils";
import {
	getProvidedSubagentTitle,
	getSubagentChatId,
	getSubagentDescriptor,
	isSubagentToolName,
	type SubagentVariant,
} from "../ChatElements/tools/subagentDescriptor";
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
	hookNotices: [],
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

const isToolCallPart = (
	part: TypesGen.ChatMessagePart,
): part is TypesGen.ChatToolCallPart => part.type === "tool-call";

const isToolResultPart = (
	part: TypesGen.ChatMessagePart,
): part is TypesGen.ChatToolResultPart => part.type === "tool-result";

const chatHasActiveToolCalls = (status: TypesGen.ChatStatus | null): boolean =>
	status === "running" || status === "requires_action";

export const getPendingToolCallIDs = (
	messages: readonly TypesGen.ChatMessage[],
	chatStatus: TypesGen.ChatStatus | null,
): ReadonlySet<string> | undefined => {
	if (!chatHasActiveToolCalls(chatStatus)) {
		return undefined;
	}

	const resultIDs = new Set<string>();
	for (const message of messages) {
		for (const part of message.content ?? []) {
			if (isToolResultPart(part) && part.tool_call_id) {
				resultIDs.add(part.tool_call_id);
			}
		}
	}

	for (const message of messages.toReversed()) {
		if (message.role === "user") {
			return undefined;
		}
		if (message.role !== "assistant") {
			continue;
		}
		const pendingToolCallIDs = (message.content ?? [])
			.filter(isToolCallPart)
			.map((part) => part.tool_call_id)
			.filter((id): id is string => Boolean(id && !resultIDs.has(id)));
		return pendingToolCallIDs.length > 0
			? new Set(pendingToolCallIDs)
			: undefined;
	}

	return undefined;
};

type MergeToolsOptions = {
	pendingToolCallIDs?: ReadonlySet<string>;
};

export const mergeTools = (
	calls: ParsedToolCall[],
	results: ParsedToolResult[],
	options: MergeToolsOptions = {},
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
		const status = result
			? result.isError
				? "error"
				: "completed"
			: options.pendingToolCallIDs?.has(call.id)
				? "running"
				: "completed";
		merged.push({
			id: call.id,
			name: call.name,
			args: call.args,
			result: result?.result,
			isError: result?.isError ?? false,
			status,
			mcpServerConfigId: call.mcpServerConfigId || result?.mcpServerConfigId,
			modelIntent,
			parsedCommands: call.parsedCommands,
			hookRewritten: call.hookRewritten,
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
					parsedCommands: part.parsed_commands,
					mcpServerConfigId: part.mcp_server_config_id,
					hookRewritten: part.hook_rewritten,
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
			case "hook-notice": {
				if (part.text.trim()) {
					parsed.hookNotices.push(part.text);
				}
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

const isEditableAttachmentMediaType = (mediaType: string): boolean =>
	mediaType.startsWith("image/") ||
	mediaType === "text/plain" ||
	mediaType === "text/markdown" ||
	mediaType === "text/csv" ||
	mediaType === "application/json" ||
	mediaType === "application/pdf";

const isEditableUserMessageFileBlock = (
	block: RenderBlock,
): block is TypesGen.ChatFilePart =>
	block.type === "file" && isEditableAttachmentMediaType(block.media_type);

export const getEditableUserMessagePayload = (
	message: TypesGen.ChatMessage,
): {
	text: string;
	fileBlocks: readonly TypesGen.ChatMessagePart[] | undefined;
} => {
	// Concatenate text parts verbatim to match the server-side string_agg in
	// GetChatUserPromptsByChatID; parseMessageContent/appendText is for streaming and drops whitespace-only chunks.
	const text = (message.content ?? [])
		.filter((part): part is TypesGen.ChatTextPart => part.type === "text")
		.map((part) => part.text)
		.join("");
	const parsed = parseMessageContent(message.content);
	const fileBlocks = parsed.blocks.filter(isEditableUserMessageFileBlock);
	return {
		text,
		fileBlocks: fileBlocks.length > 0 ? fileBlocks : undefined,
	};
};

type ParseMessagesWithMergedToolsOptions = {
	pendingToolCallIDs?: ReadonlySet<string>;
};

export const parseMessagesWithMergedTools = (
	messages: readonly TypesGen.ChatMessage[],
	options: ParseMessagesWithMergedToolsOptions = {},
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
			{ pendingToolCallIDs: options.pendingToolCallIDs },
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
			if (!args || !result?.success) continue;
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

	// Annotate backgrounded execute calls with the live process
	// state derived from their process_output observations. The
	// execute row is the anchor readers scan for "is it still
	// running"; poll rows stay chronological, and the row that
	// owns the process flips in place as observations arrive.
	const processStateByPid = new Map<
		string,
		{ state: "running" | "exited"; exitCode?: number }
	>();
	for (const { parsed } of rawParsed) {
		for (const tool of parsed.tools) {
			if (tool.name === "process_output") {
				const rec = asRecord(tool.result);
				const toolArgs = asRecord(tool.args);
				const pid = toolArgs ? asString(toolArgs.process_id) : "";
				if (!rec || !pid) continue;
				// process_output reports running:true while alive; an
				// exited process reports its final exit code.
				if (rec.running === true) {
					processStateByPid.set(pid, { state: "running" });
				} else {
					const exitCode = asNumber(rec.exit_code, { parseString: true });
					processStateByPid.set(pid, {
						state: "exited",
						exitCode: exitCode ?? undefined,
					});
				}
				continue;
			}
			// process_list returns a snapshot of every tracked
			// process; it may be the only observation when the
			// agent lists instead of polling a specific process.
			if (tool.name === "process_list") {
				const rec = asRecord(tool.result);
				const processes = rec?.processes;
				if (!Array.isArray(processes)) continue;
				for (const proc of processes) {
					const procRec = asRecord(proc);
					if (!procRec) continue;
					const pid = asString(procRec.id);
					if (!pid) continue;
					if (procRec.running === true) {
						processStateByPid.set(pid, { state: "running" });
					} else {
						const exitCode = asNumber(procRec.exit_code, {
							parseString: true,
						});
						processStateByPid.set(pid, {
							state: "exited",
							exitCode: exitCode ?? undefined,
						});
					}
				}
			}
		}
	}
	for (const [pid, sig] of signaledProcesses) {
		// A signal is terminal even without a later observation.
		if (!processStateByPid.has(pid)) {
			processStateByPid.set(pid, {
				state: "exited",
				exitCode: sig === "kill" ? 137 : 143,
			});
		}
	}
	if (processStateByPid.size > 0) {
		for (const { message, parsed } of rawParsed) {
			for (const tool of parsed.tools) {
				if (tool.name !== "execute") continue;
				const rec = asRecord(tool.result);
				const pid = rec ? asString(rec.background_process_id) : "";
				const state = pid ? processStateByPid.get(pid) : undefined;
				if (state) {
					const createdMs = Date.parse(message.created_at);
					tool.backgroundProcess = {
						...state,
						startedAtMs: Number.isNaN(createdMs) ? undefined : createdMs,
					};
				}
			}
		}
	}

	return rawParsed;
};

export const buildSubagentMaps = (
	parsedMessages: readonly ParsedMessageEntry[],
): {
	titles: Map<string, string>;
	variants: Map<string, SubagentVariant>;
} => {
	const titles = new Map<string, string>();
	const variants = new Map<string, SubagentVariant>();

	for (const { parsed } of parsedMessages) {
		for (const tool of parsed.tools) {
			if (!isSubagentToolName(tool.name)) {
				continue;
			}

			const chatId = getSubagentChatId({
				args: tool.args,
				result: tool.result,
			});
			if (!chatId) {
				continue;
			}

			const descriptor = getSubagentDescriptor({
				name: tool.name,
				args: tool.args,
				result: tool.result,
				inferredVariant: variants.get(chatId),
			});
			if (!descriptor) {
				continue;
			}

			variants.set(chatId, descriptor.variant);

			const providedTitle = getProvidedSubagentTitle({
				args: tool.args,
				result: tool.result,
			});
			if (providedTitle) {
				titles.set(chatId, providedTitle);
			}
		}
	}

	return { titles, variants };
};
