import { API, type ChatDiffStatusResponse, watchChat } from "api/api";
import {
	chat,
	chatDiffContentsKey,
	chatDiffStatus,
	chatDiffStatusKey,
	chats,
	chatModels,
	chatsKey,
	createChatMessage,
	deleteChatQueuedMessage,
	interruptChat,
	promoteChatQueuedMessage,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import {
	ConversationItem,
	Message,
	MessageContent,
	type ModelSelectorOption,
	Response,
	Shimmer,
	Thinking,
	Tool,
} from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { displayError } from "components/GlobalSnackbar/utils";
import { Skeleton } from "components/Skeleton/Skeleton";
import {
	ArchiveIcon,
	ChevronRightIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	MonitorIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
} from "lucide-react";
import { SESSION_TOKEN_PLACEHOLDER, getVSCodeHref } from "modules/apps/apps";
import {
	type FC,
	memo,
	startTransition,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { AgentChatInput, type AgentContextUsage } from "./AgentChatInput";
import { QueuedMessagesList } from "./QueuedMessagesList";
import type { AgentsOutletContext } from "./AgentsPage";
import { FilesChangedPanel } from "./FilesChangedPanel";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";

type ChatModelOption = ModelSelectorOption;

const asRecord = (value: unknown): Record<string, unknown> | null => {
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return null;
	}
	return value as Record<string, unknown>;
};

const asString = (value: unknown): string => {
	return typeof value === "string" ? value : "";
};

const asTokenCount = (value: unknown): number | undefined => {
	if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
		return undefined;
	}
	return value;
};

const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

const defaultContextCompressionThreshold = "70";

const contextCompressionThresholdStorageKey = (modelID: string): string =>
	`agents.context-compression-threshold.${modelID || "default"}`;

const parseContextCompressionThreshold = (
	value: string,
): number | undefined => {
	const parsedValue = Number.parseInt(value.trim(), 10);
	if (!Number.isFinite(parsedValue)) {
		return undefined;
	}
	if (parsedValue < 0 || parsedValue > 100) {
		return undefined;
	}
	return parsedValue;
};

type ChatMessageWithUsage = TypesGen.ChatMessage & {
	readonly input_tokens?: unknown;
	readonly output_tokens?: unknown;
	readonly total_tokens?: unknown;
	readonly reasoning_tokens?: unknown;
	readonly cache_creation_tokens?: unknown;
	readonly cache_read_tokens?: unknown;
	readonly context_limit?: unknown;
};

const extractContextUsageFromMessage = (
	message: TypesGen.ChatMessage,
): AgentContextUsage | null => {
	const withUsage = message as ChatMessageWithUsage;
	const inputTokens = asTokenCount(withUsage.input_tokens);
	const outputTokens = asTokenCount(withUsage.output_tokens);
	const totalTokens = asTokenCount(withUsage.total_tokens);
	const reasoningTokens = asTokenCount(withUsage.reasoning_tokens);
	const cacheCreationTokens = asTokenCount(withUsage.cache_creation_tokens);
	const cacheReadTokens = asTokenCount(withUsage.cache_read_tokens);
	const contextLimitTokens = asTokenCount(withUsage.context_limit);

	const components = [
		inputTokens,
		outputTokens,
		cacheReadTokens,
		cacheCreationTokens,
		reasoningTokens,
	].filter((value): value is number => value !== undefined);
	const derivedUsedTokens =
		components.length > 0
			? components.reduce((total, value) => total + value, 0)
			: undefined;
	const usedTokens = totalTokens ?? derivedUsedTokens;

	const hasUsage =
		usedTokens !== undefined ||
		contextLimitTokens !== undefined ||
		inputTokens !== undefined ||
		outputTokens !== undefined ||
		cacheReadTokens !== undefined ||
		cacheCreationTokens !== undefined ||
		reasoningTokens !== undefined;
	if (!hasUsage) {
		return null;
	}

	return {
		usedTokens,
		contextLimitTokens,
		inputTokens,
		outputTokens,
		cacheReadTokens,
		cacheCreationTokens,
		reasoningTokens,
	};
};

const getLatestContextUsage = (
	messages: readonly TypesGen.ChatMessage[],
): AgentContextUsage | null => {
	for (let index = messages.length - 1; index >= 0; index -= 1) {
		const usage = extractContextUsageFromMessage(messages[index]);
		if (usage) {
			return usage;
		}
	}
	return null;
};

type ChatWithHierarchyMetadata = TypesGen.Chat & {
	readonly parent_chat_id?: string;
};

const getParentChatID = (chat: TypesGen.Chat | undefined): string | undefined => {
	return asNonEmptyString(
		(chat as ChatWithHierarchyMetadata | undefined)?.parent_chat_id,
	);
};

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

const normalizeBlockType = (value: unknown): string =>
	asString(value).toLowerCase().replace(/_/g, "-");

const isSubagentToolName = (name: string): boolean =>
	name === "subagent" ||
	name === "subagent_await" ||
	name === "subagent_message";

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

const parseToolResultIsError = (
	toolName: string,
	block: ToolResultErrorBlock,
	result: unknown,
): boolean => {
	if (typeof block.is_error === "boolean") {
		return block.is_error;
	}
	if (!Boolean(block.error)) {
		return false;
	}
	// Some providers include generic error metadata even on successful
	// subagent completions.
	return !isCompletedSubagentResult(toolName, result);
};

const tryParseJSONObject = (value: string): unknown | null => {
	const trimmed = value.trim();
	if (!trimmed) {
		return null;
	}
	const first = trimmed[0];
	if (first !== "{" && first !== "[") {
		return null;
	}
	try {
		return JSON.parse(trimmed);
	} catch {
		return null;
	}
};

const parsePartialJSONString = (
	input: string,
	startIndex: number,
): { value: string; nextIndex: number } | "incomplete" | null => {
	if (input[startIndex] !== "\"") {
		return null;
	}
	let escaped = false;
	for (let i = startIndex + 1; i < input.length; i += 1) {
		const char = input[i];
		if (escaped) {
			escaped = false;
			continue;
		}
		if (char === "\\") {
			escaped = true;
			continue;
		}
		if (char !== "\"") {
			continue;
		}
		const token = input.slice(startIndex, i + 1);
		try {
			return {
				value: JSON.parse(token) as string,
				nextIndex: i + 1,
			};
		} catch {
			return null;
		}
	}
	return "incomplete";
};

const isJSONValueBoundary = (char: string | undefined): boolean =>
	char === undefined || char === "," || char === "}" || char === "]" || /\s/.test(char);

