import type * as TypesGen from "api/typesGenerated";
import { asRecord, asString } from "components/ai-elements/runtimeTypeUtils";
import { appendTextBlock, asNonEmptyString } from "./blockUtils";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
	ParsedMessageSection,
	ParsedToolCall,
	ParsedToolResult,
	RenderBlock,
} from "./types";

const appendText = (current: string, next: string): string => {
	const trimmed = next.trim();
	if (!trimmed) {
		return current;
	}
	if (!current) {
		return next;
	}
	return `${current}\n${next}`;
};

export const asOptionalTitle = (value: unknown): string | undefined =>
	asNonEmptyString(value);

export const normalizeBlockType = (value: unknown): string =>
	asString(value).toLowerCase().replace(/_/g, "-");

const isSubagentToolName = (name: string): boolean =>
	name === "spawn_agent" || name === "wait_agent" || name === "message_agent";

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
});

/** Wraps appendTextBlock with newline-joining for complete message blocks. */
const appendParsedTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
	title?: string,
): RenderBlock[] => appendTextBlock(blocks, type, text, title, appendText);

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
		merged.push({
			id: call.id,
			name: call.name,
			args: call.args,
			result: result?.result,
			isError: result?.isError ?? false,
			status: result ? (result.isError ? "error" : "completed") : "completed",
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
			});
		}
	}

	return merged;
};

export const parseMessageContent = (content: unknown): ParsedMessageContent => {
	if (typeof content === "string") {
		return {
			...emptyParsedMessageContent(),
			markdown: content,
		};
	}

	if (Array.isArray(content)) {
		const parsed = emptyParsedMessageContent();
		for (const [index, block] of content.entries()) {
			if (typeof block === "string") {
				parsed.markdown = appendText(parsed.markdown, block);
				parsed.blocks = appendParsedTextBlock(parsed.blocks, "response", block);
				continue;
			}

			const typedBlock = asRecord(block);
			if (!typedBlock) {
				continue;
			}

			switch (normalizeBlockType(typedBlock.type)) {
				case "text": {
					const text = asString(typedBlock.text);
					parsed.markdown = appendText(parsed.markdown, text);
					parsed.blocks = appendParsedTextBlock(
						parsed.blocks,
						"response",
						text,
					);
					break;
				}
				case "reasoning":
				case "thinking": {
					const text = asString(typedBlock.text);
					const title = asOptionalTitle(typedBlock.title);
					parsed.reasoning = appendText(parsed.reasoning, text);
					parsed.blocks = appendParsedTextBlock(
						parsed.blocks,
						"thinking",
						text,
						title,
					);
					break;
				}
				case "tool-call":
				case "toolcall": {
					const name =
						asString(typedBlock.tool_name) || asString(typedBlock.name);
					const id =
						asString(typedBlock.tool_call_id) ||
						asString(typedBlock.id) ||
						`tool-call-${index}`;
					parsed.toolCalls.push({
						id,
						name: name || "Tool",
						args: typedBlock.args ?? typedBlock.input ?? typedBlock.arguments,
					});
					parsed.blocks = ensureToolBlock(parsed.blocks, id);
					break;
				}
				case "tool-result":
				case "toolresult": {
					const name =
						asString(typedBlock.tool_name) || asString(typedBlock.name);
					const id =
						asString(typedBlock.tool_call_id) ||
						asString(typedBlock.id) ||
						`tool-result-${index}`;
					const result =
						typedBlock.result ??
						typedBlock.output ??
						typedBlock.content ??
						typedBlock.data;
					parsed.toolResults.push({
						id,
						name: name || "Tool",
						result,
						isError: parseToolResultIsError(name, typedBlock, result),
					});
					parsed.blocks = ensureToolBlock(parsed.blocks, id);
					break;
				}
				default: {
					const text = asString(typedBlock.text);
					parsed.markdown = appendText(parsed.markdown, text);
					parsed.blocks = appendParsedTextBlock(
						parsed.blocks,
						"response",
						text,
					);
					break;
				}
			}
		}
		return parsed;
	}

	if (content === null || content === undefined) {
		return emptyParsedMessageContent();
	}

	const typedContent = asRecord(content);
	if (!typedContent) {
		const markdown = String(content);
		return {
			...emptyParsedMessageContent(),
			markdown,
			blocks: appendParsedTextBlock([], "response", markdown),
		};
	}

	if (typedContent.type) {
		return parseMessageContent([typedContent]);
	}

	const markdown =
		asString(typedContent.text) || asString(typedContent.content);
	return {
		...emptyParsedMessageContent(),
		markdown,
		blocks: appendParsedTextBlock([], "response", markdown),
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

	return rawParsed;
};

export const buildSubagentTitles = (
	parsedMessages: readonly ParsedMessageEntry[],
): Map<string, string> => {
	const map = new Map<string, string>();
	for (const { parsed } of parsedMessages) {
		for (const tool of parsed.tools) {
			if (tool.name !== "spawn_agent") {
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

export const buildParsedMessageSections = (
	parsedMessages: readonly ParsedMessageEntry[],
): ParsedMessageSection[] => {
	const sections: ParsedMessageSection[] = [];

	for (const entry of parsedMessages) {
		if (entry.message.role === "user") {
			sections.push({ userEntry: entry, entries: [entry] });
			continue;
		}
		if (sections.length === 0) {
			sections.push({ userEntry: null, entries: [entry] });
			continue;
		}
		sections[sections.length - 1].entries.push(entry);
	}

	return sections;
};