const findBalancedJSONEnd = (
	input: string,
	startIndex: number,
): number | "incomplete" | null => {
	const stack: string[] = [];
	let escaped = false;
	let inString = false;

	for (let index = startIndex; index < input.length; index += 1) {
		const char = input[index];
		if (inString) {
			if (escaped) {
				escaped = false;
				continue;
			}
			if (char === "\\") {
				escaped = true;
				continue;
			}
			if (char === "\"") {
				inString = false;
			}
			continue;
		}

		switch (char) {
			case "\"":
				inString = true;
				break;
			case "{":
			case "[":
				stack.push(char);
				break;
			case "}": {
				const top = stack.pop();
				if (top !== "{") {
					return null;
				}
				break;
			}
			case "]": {
				const top = stack.pop();
				if (top !== "[") {
					return null;
				}
				break;
			}
			default:
				break;
		}

		if (stack.length === 0) {
			return index + 1;
		}
	}

	return "incomplete";
};

type PartialJSONValue =
	| { status: "ok"; value: unknown; nextIndex: number }
	| { status: "incomplete" }
	| { status: "invalid" };

const parsePartialJSONValue = (
	input: string,
	startIndex: number,
): PartialJSONValue => {
	let index = startIndex;
	while (index < input.length && /\s/.test(input[index])) {
		index += 1;
	}
	if (index >= input.length) {
		return { status: "incomplete" };
	}

	const char = input[index];
	if (char === "\"") {
		const parsed = parsePartialJSONString(input, index);
		if (parsed === "incomplete") {
			return { status: "incomplete" };
		}
		if (!parsed) {
			return { status: "invalid" };
		}
		return {
			status: "ok",
			value: parsed.value,
			nextIndex: parsed.nextIndex,
		};
	}

	if (char === "{" || char === "[") {
		const end = findBalancedJSONEnd(input, index);
		if (end === "incomplete") {
			return { status: "incomplete" };
		}
		if (end === null) {
			return { status: "invalid" };
		}
		const parsed = tryParseJSONObject(input.slice(index, end));
		if (parsed === null) {
			return { status: "invalid" };
		}
		return {
			status: "ok",
			value: parsed,
			nextIndex: end,
		};
	}

	if (input.startsWith("true", index)) {
		const next = index + 4;
		if (!isJSONValueBoundary(input[next])) {
			return { status: "invalid" };
		}
		return { status: "ok", value: true, nextIndex: next };
	}
	if ("true".startsWith(input.slice(index))) {
		return { status: "incomplete" };
	}

	if (input.startsWith("false", index)) {
		const next = index + 5;
		if (!isJSONValueBoundary(input[next])) {
			return { status: "invalid" };
		}
		return { status: "ok", value: false, nextIndex: next };
	}
	if ("false".startsWith(input.slice(index))) {
		return { status: "incomplete" };
	}

	if (input.startsWith("null", index)) {
		const next = index + 4;
		if (!isJSONValueBoundary(input[next])) {
			return { status: "invalid" };
		}
		return { status: "ok", value: null, nextIndex: next };
	}
	if ("null".startsWith(input.slice(index))) {
		return { status: "incomplete" };
	}

	if (char === "-" || (char >= "0" && char <= "9")) {
		let end = index;
		while (end < input.length && /[0-9eE+.-]/.test(input[end])) {
			end += 1;
		}
		const token = input.slice(index, end);
		if (!token) {
			return { status: "invalid" };
		}
		if (end === input.length && /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?)?$/.test(token)) {
			return { status: "incomplete" };
		}
		if (!/^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(token)) {
			return { status: "invalid" };
		}
		if (!isJSONValueBoundary(input[end])) {
			return { status: "invalid" };
		}
		return { status: "ok", value: Number(token), nextIndex: end };
	}

	return { status: "invalid" };
};

/**
 * Extracts the content of an incomplete JSON string literal that is
 * still being streamed. Given an opening quote at `startIndex`,
 * returns whatever text has been received so far (handling escape
 * sequences). Returns `null` if the character at startIndex is not
 * a quote.
 */
const extractIncompleteStringContent = (
	input: string,
	startIndex: number,
): string | null => {
	if (input[startIndex] !== '"') {
		return null;
	}
	let result = "";
	let escaped = false;
	for (let i = startIndex + 1; i < input.length; i += 1) {
		const char = input[i];
		if (escaped) {
			// Handle common JSON escape sequences.
			switch (char) {
				case '"': result += '"'; break;
				case '\\': result += '\\'; break;
				case '/': result += '/'; break;
				case 'n': result += '\n'; break;
				case 'r': result += '\r'; break;
				case 't': result += '\t'; break;
				default: result += `\\${char}`; break;
			}
			escaped = false;
			continue;
		}
		if (char === '\\') {
			escaped = true;
			continue;
		}
		if (char === '"') {
			// String is actually complete — shouldn't happen if
			// parsePartialJSONValue already returned "incomplete",
			// but return what we have.
			return result;
		}
		result += char;
	}
	return result.length > 0 ? result : null;
};

const parsePartialJSONObject = (value: string): Record<string, unknown> | null => {
	const trimmed = value.trim();
	if (!trimmed.startsWith("{")) {
		return null;
	}

	let index = 1;
	const parsed: Record<string, unknown> = {};
	let hasFields = false;

	while (index < trimmed.length) {
		while (index < trimmed.length && /\s/.test(trimmed[index])) {
			index += 1;
		}
		if (index >= trimmed.length) {
			break;
		}

		if (trimmed[index] === "}") {
			return hasFields ? parsed : null;
		}

		if (trimmed[index] === ",") {
			index += 1;
			continue;
		}

		const key = parsePartialJSONString(trimmed, index);
		if (key === "incomplete") {
			break;
		}
		if (!key) {
			return hasFields ? parsed : null;
		}
		index = key.nextIndex;

		while (index < trimmed.length && /\s/.test(trimmed[index])) {
			index += 1;
		}
		if (index >= trimmed.length || trimmed[index] !== ":") {
			break;
		}
		index += 1;

		const nextValue = parsePartialJSONValue(trimmed, index);
		if (nextValue.status === "incomplete") {
			// For incomplete string values, extract whatever content
			// has been received so far. This lets streaming tool args
			// (e.g. a command being typed) appear incrementally.
			const partialStr = extractIncompleteStringContent(trimmed, index);
			if (partialStr !== null) {
				parsed[key.value] = partialStr;
				hasFields = true;
			}
			break;
		}
		if (nextValue.status === "invalid") {
			return hasFields ? parsed : null;
		}

		parsed[key.value] = nextValue.value;
		hasFields = true;
		index = nextValue.nextIndex;

		while (index < trimmed.length && /\s/.test(trimmed[index])) {
			index += 1;
		}
		if (index >= trimmed.length) {
			break;
		}
		if (trimmed[index] === ",") {
			index += 1;
			continue;
		}
		if (trimmed[index] === "}") {
			return parsed;
		}
		return hasFields ? parsed : null;
	}

	return hasFields ? parsed : null;
};

const parseStreamingJSON = (value: string): unknown | null => {
	const complete = tryParseJSONObject(value);
	if (complete !== null) {
		return complete;
	}
	return parsePartialJSONObject(value);
};

type StreamPayloadMerge = {
	value: unknown;
	rawText?: string;
};

const mergeStreamPayload = (
	existingValue: unknown,
	existingRawText: string | undefined,
	value: unknown,
	delta: unknown,
): StreamPayloadMerge => {
	if (value !== undefined) {
		if (typeof value !== "string") {
			return { value };
		}
		const parsed = parseStreamingJSON(value);
		if (parsed !== null) {
			return { value: parsed, rawText: value };
		}
		return { value, rawText: value };
	}

	const chunk = typeof delta === "string" ? delta : "";
	if (!chunk) {
		return {
			value: existingValue,
			rawText: existingRawText,
		};
	}

	if (
		existingValue !== undefined &&
		typeof existingValue !== "string" &&
		existingRawText === undefined
	) {
		return {
			value: existingValue,
		};
	}

	const base = existingRawText ?? (typeof existingValue === "string" ? existingValue : "");
	const rawText = `${base}${chunk}`;
	const parsed = parseStreamingJSON(rawText);

	return {
		value: parsed ?? rawText,
		rawText,
	};
};

type ParsedToolCall = {
	id: string;
	name: string;
	args?: unknown;
};

type ParsedToolResult = {
	id: string;
	name: string;
	result?: unknown;
	isError: boolean;
};

type MergedTool = {
	id: string;
	name: string;
	args?: unknown;
	result?: unknown;
	isError: boolean;
	status: "completed" | "error" | "running";
};

type ParsedMessageContent = {
	markdown: string;
	reasoning: string;
	toolCalls: ParsedToolCall[];
	toolResults: ParsedToolResult[];
	tools: MergedTool[];
};

const emptyParsedMessageContent = (): ParsedMessageContent => ({
	markdown: "",
	reasoning: "",
	toolCalls: [],
	toolResults: [],
	tools: [],
});

const mergeTools = (
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

	// Results without a matching call (standalone tool-result parts).
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

const parseMessageContent = (content: unknown): ParsedMessageContent => {
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
				continue;
			}

			const typedBlock = asRecord(block);
			if (!typedBlock) {
				continue;
			}

			switch (normalizeBlockType(typedBlock.type)) {
				case "text":
					parsed.markdown = appendText(
						parsed.markdown,
						asString(typedBlock.text),
					);
					break;
				case "reasoning":
				case "thinking":
					parsed.reasoning = appendText(
						parsed.reasoning,
						asString(typedBlock.text),
					);
					break;
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
					break;
				}
				default:
					parsed.markdown = appendText(
						parsed.markdown,
						asString(typedBlock.text),
					);
					break;
			}
		}
		return parsed;
	}

	if (content === null || content === undefined) {
		return emptyParsedMessageContent();
	}

	const typedContent = asRecord(content);
	if (!typedContent) {
		return {
			...emptyParsedMessageContent(),
			markdown: String(content),
		};
	}

	if (Array.isArray(typedContent.parts)) {
		const parsed = emptyParsedMessageContent();
		for (const part of typedContent.parts) {
			const typedPart = asRecord(part);
			if (!typedPart) {
				continue;
			}
			if (normalizeBlockType(typedPart.type) === "text") {
				parsed.markdown = appendText(parsed.markdown, asString(typedPart.text));
			}
		}
		return parsed;
	}

	if (typedContent.type) {
		return parseMessageContent([typedContent]);
	}

	return {
		...emptyParsedMessageContent(),
		markdown: asString(typedContent.text) || asString(typedContent.content),
	};
};

const resolveModelFromChatConfig = (
	modelConfig: TypesGen.Chat["model_config"],
	modelOptions: readonly ChatModelOption[],
): string => {
	if (modelOptions.length === 0) {
		return "";
	}

	if (!modelConfig || typeof modelConfig !== "object") {
		return modelOptions[0]?.id ?? "";
	}

	const typedModelConfig = modelConfig as Record<string, unknown>;
	const model = asString(typedModelConfig.model);
	const provider = asString(typedModelConfig.provider);

	const candidates = [model];
	if (provider && model) {
		candidates.push(`${provider}:${model}`);
	}

	for (const candidate of candidates) {
		const match = modelOptions.find((option) => option.id === candidate);
		if (match) {
			return match.id;
		}
	}

	if (model) {
		const modelMatch = modelOptions.find(
			(option) =>
				option.model === model && (!provider || option.provider === provider),
		);
		if (modelMatch) {
			return modelMatch.id;
		}
	}

	return modelOptions[0]?.id ?? "";
};

const resolveContextCompressionThresholdFromChatConfig = (
	modelConfig: TypesGen.Chat["model_config"],
): string | null => {
	if (!modelConfig || typeof modelConfig !== "object") {
		return null;
	}

	const typedModelConfig = modelConfig as Record<string, unknown>;
	const threshold = typedModelConfig.context_compression_threshold;
	if (typeof threshold !== "number" || !Number.isFinite(threshold)) {
		return null;
	}
	if (threshold < 0 || threshold > 100) {
		return null;
	}
	return String(Math.trunc(threshold));
};

type StreamToolCall = {
	id: string;
	name: string;
	args?: unknown;
	argsRaw?: string;
};

type StreamToolResult = {
	id: string;
	name: string;
	result?: unknown;
	resultRaw?: string;
	isError: boolean;
};

type StreamState = {
	content: string;
	reasoning: string;
	toolCalls: Record<string, StreamToolCall>;
	toolResults: Record<string, StreamToolResult>;
};

const createEmptyStreamState = (): StreamState => ({
	content: "",
	reasoning: "",
	toolCalls: {},
	toolResults: {},
});

/**
 * Collects all tool results across every message into a single
 * lookup map keyed by tool call ID. This lets us match a tool
 * call in one assistant message with its result that arrives in
 * a later message.
 */
type CreateChatMessagePayload = TypesGen.CreateChatMessageRequest & {
	readonly model?: string;
};

const noopSetChatErrorReason: AgentsOutletContext["setChatErrorReason"] =
	() => {};
const noopClearChatErrorReason: AgentsOutletContext["clearChatErrorReason"] =
	() => {};
const noopSetRightPanelOpen: AgentsOutletContext["setRightPanelOpen"] =
	() => {};
const noopRequestArchiveAgent: AgentsOutletContext["requestArchiveAgent"] =
	() => {};

interface DiffStatsBadgeProps {
	status: ChatDiffStatusResponse;
	isOpen: boolean;
	onToggle: () => void;
}

const DiffStatsBadge: FC<DiffStatsBadgeProps> = ({
	status,
	isOpen,
	onToggle,
}) => {
	const additions = status.additions ?? 0;
	const deletions = status.deletions ?? 0;

	return (
		<div
			role="button"
			tabIndex={0}
			onClick={onToggle}
			onKeyDown={(e) => {
				if (e.key === "Enter" || e.key === " ") {
					onToggle();
				}
			}}
			className="flex cursor-pointer items-center gap-3 px-2 py-1 text-content-secondary transition-colors hover:text-content-primary"
		>
			<span className="font-mono text-sm font-semibold text-content-success">
				+{additions}
			</span>
			<span className="font-mono text-sm font-semibold text-content-destructive">
				−{deletions}
			</span>
			{isOpen ? (
				<PanelRightCloseIcon className="h-4 w-4" />
			) : (
				<PanelRightOpenIcon className="h-4 w-4" />
			)}
		</div>
	);
};

// ---------------------------------------------------------------------------
// Memoized sub-components
// ---------------------------------------------------------------------------

/**
 * Renders a single historic chat message. Wrapped in React.memo so
 * it only re-renders when its own parsed content changes — not on
 * every stream chunk or input keystroke.
 */
const ChatMessageItem = memo<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
}>(({ message, parsed }) => {
	const isUser = message.role === "user";

	// Skip messages that only carry tool results. Those results
	// are shown inline with the tool-call message they belong to.
	if (
		parsed.toolResults.length > 0 &&
		parsed.toolCalls.length === 0 &&
		parsed.markdown === "" &&
		parsed.reasoning === ""
	) {
		return null;
	}

	const hasRenderableContent =
		parsed.markdown !== "" ||
		parsed.reasoning !== "" ||
		parsed.tools.length > 0;
	const conversationItemProps = {
		role: (isUser ? "user" as const : "assistant" as const),
	};

	return (
		<ConversationItem {...conversationItemProps}>
			{isUser ? (
				<Message className="my-2 w-full max-w-none">
					<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
						{parsed.markdown || ""}
					</MessageContent>
				</Message>
			) : (
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<div className="space-y-3">
							{parsed.markdown && <Response>{parsed.markdown}</Response>}
							{parsed.reasoning && <Thinking>{parsed.reasoning}</Thinking>}
							{parsed.tools.map((tool) => (
								<Tool
									key={tool.id}
									name={tool.name}
									args={tool.args}
									result={tool.result}
									status={tool.status}
									isError={tool.isError}
								/>
							))}
							{!hasRenderableContent && (
								<div className="text-xs text-content-secondary">
									Message has no renderable content.
								</div>
							)}
						</div>
					</MessageContent>
				</Message>
			)}
		</ConversationItem>
	);
});
ChatMessageItem.displayName = "ChatMessageItem";

/**
 * Renders the live streaming assistant output. Isolated via memo so
 * historic messages and the input area are not re-rendered on each
 * chunk.
 */
const StreamingOutput = memo<{
	streamState: StreamState | null;
	streamTools: MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
}>(({ streamState, streamTools, subagentTitles, subagentStatusOverrides }) => {
	const conversationItemProps = { role: "assistant" as const };

	return (
		<ConversationItem {...conversationItemProps}>
			<Message className="w-full">
				<MessageContent className="whitespace-normal">
					<div className="space-y-3">
						{streamState?.content ? (
							<Response>{streamState.content}</Response>
						) : streamTools.length === 0 ? (
							<Shimmer as="span" className="text-sm">
								Thinking...
							</Shimmer>
						) : null}
						{streamState?.reasoning && (
							<Thinking>{streamState.reasoning}</Thinking>
						)}
						{streamTools.map((tool) => (
							<Tool
								key={tool.id}
								name={tool.name}
								args={tool.args}
								result={tool.result}
								status={tool.status}
								isError={tool.isError}
								subagentTitles={subagentTitles}
								subagentStatusOverrides={subagentStatusOverrides}
							/>
						))}
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
});
StreamingOutput.displayName = "StreamingOutput";

const StickyUserMessage: FC<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
}> = ({ message, parsed }) => {
	const sentinelRef = useRef<HTMLDivElement | null>(null);
	const [isStuck, setIsStuck] = useState(false);

	useEffect(() => {
		const el = sentinelRef.current;
		if (!el) return;
		const observer = new IntersectionObserver(
			([entry]) => setIsStuck(!entry.isIntersecting),
			{ threshold: 0 },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, []);

	// When stuck, visually clip via clip-path so layout height is
	// unchanged — no sentinel jitter, no feedback loop.

	return (
		<>
			<div ref={sentinelRef} className="pointer-events-none h-px" />
			<div
				className="sticky -top-2 z-10 pt-1 drop-shadow-xl"
				style={isStuck ? { clipPath: "inset(0 0 calc(100% - 5rem) 0 round 0 0 0.5rem 0.5rem)" } : undefined}
			>
				<ChatMessageItem message={message} parsed={parsed} />
			</div>
		</>
	);
};

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export const AgentDetail: FC = () => {
	const navigate = useNavigate();
	const { agentId } = useParams<{ agentId: string }>();
	const outletContext = useOutletContext<AgentsOutletContext | undefined>();
	const queryClient = useQueryClient();
	const [messagesById, setMessagesById] = useState<
		Map<number, TypesGen.ChatMessage>
	>(new Map());
	const [streamState, setStreamState] = useState<StreamState | null>(null);
	const [streamError, setStreamError] = useState<string | null>(null);
	const [queuedMessages, setQueuedMessages] = useState<
		readonly TypesGen.ChatQueuedMessage[]
	>([]);
	const [chatStatus, setChatStatus] = useState<TypesGen.ChatStatus | null>(
		null,
	);
	const [subagentStatusOverrides, setSubagentStatusOverrides] = useState<
		Map<string, TypesGen.ChatStatus>
	>(new Map());
	const [selectedModel, setSelectedModel] = useState("");
	const [contextCompressionThreshold, setContextCompressionThreshold] =
		useState(defaultContextCompressionThreshold);
	const chatErrorReasons = outletContext?.chatErrorReasons ?? {};
	const setChatErrorReason =
		outletContext?.setChatErrorReason ?? noopSetChatErrorReason;
	const clearChatErrorReason =
		outletContext?.clearChatErrorReason ?? noopClearChatErrorReason;
	const setRightPanelOpen =
		outletContext?.setRightPanelOpen ?? noopSetRightPanelOpen;
	const requestArchiveAgent =
		outletContext?.requestArchiveAgent ?? noopRequestArchiveAgent;
	const streamResetFrameRef = useRef<number | null>(null);
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatsQuery = useQuery(chats());
	const workspaceId = chatQuery.data?.chat.workspace_id;
	const workspaceAgentId = chatQuery.data?.chat.workspace_agent_id;
	const workspaceQuery = useQuery({
		queryKey: ["workspace", "agent-detail", workspaceId ?? ""],
		queryFn: () => API.getWorkspace(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const diffStatusQuery = useQuery({
		...chatDiffStatus(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatModelsQuery = useQuery(chatModels());
	const hasDiffStatus = Boolean(diffStatusQuery.data?.url);
	const [showDiffPanel, setShowDiffPanel] = useState(false);

	const workspaceAgent = useMemo(() => {
		const workspace = workspaceQuery.data;
		if (!workspace) {
			return undefined;
		}
		const agents = workspace.latest_build.resources.flatMap(
			(resource) => resource.agents ?? [],
		);
		if (agents.length === 0) {
			return undefined;
		}
		return (
			agents.find((agent) => agent.id === workspaceAgentId) ??
			agents[0]
		);
	}, [workspaceAgentId, workspaceQuery.data]);

	// Auto-open the diff panel when diff status becomes available.
	useEffect(() => {
		if (hasDiffStatus) {
			setShowDiffPanel(true);
		}
	}, [hasDiffStatus]);

	useEffect(() => {
		setRightPanelOpen(hasDiffStatus && showDiffPanel);
		return () => {
			setRightPanelOpen(false);
		};
	}, [hasDiffStatus, setRightPanelOpen, showDiffPanel]);

	const catalogModelOptions = useMemo(
		() => getModelOptionsFromCatalog(chatModelsQuery.data),
		[chatModelsQuery.data],
	);
	const modelOptions = catalogModelOptions;

	const sendMutation = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const interruptMutation = useMutation(
		interruptChat(queryClient, agentId ?? ""),
	);
	const deleteQueuedMutation = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const promoteQueuedMutation = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const updateSidebarChat = useCallback(
		(updater: (chat: TypesGen.Chat) => TypesGen.Chat) => {
			if (!agentId) {
				return;
			}

			queryClient.setQueryData<readonly TypesGen.Chat[] | undefined>(
				chatsKey,
				(currentChats) => {
					if (!currentChats) {
						return currentChats;
					}

					let didUpdate = false;
					const nextChats = currentChats.map((chat) => {
						if (chat.id !== agentId) {
							return chat;
						}
						didUpdate = true;
						return updater(chat);
					});

					return didUpdate ? nextChats : currentChats;
				},
			);
		},
		[agentId, queryClient],
	);
	const cancelScheduledStreamReset = useCallback(() => {
		if (streamResetFrameRef.current === null) {
			return;
		}
		window.cancelAnimationFrame(streamResetFrameRef.current);
		streamResetFrameRef.current = null;
	}, []);
	const scheduleStreamReset = useCallback(() => {
		cancelScheduledStreamReset();
		streamResetFrameRef.current = window.requestAnimationFrame(() => {
			setStreamState(null);
			streamResetFrameRef.current = null;
		});
	}, [cancelScheduledStreamReset]);

	useEffect(() => {
		if (!chatQuery.data) {
			setMessagesById(new Map());
			setChatStatus(null);
			setQueuedMessages([]);
			return;
		}
		setMessagesById(
			new Map(chatQuery.data.messages.map((message) => [message.id, message])),
		);
		setChatStatus(chatQuery.data.chat.status);
		setQueuedMessages(chatQuery.data.queued_messages ?? []);
	}, [chatQuery.data]);

	useEffect(() => {
		if (!chatQuery.data) {
			return;
		}
		setSelectedModel((current) => {
			if (current && modelOptions.some((model) => model.id === current)) {
				return current;
			}
			return resolveModelFromChatConfig(
				chatQuery.data.chat.model_config,
				modelOptions,
			);
		});
	}, [chatQuery.data, modelOptions]);

	useEffect(() => {
		if (!chatQuery.data) {
			return;
		}

		const configuredModel = resolveModelFromChatConfig(
			chatQuery.data.chat.model_config,
			modelOptions,
		);
		const fromChatConfig = resolveContextCompressionThresholdFromChatConfig(
			chatQuery.data.chat.model_config,
		);
		if (fromChatConfig !== null && selectedModel === configuredModel) {
			setContextCompressionThreshold(fromChatConfig);
			return;
		}

		if (typeof window === "undefined") {
			return;
		}
		const storedThreshold = localStorage.getItem(
			contextCompressionThresholdStorageKey(selectedModel),
		);
		setContextCompressionThreshold(
			storedThreshold ?? defaultContextCompressionThreshold,
		);
	}, [chatQuery.data, modelOptions, selectedModel]);

	useEffect(() => {
		if (!agentId) {
			return;
		}

		cancelScheduledStreamReset();
		setStreamState(null);
		setStreamError(null);
		setSubagentStatusOverrides(new Map());

		const socket = watchChat(agentId);
		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
		) => {
			if (payload.parseError || !payload.parsedMessage) {
				setStreamError("Failed to parse chat stream update.");
				return;
			}
			if (payload.parsedMessage.type !== "data") {
				return;
			}

			const streamEvent = payload.parsedMessage
				.data as TypesGen.ChatStreamEvent & Record<string, unknown>;
			if (!streamEvent?.type) {
				return;
			}

			switch (streamEvent.type) {
				case "message": {
					const message = streamEvent.message;
					if (!message) {
						return;
					}
					setMessagesById((prev) => {
						const next = new Map(prev);
						next.set(message.id, message);
						return next;
					});
					scheduleStreamReset();
					updateSidebarChat((chat) => ({
						...chat,
						updated_at: message.created_at ?? new Date().toISOString(),
					}));
					void queryClient.invalidateQueries({ queryKey: chatsKey });
					return;
				}
				case "message_part": {
					const messagePart = streamEvent.message_part;
					const part = messagePart?.part;
					if (!part) {
						return;
					}
					cancelScheduledStreamReset();

					// Wrap stream state updates in startTransition so React
					// treats them as low-priority — keeping user interactions
					// (typing, clicking) responsive during rapid streaming.
					switch (normalizeBlockType(part.type)) {
						case "text":
							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									return {
										...nextState,
										content: `${nextState.content}${asString(part.text)}`,
									};
								});
							});
							return;
						case "reasoning":
						case "thinking":
							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									return {
										...nextState,
										reasoning: `${nextState.reasoning}${asString(part.text)}`,
									};
								});
							});
							return;
						case "tool-call":
						case "toolcall": {
							const toolName = asString(part.tool_name);

							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									const existingByName = Object.values(
										nextState.toolCalls,
									).find((call) => call.name === toolName);
									const toolCallID =
										asString(part.tool_call_id) ||
										existingByName?.id ||
										`tool-call-${Object.keys(nextState.toolCalls).length + 1}`;
									const existing = nextState.toolCalls[toolCallID];
									const nextArgs = mergeStreamPayload(
										existing?.args,
										existing?.argsRaw,
										part.args,
										part.args_delta,
									);

									return {
										...nextState,
										toolCalls: {
											...nextState.toolCalls,
											[toolCallID]: {
												id: toolCallID,
												name: toolName || existing?.name || "Tool",
												args: nextArgs.value,
												argsRaw: nextArgs.rawText,
											},
										},
									};
								});
							});
							return;
						}
						case "tool-result":
						case "toolresult": {
							const toolName = asString(part.tool_name);

							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									const existingByName = Object.values(
										nextState.toolResults,
									).find((result) => result.name === toolName);
									const existingCallByName = Object.values(
										nextState.toolCalls,
									).find((call) => call.name === toolName);
									const toolCallID =
										asString(part.tool_call_id) ||
										existingByName?.id ||
										existingCallByName?.id ||
										`tool-result-${Object.keys(nextState.toolResults).length + 1}`;
									const existing = nextState.toolResults[toolCallID];
									const nextResult = mergeStreamPayload(
										existing?.result,
										existing?.resultRaw,
										part.result,
										part.result_delta,
									);
									const nextToolName = toolName || existing?.name || "Tool";
									const nextIsError =
										existing?.isError ||
										parseToolResultIsError(
											nextToolName,
											part,
											nextResult.value,
										);

									return {
										...nextState,
										toolResults: {
											...nextState.toolResults,
											[toolCallID]: {
												id: toolCallID,
												name: nextToolName,
												result: nextResult.value,
												resultRaw: nextResult.rawText,
												isError: nextIsError,
											},
										},
									};
								});
							});
							return;
						}
						default:
							return;
					}
				}
				case "queue_update": {
					const queuedMsgs = streamEvent.queued_messages;
					setQueuedMessages(queuedMsgs ?? []);
					return;
				}
				case "status": {
					const status = asRecord(streamEvent.status);
					const nextStatus = asString(status?.status) as TypesGen.ChatStatus;
					if (!nextStatus) {
						return;
					}

					const eventChatID = asString(streamEvent.chat_id);
					if (eventChatID && eventChatID !== agentId) {
						setSubagentStatusOverrides((prev) => {
							if (prev.get(eventChatID) === nextStatus) {
								return prev;
							}
							const next = new Map(prev);
							next.set(eventChatID, nextStatus);
							return next;
						});
						return;
					}

					setChatStatus(nextStatus);
					if (agentId && nextStatus !== "error") {
						clearChatErrorReason(agentId);
					}
					updateSidebarChat((chat) => ({
						...chat,
						status: nextStatus,
						updated_at: new Date().toISOString(),
					}));
					// Always refresh diff queries on any status event
					// because the background refresh may have discovered
					// a new PR or updated diff contents.
					if (agentId) {
						void Promise.all([
							queryClient.invalidateQueries({
								queryKey: chatDiffStatusKey(agentId),
							}),
							queryClient.invalidateQueries({
								queryKey: chatDiffContentsKey(agentId),
							}),
						]);
					}
					const shouldRefreshQueries =
						nextStatus === "completed" ||
						nextStatus === "error" ||
						nextStatus === "paused" ||
						nextStatus === "waiting";
					if (shouldRefreshQueries) {
						void queryClient.invalidateQueries({ queryKey: chatsKey });
					}
					return;
				}
				case "error": {
					const error = asRecord(streamEvent.error);
					const reason =
						asString(error?.message).trim() || "Chat processing failed.";
					setChatStatus("error");
					setStreamError(reason);
					if (agentId) {
						setChatErrorReason(agentId, reason);
					}
					updateSidebarChat((chat) => ({
						...chat,
						status: "error",
						updated_at: new Date().toISOString(),
					}));
					void queryClient.invalidateQueries({ queryKey: chatsKey });
					return;
				}
				default:
					break;
			}
		};

		const handleError = () => {
			setStreamError((current) => current ?? "Chat stream disconnected.");
			void queryClient.invalidateQueries({ queryKey: chatsKey });
		};

		socket.addEventListener("message", handleMessage);
		socket.addEventListener("error", handleError);

		return () => {
			socket.removeEventListener("message", handleMessage);
			socket.removeEventListener("error", handleError);
			socket.close();
			cancelScheduledStreamReset();
		};
	}, [
		agentId,
		cancelScheduledStreamReset,
		clearChatErrorReason,
		queryClient,
		scheduleStreamReset,
		setChatErrorReason,
		updateSidebarChat,
	]);

	const messages = useMemo(() => {
		const list = Array.from(messagesById.values());
		list.sort(
			(a, b) =>
				new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
		);
		return list;
	}, [messagesById]);
	const latestContextUsageRaw = useMemo(
		() => getLatestContextUsage(messages),
		[messages],
	);
	// Stabilize the reference so AgentChatInput doesn't re-render
	// when the token counts haven't actually changed.
	const contextUsageRef = useRef<AgentContextUsage | null>(null);
	const latestContextUsage = useMemo(() => {
		const prev = contextUsageRef.current;
		if (
			prev !== null &&
			latestContextUsageRaw !== null &&
			prev.usedTokens === latestContextUsageRaw.usedTokens &&
			prev.contextLimitTokens === latestContextUsageRaw.contextLimitTokens &&
			prev.inputTokens === latestContextUsageRaw.inputTokens &&
			prev.outputTokens === latestContextUsageRaw.outputTokens &&
			prev.cacheReadTokens === latestContextUsageRaw.cacheReadTokens &&
			prev.cacheCreationTokens === latestContextUsageRaw.cacheCreationTokens &&
			prev.reasoningTokens === latestContextUsageRaw.reasoningTokens
		) {
			return prev;
		}
		contextUsageRef.current = latestContextUsageRaw;
		return latestContextUsageRaw;
	}, [latestContextUsageRaw]);

	const isStreaming =
		Boolean(streamState) ||
		chatStatus === "running" ||
		chatStatus === "pending";
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(
		chatModelsQuery.data,
	);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		chatModelsQuery.isLoading,
		hasConfiguredModels,
	);
	const modelCatalogStatusMessage = getModelCatalogStatusMessage(
		chatModelsQuery.data,
		modelOptions,
		chatModelsQuery.isLoading,
		Boolean(chatModelsQuery.error),
	);
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";
	const isSubmissionPending =
		sendMutation.isPending || interruptMutation.isPending;
	const isInputDisabled = isSubmissionPending || !hasModelOptions;
	const handleContextCompressionThresholdChange = useCallback(
		(value: string) => {
			setContextCompressionThreshold(value);
			if (typeof window !== "undefined") {
				localStorage.setItem(
					contextCompressionThresholdStorageKey(selectedModel),
					value,
				);
			}
		},
		[selectedModel],
	);

	// Stable callback refs — the actual implementation is updated on
	// every render, but the reference passed to ChatInput never changes.
	// This prevents ChatInput from re-rendering when unrelated parent
	// state (streamState, messagesById) changes.
	const handleSendRef = useRef<(message: string) => Promise<void>>(
		async () => {},
	);
	handleSendRef.current = async (message: string) => {
		if (
			!message.trim() ||
			isSubmissionPending ||
			!agentId ||
			!hasModelOptions
		) {
			return;
		}
		const parsedCompressionThreshold = parseContextCompressionThreshold(
			contextCompressionThreshold,
		);
		const request: CreateChatMessagePayload = {
			role: "user",
			content: JSON.parse(JSON.stringify(message)),
			model: selectedModel || undefined,
			context_compression_threshold: parsedCompressionThreshold,
		};
		clearChatErrorReason(agentId);
		setStreamError(null);
		// Scroll to bottom when sending a new message. With
		// flex-col-reverse the bottom is scrollTop = 0.
		if (scrollContainerRef.current) {
			scrollContainerRef.current.scrollTop = 0;
		}
		await sendMutation.mutateAsync(request);
	};

	const handleInterruptRef = useRef<() => void>(() => {});
	handleInterruptRef.current = () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		void interruptMutation.mutateAsync();
	};

	// Stable wrappers that never change identity.
	const stableOnSend = useCallback(
		(message: string) => handleSendRef.current(message),
		[],
	);
	const stableOnInterrupt = useCallback(() => handleInterruptRef.current(), []);

	const streamTools = useMemo((): MergedTool[] => {
		if (!streamState) {
			return [];
		}
		const calls = Object.values(streamState.toolCalls);
		const seen = new Set<string>();
		const merged: MergedTool[] = [];

		for (const call of calls) {
			seen.add(call.id);
			const result = streamState.toolResults[call.id];
			merged.push({
				id: call.id,
				name: call.name,
				args: call.args,
				result: result?.result,
				isError: result?.isError ?? false,
				status: result ? (result.isError ? "error" : "completed") : "running",
			});
		}

		for (const result of Object.values(streamState.toolResults)) {
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
	}, [streamState]);

	const visibleMessages = useMemo(
		() => messages.filter((message) => !message.hidden),
		[messages],
	);

	// Windowed rendering: only mount the most recent N messages
	// and progressively reveal older ones as the user scrolls up.
	const MESSAGES_PAGE_SIZE = 50;
	const [renderedMessageCount, setRenderedMessageCount] =
		useState(MESSAGES_PAGE_SIZE);

	// Reset the window when switching chats.
	useEffect(() => {
		setRenderedMessageCount(MESSAGES_PAGE_SIZE);
	}, [agentId]);

	const hasMoreMessages = renderedMessageCount < visibleMessages.length;
	const windowedMessages = useMemo(() => {
		if (renderedMessageCount >= visibleMessages.length) {
			return visibleMessages;
		}
		return visibleMessages.slice(
			visibleMessages.length - renderedMessageCount,
		);
	}, [visibleMessages, renderedMessageCount]);

	// Sentinel ref: when it scrolls into view, load more messages.
	const loadMoreSentinelRef = useRef<HTMLDivElement | null>(null);
	useEffect(() => {
		const node = loadMoreSentinelRef.current;
		if (!node || !hasMoreMessages) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					setRenderedMessageCount((prev) =>
						prev + MESSAGES_PAGE_SIZE,
					);
				}
			},
			{ rootMargin: "200px" },
		);
		observer.observe(node);
		return () => observer.disconnect();
	}, [hasMoreMessages, windowedMessages]);

	// Each message is parsed once; the global tool-result map and
	// final merged content are derived from the same parse.
	const parsedMessages = useMemo(() => {
		// Step 1: parse each message once.
		const rawParsed = windowedMessages.map((message) => ({
			message,
			parsed:
				Array.isArray(message.parts) && message.parts.length > 0
					? parseMessageContent(message.parts)
					: parseMessageContent(message.content),
		}));

		// Step 2: build the global tool-result map from the already-parsed data.
		const globalToolResults = new Map<string, ParsedToolResult>();
		for (const { parsed } of rawParsed) {
			for (const result of parsed.toolResults) {
				globalToolResults.set(result.id, result);
			}
		}

		// Step 3: merge tool calls with their results.
		for (const { parsed } of rawParsed) {
			const resultById = new Map<string, ParsedToolResult>();
			for (const r of parsed.toolResults) {
				resultById.set(r.id, r);
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
	}, [windowedMessages]);

	// Build a lookup of sub-agent chat_id → title from spawn tool
	// results so await/message tools can show the title while still
	// in progress (before their own result arrives with the title).
	const subagentTitles = useMemo(() => {
		const map = new Map<string, string>();
		for (const { parsed } of parsedMessages) {
			for (const tool of parsed.tools) {
				if (tool.name !== "subagent") continue;
				const rec = asRecord(tool.result);
				if (!rec) continue;
				const chatId = asString(rec.chat_id);
				const title = asString(rec.title);
				if (chatId && title) {
					map.set(chatId, title);
				}
			}
		}
		return map;
	}, [parsedMessages]);

	// Group parsed messages into sections for rendering.
	const parsedSections = useMemo(() => {
		const sections: Array<{
			userEntry: (typeof parsedMessages)[number] | null;
			entries: typeof parsedMessages;
		}> = [];

		for (const entry of parsedMessages) {
			if (entry.message.role === "user") {
				sections.push({ userEntry: entry, entries: [entry] });
			} else if (sections.length === 0) {
				sections.push({ userEntry: null, entries: [entry] });
			} else {
				sections[sections.length - 1].entries.push(entry);
			}
		}

		return sections;
	}, [parsedMessages]);
	const persistedErrorReason = agentId ? chatErrorReasons[agentId] : undefined;
	const detailErrorMessage =
		(chatStatus === "error" ? persistedErrorReason : undefined) || streamError;
	const hasStreamOutput =
		chatStatus === "running" ||
		chatStatus === "pending" ||
		(!!streamState &&
			(streamState.content !== "" ||
				streamState.reasoning !== "" ||
				streamTools.length > 0));

	const topBarTitleRef = outletContext?.topBarTitleRef;
	const topBarActionsRef = outletContext?.topBarActionsRef;
	const rightPanelRef = outletContext?.rightPanelRef;
	const chatTitle = chatQuery.data?.chat.title;
	const parentChatID = getParentChatID(chatQuery.data?.chat);
	const parentChat = parentChatID
		? chatsQuery.data?.find((chat) => chat.id === parentChatID)
		: undefined;
	const workspace = workspaceQuery.data;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const shouldShowDiffPanel = hasDiffStatus && showDiffPanel;

	const handleOpenInEditor = useCallback(
		async (editor: "cursor" | "vscode") => {
			if (!workspace || !workspaceAgent) {
				return;
			}

			try {
				const { key } = await API.getApiKey();
				const vscodeHref = getVSCodeHref("vscode", {
					owner: workspace.owner_name,
					workspace: workspace.name,
					token: key,
					agent: workspaceAgent.name,
					folder: workspaceAgent.expanded_directory,
				});

				if (editor === "cursor") {
					const cursorApp = workspaceAgent.apps.find((app) => {
						const name = (app.display_name ?? app.slug).toLowerCase();
						return app.slug.toLowerCase() === "cursor" || name === "cursor";
					});
					if (cursorApp?.external && cursorApp.url) {
						const href = cursorApp.url.includes(SESSION_TOKEN_PLACEHOLDER)
							? cursorApp.url.replaceAll(SESSION_TOKEN_PLACEHOLDER, key)
							: cursorApp.url;
						window.location.assign(href);
						return;
					}
					window.location.assign(vscodeHref.replace(/^vscode:/, "cursor:"));
					return;
				}

				window.location.assign(vscodeHref);
			} catch {
				displayError(
					editor === "cursor"
						? "Failed to open in Cursor."
						: "Failed to open in VS Code.",
				);
			}
		},
		[workspace, workspaceAgent],
	);

	const handleViewWorkspace = useCallback(() => {
		if (!workspaceRoute) {
			return;
		}
		navigate(workspaceRoute);
	}, [navigate, workspaceRoute]);

	const handleArchiveAgentAction = useCallback(() => {
		if (!agentId) {
			return;
		}
		requestArchiveAgent(agentId);
	}, [agentId, requestArchiveAgent]);

	if (chatQuery.isLoading) {
		return (
			<div className="mx-auto w-full max-w-3xl space-y-6 py-6">
				{/* User message skeleton */}
				<div className="flex justify-end">
					<Skeleton className="h-10 w-2/3 rounded-xl" />
				</div>
				{/* Assistant response skeleton */}
				<div className="space-y-3">
					<Skeleton className="h-4 w-full" />
					<Skeleton className="h-4 w-5/6" />
					<Skeleton className="h-4 w-4/6" />
					<Skeleton className="h-4 w-full" />
					<Skeleton className="h-4 w-3/5" />
				</div>
			</div>
		);
	}

	if (!chatQuery.data || !agentId) {
		return (
			<div className="flex flex-1 items-center justify-center text-content-secondary">
				Chat not found
			</div>
		);
	}

	const chatContent = (
		<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{chatTitle &&
				topBarTitleRef?.current &&
				createPortal(
					<div className="flex min-w-0 items-center gap-1.5">
						{parentChat && (
							<>
								<Button
									size="sm"
									variant="subtle"
									className="h-auto max-w-[16rem] rounded-sm px-1 py-0.5 text-xs text-content-secondary shadow-none hover:bg-transparent hover:text-content-primary"
									onClick={() => navigate(`/agents/${parentChat.id}`)}
								>
									<span className="truncate">{parentChat.title}</span>
								</Button>
								<ChevronRightIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary/70" />
							</>
						)}
						<span className="truncate text-sm text-content-primary">
							{chatTitle}
						</span>
					</div>,
					topBarTitleRef.current,
				)}
			{hasDiffStatus &&
				diffStatusQuery.data &&
				topBarActionsRef?.current &&
				createPortal(
					<DiffStatsBadge
						status={diffStatusQuery.data}
						isOpen={showDiffPanel}
						onToggle={() => setShowDiffPanel((prev) => !prev)}
					/>,
					topBarActionsRef.current,
				)}
			{topBarActionsRef?.current &&
				createPortal(
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								className="h-7 w-7 text-content-secondary hover:text-content-primary"
								aria-label="Open agent actions"
							>
								<EllipsisIcon className="h-4 w-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem
								disabled={!canOpenEditors}
								onSelect={() => {
									void handleOpenInEditor("cursor");
								}}
							>
								<ExternalLinkIcon className="h-3.5 w-3.5" />
								Open in Cursor
							</DropdownMenuItem>
							<DropdownMenuItem
								disabled={!canOpenEditors}
								onSelect={() => {
									void handleOpenInEditor("vscode");
								}}
							>
								<ExternalLinkIcon className="h-3.5 w-3.5" />
								Open in VS Code
							</DropdownMenuItem>
							<DropdownMenuItem
								disabled={!canOpenWorkspace}
								onSelect={handleViewWorkspace}
							>
								<MonitorIcon className="h-3.5 w-3.5" />
								View Workspace
							</DropdownMenuItem>
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onSelect={handleArchiveAgentAction}
							>
								<ArchiveIcon className="h-3.5 w-3.5" />
								Archive Agent
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>,
					topBarActionsRef.current,
				)}
			{shouldShowDiffPanel &&
				rightPanelRef?.current &&
				createPortal(<FilesChangedPanel chatId={agentId} />, rightPanelRef.current)}
			<div
				ref={scrollContainerRef}
				className="flex h-full flex-col-reverse overflow-y-auto [scrollbar-width:thin] [scrollbar-color:hsl(240_5%_26%)_transparent]"
			>
				<div>
					<div className="mx-auto w-full max-w-3xl py-6">
						{visibleMessages.length === 0 && !hasStreamOutput ? (
							<div className="py-12 text-center text-content-secondary">
								<p className="text-sm">Start a conversation with your agent.</p>
							</div>
						) : (
							<div className="flex flex-col">
								{hasMoreMessages && (
									<div
										ref={loadMoreSentinelRef}
										className="flex items-center justify-center py-4 text-xs text-content-secondary"
									>
										Loading earlier messages…
									</div>
								)}
								{parsedSections.map((section, sectionIdx) => (
									<div
										key={
											section.userEntry?.message.id ?? `section-${sectionIdx}`
										}
									>
										<div className="flex flex-col gap-3">
											{section.entries.map(({ message, parsed }) =>
												message.role === "user" ? (
													<StickyUserMessage
														key={message.id}
														message={message}
														parsed={parsed}
													/>
												) : (
													<ChatMessageItem
														key={message.id}
														message={message}
														parsed={parsed}
													/>
												),
											)}
										</div>
									</div>
								))}

								{hasStreamOutput && (
									<div className="mt-5">
										<StreamingOutput
											streamState={streamState}
											streamTools={streamTools}
											subagentTitles={subagentTitles}
											subagentStatusOverrides={subagentStatusOverrides}
										/>
									</div>
								)}
							</div>
						)}

						{detailErrorMessage && (
							<div className="mt-4 rounded-md border border-border-destructive bg-surface-red px-3 py-2 text-xs text-content-destructive">
								{detailErrorMessage}
							</div>
						)}
					</div>

					{queuedMessages.length > 0 && (
						<QueuedMessagesList
							messages={queuedMessages}
							onDelete={(id) => deleteQueuedMutation.mutate(id)}
							onPromote={(id) => promoteQueuedMutation.mutate(id)}
						/>
					)}
					<AgentChatInput
						onSend={stableOnSend}
						isDisabled={isInputDisabled}
						isLoading={sendMutation.isPending}
						isStreaming={isStreaming}
						onInterrupt={stableOnInterrupt}
						isInterruptPending={interruptMutation.isPending}
						hasQueuedMessages={queuedMessages.length > 0}
						contextUsage={latestContextUsage}
						hasModelOptions={hasModelOptions}
						selectedModel={selectedModel}
						onModelChange={setSelectedModel}
						modelOptions={modelOptions}
						modelSelectorPlaceholder={modelSelectorPlaceholder}
						inputStatusText={inputStatusText}
						modelCatalogStatusMessage={modelCatalogStatusMessage}
						contextCompressionThreshold={contextCompressionThreshold}
						onContextCompressionThresholdChange={
							handleContextCompressionThresholdChange
						}
						sticky
					/>
				</div>
			</div>
		</div>
	);

	return chatContent;
};

export default AgentDetail;
