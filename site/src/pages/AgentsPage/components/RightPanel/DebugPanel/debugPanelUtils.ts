export interface NormalizedAttempt {
	attempt_number: number;
	status: string;
	method?: string;
	url?: string;
	path?: string;
	raw_request?: Record<string, unknown>;
	raw_response?: Record<string, unknown>;
	error?: Record<string, string> | string;
	duration_ms?: number;
	started_at?: string;
	finished_at?: string;
	response_status?: number;
}

const RUN_KIND_LABELS: Record<string, string> = {
	chat_turn: "Chat Turn",
	title_generation: "Title Generation",
	compaction: "Compaction",
	quickgen: "Quick Gen",
	quick_gen: "Quick Gen",
	llm_call: "LLM Call",
	post_process: "Post-process",
	tool_call: "Tool Call",
};

export const SUCCESS_STATUSES = new Set([
	"completed",
	"success",
	"succeeded",
	"ok",
]);
const WARNING_STATUSES = new Set([
	"pending",
	"queued",
	"retrying",
	"scheduled",
]);
const INFO_STATUSES = new Set([
	"running",
	"in_progress",
	"processing",
	"started",
]);
export const ERROR_STATUSES = new Set([
	"failed",
	"error",
	"errored",
	"interrupted",
	"cancelled",
	"canceled",
]);

const isRecord = (value: unknown): value is Record<string, unknown> => {
	return typeof value === "object" && value !== null && !Array.isArray(value);
};

const toFiniteNumber = (value: unknown): number | undefined => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}
	if (typeof value !== "string" || value.trim() === "") {
		return undefined;
	}
	const parsed = Number(value);
	return Number.isFinite(parsed) ? parsed : undefined;
};

const toOptionalString = (value: unknown): string | undefined => {
	if (typeof value === "string") {
		return value;
	}
	if (typeof value === "number" || typeof value === "boolean") {
		return String(value);
	}
	return undefined;
};

const toStringRecord = (value: unknown): Record<string, string> | undefined => {
	if (!isRecord(value)) {
		return undefined;
	}

	const result: Record<string, string> = {};
	for (const [key, entry] of Object.entries(value)) {
		const normalized = toOptionalString(entry);
		if (normalized !== undefined) {
			result[key] = normalized;
		}
	}

	return Object.keys(result).length > 0 ? result : undefined;
};

const humanizeToken = (value: string): string => {
	return value
		.replace(/_/g, " ")
		.replace(/\b\w/g, (match) => match.toUpperCase());
};

export const safeJsonStringify = (value: unknown): string => {
	if (typeof value === "string") {
		return value;
	}
	if (value === undefined) {
		// JSON.stringify(undefined) returns undefined (not a string), which
		// would break the string return contract and surface "undefined" in
		// the debug panel as literal text from String(undefined).
		return "";
	}

	try {
		const serialized = JSON.stringify(value, null, 2);
		// JSON.stringify returns undefined for values like functions or
		// symbols. Fall back to String() so the caller always gets a string.
		return serialized ?? String(value);
	} catch {
		return String(value);
	}
};

const normalizeAttemptEntry = (
	value: unknown,
	fallbackAttemptNumber: number,
): NormalizedAttempt | null => {
	const candidate = typeof value === "string" ? tryParseJson(value) : value;
	if (!isRecord(candidate)) {
		return null;
	}

	// Support both old shape (attempt_number) and new backend shape (number).
	const attemptNumber =
		toFiniteNumber(candidate.attempt_number) ??
		toFiniteNumber(candidate.number) ??
		fallbackAttemptNumber;
	const status = toOptionalString(candidate.status) ?? "unknown";
	const method = toOptionalString(candidate.method);
	const url = toOptionalString(candidate.url);
	const path = toOptionalString(candidate.path);

	// Build raw_request from backend fields if direct raw_request is absent.
	let rawRequest = toRecord(candidate.raw_request);
	if (!rawRequest) {
		const reqParts: Record<string, unknown> = {};
		if (method) {
			reqParts.method = method;
		}
		if (url) {
			reqParts.url = url;
		}
		if (path) {
			reqParts.path = path;
		}
		const reqHeaders = candidate.request_headers;
		if (isRecord(reqHeaders) && Object.keys(reqHeaders).length > 0) {
			reqParts.headers = reqHeaders;
		}
		const reqBody = candidate.request_body;
		if (reqBody && typeof reqBody === "string") {
			const parsed =
				tryParseJson(reqBody) ??
				tryDecodeBase64Json(reqBody) ??
				tryDecodeBase64(reqBody);
			reqParts.body = parsed !== undefined ? parsed : reqBody;
		} else if (reqBody) {
			reqParts.body = reqBody;
		}
		if (Object.keys(reqParts).length > 0) {
			rawRequest = reqParts;
		}
	}

	// Build raw_response from backend fields if direct raw_response is absent.
	let rawResponse = toRecord(candidate.raw_response);
	if (!rawResponse) {
		const resParts: Record<string, unknown> = {};
		const respStatus = toFiniteNumber(candidate.response_status);
		if (respStatus !== undefined) {
			resParts.status = respStatus;
		}
		const resHeaders = candidate.response_headers;
		if (isRecord(resHeaders) && Object.keys(resHeaders).length > 0) {
			resParts.headers = resHeaders;
		}
		const resBody = candidate.response_body;
		if (resBody && typeof resBody === "string") {
			const parsed =
				tryParseJson(resBody) ??
				tryDecodeBase64Json(resBody) ??
				tryDecodeBase64(resBody);
			resParts.body = parsed !== undefined ? parsed : resBody;
		} else if (resBody) {
			resParts.body = resBody;
		}
		if (Object.keys(resParts).length > 0) {
			rawResponse = resParts;
		}
	}

	// Error: support both string and object shapes.
	let error: Record<string, string> | string | undefined;
	const rawError = candidate.error;
	if (typeof rawError === "string" && rawError.length > 0) {
		error = rawError;
	} else {
		error = toStringRecord(rawError);
	}

	return {
		attempt_number: attemptNumber,
		status,
		method,
		url,
		path,
		raw_request: rawRequest,
		raw_response: rawResponse,
		error,
		duration_ms: toFiniteNumber(candidate.duration_ms),
		started_at: toOptionalString(candidate.started_at),
		finished_at: toOptionalString(candidate.finished_at),
		response_status: toFiniteNumber(candidate.response_status),
	};
};

const toRecord = (value: unknown): Record<string, unknown> | undefined => {
	if (!isRecord(value)) {
		return undefined;
	}
	return Object.keys(value).length > 0 ? value : undefined;
};

const normalizeAttemptList = (
	value: readonly unknown[],
): NormalizedAttempt[] => {
	const parsed: NormalizedAttempt[] = [];

	for (const [index, entry] of value.entries()) {
		const normalized = normalizeAttemptEntry(entry, index + 1);
		if (normalized) {
			parsed.push(normalized);
		}
	}

	return parsed.toSorted(
		(left, right) => left.attempt_number - right.attempt_number,
	);
};

const tryParseJson = (value: string): unknown => {
	try {
		return JSON.parse(value);
	} catch {
		return undefined;
	}
};
/**
 * Go's encoding/json marshals []byte fields as base64. Attempt to
 * decode a base64 string and then parse the result as JSON. Returns
 * the parsed object on success, undefined otherwise.
 */
const tryDecodeBase64Json = (value: string): unknown => {
	try {
		const binary = atob(value);
		const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
		const decoded = new TextDecoder().decode(bytes);
		return JSON.parse(decoded);
	} catch {
		return undefined;
	}
};

// Matches canonical (Go-emitted) base64: the full base64 alphabet with
// optional trailing `=` padding. `atob` is lenient and will happily decode
// strings like "test" into garbage bytes, so require a strict format before
// attempting a decode.
const STRICT_BASE64 = /^[A-Za-z0-9+/]+={0,2}$/;

/**
 * Try to decode a base64 string to plain text.  Go's encoding/json
 * marshals []byte as base64, so raw body payloads may appear as
 * gibberish in the debug panel unless we decode them.  Returns the
 * decoded UTF-8 string on success, undefined otherwise.
 *
 * Gated behind a strict base64 format check and a fatal UTF-8 decode
 * so plain-text payloads that happen to be valid base64 alphabet
 * (e.g. "test") are preserved as-is instead of being turned into
 * mojibake.
 */
const tryDecodeBase64 = (value: string): string | undefined => {
	if (value.length === 0 || value.length % 4 !== 0) {
		return undefined;
	}
	if (!STRICT_BASE64.test(value)) {
		return undefined;
	}
	try {
		const binary = atob(value);
		const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
		return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
	} catch {
		return undefined;
	}
};

export const getRunKindLabel = (kind: string): string => {
	if (!kind.trim()) {
		return "Unknown";
	}
	return RUN_KIND_LABELS[kind] ?? humanizeToken(kind);
};

export const getStatusBadgeVariant = (status: string) => {
	const normalizedStatus = status.trim().toLowerCase();
	if (SUCCESS_STATUSES.has(normalizedStatus)) {
		return "green";
	}
	if (ERROR_STATUSES.has(normalizedStatus)) {
		return "destructive";
	}
	if (INFO_STATUSES.has(normalizedStatus)) {
		return "info";
	}
	if (WARNING_STATUSES.has(normalizedStatus)) {
		return "warning";
	}
	return "default";
};

export const normalizeAttempts = (
	attempts: unknown,
): { parsed: NormalizedAttempt[]; rawFallback?: string } => {
	const source = attempts;

	if (Array.isArray(source)) {
		const parsed = normalizeAttemptList(source);
		if (parsed.length > 0) {
			return { parsed };
		}
		return source.length === 0
			? { parsed: [] }
			: { parsed: [], rawFallback: safeJsonStringify(source) };
	}

	if (typeof source === "string") {
		const parsedJson = tryParseJson(source);
		if (Array.isArray(parsedJson)) {
			const parsed = normalizeAttemptList(parsedJson);
			if (parsed.length > 0) {
				return { parsed };
			}
			return parsedJson.length === 0
				? { parsed: [] }
				: { parsed: [], rawFallback: source };
		}
		// Handle object-shaped JSON strings (e.g., a dict of attempts
		// keyed by index) by treating them as a single-element record.
		if (isRecord(parsedJson)) {
			const parsed = normalizeAttemptList(Object.values(parsedJson));
			if (parsed.length > 0) {
				return { parsed };
			}
		}
		return { parsed: [], rawFallback: source };
	}

	if (isRecord(source)) {
		const parsed: NormalizedAttempt[] = [];
		for (const value of Object.values(source)) {
			if (typeof value === "string") {
				const parsedValue = tryParseJson(value);
				if (Array.isArray(parsedValue)) {
					parsed.push(...normalizeAttemptList(parsedValue));
					continue;
				}
				const normalized = normalizeAttemptEntry(value, parsed.length + 1);
				if (normalized) {
					parsed.push(normalized);
				}
				continue;
			}

			const normalized = normalizeAttemptEntry(value, parsed.length + 1);
			if (normalized) {
				parsed.push(normalized);
			}
		}

		if (parsed.length > 0) {
			return {
				parsed: parsed.toSorted(
					(left, right) => left.attempt_number - right.attempt_number,
				),
			};
		}

		return Object.keys(source).length === 0
			? { parsed: [] }
			: { parsed: [], rawFallback: safeJsonStringify(source) };
	}

	return { parsed: [], rawFallback: safeJsonStringify(source) };
};

export const computeDurationMs = (
	startedAt: string,
	finishedAt?: string,
): number | null => {
	const startedAtMs = Date.parse(startedAt);
	if (Number.isNaN(startedAtMs)) {
		return null;
	}

	const finishedAtMs = finishedAt ? Date.parse(finishedAt) : Date.now();
	if (Number.isNaN(finishedAtMs)) {
		return null;
	}

	return Math.max(0, finishedAtMs - startedAtMs);
};

export const compactDuration = (ms: number): string => {
	if (ms < 1000) {
		return `${Math.round(ms)}ms`;
	}
	if (ms < 60000) {
		return `${(ms / 1000).toFixed(1)}s`;
	}

	const totalSeconds = Math.round(ms / 1000);
	const mins = Math.floor(totalSeconds / 60);
	const secs = totalSeconds % 60;
	return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`;
};

// ---------------------------------------------------------------------------
// View-model types for coerced debug payloads.
// ---------------------------------------------------------------------------

interface RunSummaryViewModel {
	primaryLabel: string;
	endpointLabel: string | undefined;
	model: string | undefined;
	provider: string | undefined;
	stepCount: number | undefined;
	totalInputTokens: number | undefined;
	totalOutputTokens: number | undefined;
	warnings: string[];
}

export interface MessagePart {
	role: string;
	content: string;
	toolCallId?: string;
	toolName?: string;
	kind?: "tool-call" | "tool-result";
	arguments?: string;
	result?: string;
}

interface ToolDef {
	name: string;
	description?: string;
	inputSchema?: string;
}

interface ToolCallPart {
	id?: string;
	name: string;
	arguments?: string;
}

interface StepRequestViewModel {
	model?: string;
	messages: MessagePart[];
	tools: ToolDef[];
	options: Record<string, unknown>;
	policy: Record<string, unknown>;
}

interface StepResponseViewModel {
	content: string;
	toolCalls: ToolCallPart[];
	finishReason?: string;
	usage: Record<string, number>;
	warnings: string[];
	model?: string;
}

// ---------------------------------------------------------------------------
// Internal helpers for coercion.
// ---------------------------------------------------------------------------

/** Look up the first defined value among several possible field names. */
const pickField = (
	obj: Record<string, unknown>,
	...names: string[]
): unknown => {
	for (const name of names) {
		if (name in obj && obj[name] !== undefined) {
			return obj[name];
		}
	}
	return undefined;
};

/**
 * If `value` is a JSON string that wraps an object or array, parse and
 * return the result.  All other values pass through untouched.
 */
const deepParse = (value: unknown): unknown => {
	if (typeof value !== "string") {
		return value;
	}
	const trimmed = value.trim();
	if (
		(trimmed.startsWith("{") && trimmed.endsWith("}")) ||
		(trimmed.startsWith("[") && trimmed.endsWith("]"))
	) {
		const parsed = tryParseJson(trimmed);
		return parsed !== undefined ? parsed : value;
	}
	return value;
};

const toCodeContent = (value: unknown): string | undefined => {
	const parsed = deepParse(value);
	if (parsed === undefined) {
		return undefined;
	}
	if (typeof parsed === "string") {
		if (parsed.trim() === "") {
			return undefined;
		}
		const reparsed = tryParseJson(parsed);
		if (isRecord(reparsed) || Array.isArray(reparsed)) {
			return safeJsonStringify(reparsed);
		}
		return parsed;
	}
	// Preserve primitive JSON values (null, numbers, booleans) as their
	// string representation so tool payloads like `0`, `false`, or
	// explicit `null` results are not silently dropped from the debug
	// panel.
	if (
		parsed === null ||
		typeof parsed === "number" ||
		typeof parsed === "boolean"
	) {
		return String(parsed);
	}
	if (isRecord(parsed) || Array.isArray(parsed)) {
		return safeJsonStringify(parsed);
	}
	return undefined;
};

const isToolCallPartType = (partType: string): boolean => {
	// `tool_input` is the streaming-delta form emitted by the backend
	// when a response is interrupted before the final `tool_call`
	// summary lands. Treat it as a tool-call invocation so interrupted
	// steps still surface which call was in progress.
	return (
		partType === "tool-call" ||
		partType === "tool_call" ||
		partType === "tool-input" ||
		partType === "tool_input"
	);
};

const isToolResultPartType = (partType: string): boolean => {
	return partType === "tool-result" || partType === "tool_result";
};

interface NormalizedMessagePartViewModel {
	rendered: string;
	kind?: NonNullable<MessagePart["kind"]>;
	toolCallId?: string;
	toolName?: string;
	arguments?: string;
	result?: string;
}

// ---------------------------------------------------------------------------
// Message coercion -- handles both plain objects and stringified entries.
// ---------------------------------------------------------------------------

const coerceNormalizedMessagePart = (
	part: Record<string, unknown>,
): NormalizedMessagePartViewModel => {
	const partType = (toOptionalString(part.type) ?? "").trim().toLowerCase();
	const toolName =
		toOptionalString(part.tool_name) ?? toOptionalString(part.toolName);
	const toolCallId =
		toOptionalString(part.tool_call_id) ?? toOptionalString(part.toolCallId);

	if (isToolCallPartType(partType)) {
		const label = toolName ?? toolCallId ?? "tool";
		return {
			rendered: `[tool call: ${label}]`,
			kind: "tool-call",
			toolCallId,
			toolName,
			arguments: toCodeContent(pickField(part, "arguments", "input")),
		};
	}

	if (isToolResultPartType(partType)) {
		const label = toolCallId ?? toolName ?? "tool";
		return {
			rendered: `[tool result: ${label}]`,
			kind: "tool-result",
			toolCallId,
			toolName,
			result:
				toCodeContent(pickField(part, "result", "output")) ??
				toCodeContent(part.text),
		};
	}

	const text = toOptionalString(part.text);
	if (text) {
		return { rendered: text };
	}

	const filename = toOptionalString(part.filename);
	if (filename) {
		return { rendered: `[file: ${filename}]` };
	}

	if (partType) {
		return { rendered: `[${partType}]` };
	}

	return { rendered: "" };
};

const coerceMessage = (value: unknown): MessagePart | null => {
	const parsed = deepParse(value);
	if (!isRecord(parsed)) {
		return null;
	}

	const role = toOptionalString(parsed.role) ?? "unknown";

	// The backend normalizes messages as { role, parts: [...] }.
	// Support that shape alongside older content-based shapes.
	let content = "";
	let structuredPart: NormalizedMessagePartViewModel | undefined;
	const rawParts = parsed.parts;
	if (Array.isArray(rawParts) && rawParts.length > 0) {
		const fragments: string[] = [];
		const normalizedParts: NormalizedMessagePartViewModel[] = [];
		for (const part of rawParts) {
			if (typeof part === "string") {
				fragments.push(part);
				normalizedParts.push({ rendered: part });
				continue;
			}
			if (!isRecord(part)) {
				continue;
			}
			const normalizedPart = coerceNormalizedMessagePart(part);
			normalizedParts.push(normalizedPart);
			if (normalizedPart.rendered) {
				fragments.push(normalizedPart.rendered);
			}
		}
		content = fragments.join("\n");
		if (normalizedParts.length === 1 && normalizedParts[0]?.kind) {
			structuredPart = normalizedParts[0];
		}
	}

	// Fallback to content-based shapes (OpenAI / older payloads).
	if (!content) {
		const rawContent = parsed.content;
		if (typeof rawContent === "string") {
			content = rawContent;
		} else if (Array.isArray(rawContent)) {
			const textParts: string[] = [];
			for (const part of rawContent) {
				if (typeof part === "string") {
					textParts.push(part);
				} else if (isRecord(part)) {
					const text = toOptionalString(part.text);
					if (text) {
						textParts.push(text);
					}
				}
			}
			content = textParts.join("\n");
		} else if (isRecord(rawContent)) {
			content =
				toOptionalString(rawContent.text) ?? safeJsonStringify(rawContent);
		}
	}

	// Fallback: try a top-level `text` field (used by some providers).
	if (!content) {
		const text = toOptionalString(parsed.text);
		if (text) {
			content = text;
		}
	}

	return {
		role,
		content,
		toolCallId:
			structuredPart?.toolCallId ??
			toOptionalString(pickField(parsed, "tool_call_id", "toolCallId")),
		toolName:
			structuredPart?.toolName ??
			toOptionalString(pickField(parsed, "name", "tool_name", "toolName")),
		kind: structuredPart?.kind,
		arguments: structuredPart?.arguments,
		result: structuredPart?.result,
	};
};

const coerceMessages = (value: unknown): MessagePart[] => {
	const parsed = deepParse(value);
	if (!Array.isArray(parsed)) {
		return [];
	}
	const result: MessagePart[] = [];
	for (const item of parsed) {
		const msg = coerceMessage(item);
		if (msg) {
			result.push(msg);
		}
	}
	return result;
};

// ---------------------------------------------------------------------------
// Tool definition coercion.
// ---------------------------------------------------------------------------

const coerceToolDef = (value: unknown): ToolDef | null => {
	const parsed = deepParse(value);
	if (!isRecord(parsed)) {
		return null;
	}
	// OpenAI format: { type: "function", function: { name, description } }
	const fn = isRecord(parsed.function) ? parsed.function : parsed;
	const name = toOptionalString(fn.name);
	if (!name) {
		return null;
	}
	return {
		name,
		description: toOptionalString(fn.description),
		inputSchema:
			toCodeContent(
				pickField(fn, "input_schema", "inputSchema", "parameters"),
			) ??
			toCodeContent(
				pickField(parsed, "input_schema", "inputSchema", "parameters"),
			),
	};
};

const coerceTools = (value: unknown): ToolDef[] => {
	const parsed = deepParse(value);
	if (!Array.isArray(parsed)) {
		return [];
	}
	const result: ToolDef[] = [];
	for (const item of parsed) {
		const tool = coerceToolDef(item);
		if (tool) {
			result.push(tool);
		}
	}
	return result;
};

// ---------------------------------------------------------------------------
// Tool call coercion (from responses).
// ---------------------------------------------------------------------------

const coerceToolCall = (value: unknown): ToolCallPart | null => {
	const parsed = deepParse(value);
	if (!isRecord(parsed)) {
		return null;
	}
	const fn = isRecord(parsed.function) ? parsed.function : parsed;
	const name = toOptionalString(fn.name) ?? toOptionalString(parsed.name);
	if (!name) {
		return null;
	}
	const args =
		toCodeContent(pickField(fn, "arguments", "input")) ??
		toCodeContent(pickField(parsed, "arguments", "input"));
	return {
		// Normalize an empty `tool_call_id` to `undefined` so downstream
		// dedup and React key logic treat it as "no id" rather than
		// colliding on the same empty string.
		id:
			toOptionalString(pickField(parsed, "id", "tool_call_id", "toolCallId")) ||
			undefined,
		name,
		arguments: args,
	};
};

const coerceToolCalls = (value: unknown): ToolCallPart[] => {
	const parsed = deepParse(value);
	if (!Array.isArray(parsed)) {
		return [];
	}
	const result: ToolCallPart[] = [];
	for (const item of parsed) {
		const tc = coerceToolCall(item);
		if (tc) {
			result.push(tc);
		}
	}
	return result;
};

// ---------------------------------------------------------------------------
// Known option / policy field extraction.
// ---------------------------------------------------------------------------

// Option/policy keys are expressed as `[canonical, ...aliases]`. AI
// providers mix snake_case and camelCase for the same concept; we
// canonicalize to snake_case here so downstream renderers don't have to
// check both variants (and so the key-value grid never shows duplicate
// rows for the same value).
const OPTION_KEYS: ReadonlyArray<readonly [string, ...string[]]> = [
	["temperature"],
	["top_p", "topP"],
	["top_k", "topK"],
	["max_output_tokens", "maxOutputTokens", "max_tokens", "maxTokens"],
	["frequency_penalty", "frequencyPenalty"],
	["presence_penalty", "presencePenalty"],
	["seed"],
	["stop"],
];

const POLICY_KEYS: ReadonlyArray<readonly [string, ...string[]]> = [
	["tool_choice", "toolChoice"],
	["response_format", "responseFormat"],
	["structured_output", "structuredOutput"],
	["parallel_tool_calls", "parallelToolCalls"],
];

const extractKnownFields = (
	obj: Record<string, unknown>,
	keys: ReadonlyArray<readonly [string, ...string[]]>,
): Record<string, unknown> => {
	const result: Record<string, unknown> = {};
	for (const [canonical, ...aliases] of keys) {
		for (const candidate of [canonical, ...aliases]) {
			const value = obj[candidate];
			if (value !== undefined && value !== null) {
				result[canonical] = deepParse(value);
				break;
			}
		}
	}
	return result;
};

// ---------------------------------------------------------------------------
// Public coercion: run summary.
// ---------------------------------------------------------------------------

export const coerceRunSummary = (data: unknown): RunSummaryViewModel => {
	const defaults: RunSummaryViewModel = {
		primaryLabel: "",
		endpointLabel: undefined,
		model: undefined,
		provider: undefined,
		stepCount: undefined,
		totalInputTokens: undefined,
		totalOutputTokens: undefined,
		warnings: [],
	};
	const parsed = deepParse(data);
	if (!isRecord(parsed)) {
		return defaults;
	}
	const firstMessage = toOptionalString(
		pickField(
			parsed,
			"first_message",
			"firstMessage",
			"primary_label",
			"primaryLabel",
		),
	);
	return {
		primaryLabel: firstMessage ?? "",
		endpointLabel: toOptionalString(
			pickField(parsed, "endpoint_label", "endpointLabel"),
		),
		model: toOptionalString(pickField(parsed, "model")),
		provider: toOptionalString(pickField(parsed, "provider")),
		stepCount: toFiniteNumber(
			pickField(parsed, "step_count", "stepCount", "steps"),
		),
		totalInputTokens: toFiniteNumber(
			pickField(
				parsed,
				"total_input_tokens",
				"totalInputTokens",
				"input_tokens",
				"inputTokens",
				"prompt_tokens",
				"promptTokens",
			),
		),
		totalOutputTokens: toFiniteNumber(
			pickField(
				parsed,
				"total_output_tokens",
				"totalOutputTokens",
				"output_tokens",
				"outputTokens",
				"completion_tokens",
				"completionTokens",
			),
		),
		warnings: [],
	};
};

// ---------------------------------------------------------------------------
// Public coercion: step request.
// ---------------------------------------------------------------------------

export const coerceStepRequest = (data: unknown): StepRequestViewModel => {
	const defaults: StepRequestViewModel = {
		model: undefined,
		messages: [],
		tools: [],
		options: {},
		policy: {},
	};
	const parsed = deepParse(data);
	if (!isRecord(parsed)) {
		return defaults;
	}
	// `options` and `policy` can arrive as JSON-string wrappers when the
	// payload has been round-tripped through Go's `json.RawMessage`, so
	// unwrap them before the `isRecord` branch.
	const rawOptions = deepParse(parsed.options);
	const rawPolicy = deepParse(parsed.policy);
	const optionsSource = isRecord(rawOptions) ? rawOptions : parsed;
	const policySource = isRecord(rawPolicy) ? rawPolicy : parsed;
	return {
		model: toOptionalString(pickField(parsed, "model")),
		messages: coerceMessages(pickField(parsed, "messages", "input")),
		tools: coerceTools(pickField(parsed, "tools")),
		options: extractKnownFields(optionsSource, OPTION_KEYS),
		policy: extractKnownFields(policySource, POLICY_KEYS),
	};
};

const coerceChoiceContentText = (value: unknown): string => {
	if (typeof value === "string") {
		return value;
	}
	if (!Array.isArray(value)) {
		return "";
	}
	const parts: string[] = [];
	for (const item of value) {
		if (typeof item === "string") {
			parts.push(item);
			continue;
		}
		if (!isRecord(item)) {
			continue;
		}
		const text = toOptionalString(item.text);
		if (text) {
			parts.push(text);
		}
	}
	return parts.join("");
};

// ---------------------------------------------------------------------------
// Public coercion: step response.
// ---------------------------------------------------------------------------

export const coerceStepResponse = (data: unknown): StepResponseViewModel => {
	const defaults: StepResponseViewModel = {
		content: "",
		toolCalls: [],
		finishReason: undefined,
		usage: {},
		warnings: [],
		model: undefined,
	};
	const parsed = deepParse(data);
	if (!isRecord(parsed)) {
		return defaults;
	}

	// The backend normalizes responses as:
	//   { content: [{ type, text?, tool_name?, ... }], finish_reason, usage, warnings }
	// Support that alongside plain string content and OpenAI choices.
	let content = "";
	let toolCalls: ToolCallPart[] = [];

	const rawContent = parsed.content;
	if (Array.isArray(rawContent)) {
		// Backend normalized content parts.
		const textFragments: string[] = [];
		const extractedToolCalls: ToolCallPart[] = [];
		// Streamed responses can carry a `tool_input` delta followed by a
		// final `tool_call` summary for the same call ID. Track each call
		// ID so we collapse them into a single row, preferring the
		// finalized form when both are present.
		const toolCallIndexById = new Map<string, number>();
		const finalizedToolCallIds = new Set<string>();
		for (const part of rawContent) {
			if (typeof part === "string") {
				textFragments.push(part);
				continue;
			}
			if (!isRecord(part)) {
				continue;
			}
			const partType = toOptionalString(part.type) ?? "";
			const text = toOptionalString(part.text);
			const toolResult = isToolResultPartType(partType)
				? (toCodeContent(pickField(part, "result", "output")) ??
					toCodeContent(part.text))
				: undefined;
			if (text) {
				textFragments.push(text);
			} else if (toolResult) {
				textFragments.push(toolResult);
			}
			// Extract tool calls from content parts.
			if (isToolCallPartType(partType)) {
				const name =
					toOptionalString(part.tool_name) ??
					toOptionalString(part.toolName) ??
					toOptionalString(part.name);
				if (!name) {
					continue;
				}
				// Treat an empty `tool_call_id` as "no id" so distinct streamed
				// calls don't collide on the same Map key during dedup. Go's
				// zero value for string is `""` and `ChatStreamToolCall`
				// serializes without `omitempty`, so unset IDs arrive as `""`.
				const toolCall: ToolCallPart = {
					id:
						toOptionalString(
							pickField(part, "tool_call_id", "toolCallId", "id"),
						) || undefined,
					name,
					arguments: toCodeContent(pickField(part, "arguments", "input")),
				};
				const isFinalized =
					partType === "tool-call" || partType === "tool_call";
				if (toolCall.id === undefined) {
					extractedToolCalls.push(toolCall);
					continue;
				}
				const existingIndex = toolCallIndexById.get(toolCall.id);
				if (existingIndex === undefined) {
					extractedToolCalls.push(toolCall);
					toolCallIndexById.set(toolCall.id, extractedToolCalls.length - 1);
					if (isFinalized) {
						finalizedToolCallIds.add(toolCall.id);
					}
				} else if (isFinalized && !finalizedToolCallIds.has(toolCall.id)) {
					// Replace a partial `tool_input` entry with the finalized
					// `tool_call` summary for the same call ID.
					extractedToolCalls[existingIndex] = toolCall;
					finalizedToolCallIds.add(toolCall.id);
				}
				// Otherwise: already have a finalized entry (or a duplicate
				// partial delta) -- skip to avoid duplicated rows in the
				// Debug panel.
			}
		}
		content = textFragments.join("");
		toolCalls = extractedToolCalls;
	} else if (typeof rawContent === "string") {
		content = rawContent;
	}

	// Fallback: OpenAI choices shape.
	const choices = deepParse(parsed.choices);
	let firstChoice: Record<string, unknown> | null = null;
	if (Array.isArray(choices) && choices.length > 0 && isRecord(choices[0])) {
		firstChoice = choices[0] as Record<string, unknown>;
	}
	if (!content && firstChoice) {
		const msg = isRecord(firstChoice.message)
			? firstChoice.message
			: firstChoice;
		content =
			toOptionalString(msg.content) ??
			coerceChoiceContentText(msg.content) ??
			"";
	}

	// Tool calls: merge from direct fields and first choice.
	if (toolCalls.length === 0) {
		toolCalls = coerceToolCalls(pickField(parsed, "tool_calls", "toolCalls"));
	}
	if (toolCalls.length === 0 && firstChoice) {
		const msg = isRecord(firstChoice.message)
			? firstChoice.message
			: firstChoice;
		toolCalls = coerceToolCalls(
			pickField(msg as Record<string, unknown>, "tool_calls", "toolCalls"),
		);
	}

	// Finish reason.
	let finishReason = toOptionalString(
		pickField(parsed, "finish_reason", "finishReason"),
	);
	if (!finishReason && firstChoice) {
		finishReason = toOptionalString(
			pickField(firstChoice, "finish_reason", "finishReason"),
		);
	}

	// Usage (within the response body itself).
	const usage = coerceUsageRecord(pickField(parsed, "usage"));

	// Warnings -- support both string arrays and object arrays.
	const rawWarnings = deepParse(pickField(parsed, "warnings"));
	const warnings: string[] = [];
	if (Array.isArray(rawWarnings)) {
		for (const w of rawWarnings) {
			if (typeof w === "string") {
				warnings.push(w);
			} else if (isRecord(w)) {
				const msg = toOptionalString(w.message) ?? toOptionalString(w.details);
				if (msg) {
					warnings.push(msg);
				}
			}
		}
	}

	return {
		content,
		toolCalls,
		finishReason,
		usage,
		warnings,
		model: toOptionalString(parsed.model),
	};
};

// ---------------------------------------------------------------------------
// Public coercion: usage record (string values → numbers).
// ---------------------------------------------------------------------------

export const coerceUsageRecord = (data: unknown): Record<string, number> => {
	const parsed = deepParse(data);
	if (!isRecord(parsed)) {
		return {};
	}
	const result: Record<string, number> = {};
	for (const [key, val] of Object.entries(parsed)) {
		const num = toFiniteNumber(val);
		if (num !== undefined) {
			result[key] = num;
		}
	}
	return result;
};

// ---------------------------------------------------------------------------
// Token extraction and formatting.
// ---------------------------------------------------------------------------

export const extractTokenCounts = (
	usage: Record<string, number>,
): { input?: number; output?: number; total?: number } => {
	return {
		input: usage.prompt_tokens ?? usage.input_tokens,
		output: usage.completion_tokens ?? usage.output_tokens,
		total: usage.total_tokens,
	};
};

export const formatTokenSummary = (input?: number, output?: number): string => {
	if (input !== undefined && output !== undefined) {
		return `${input.toLocaleString("en-US")}→${output.toLocaleString("en-US")} tok`;
	}
	if (input !== undefined) {
		return `${input.toLocaleString("en-US")} in`;
	}
	if (output !== undefined) {
		return `${output.toLocaleString("en-US")} out`;
	}
	return "";
};

// ---------------------------------------------------------------------------
// Role badge variant mapping.
// ---------------------------------------------------------------------------

const ROLE_BADGE_VARIANTS: Record<string, string> = {
	system: "purple",
	user: "info",
	assistant: "green",
	tool: "warning",
	function: "warning",
};

export const getRoleBadgeVariant = (
	role: string,
): "purple" | "info" | "green" | "warning" | "default" => {
	const normalized = role.trim().toLowerCase();
	return (
		(ROLE_BADGE_VARIANTS[normalized] as
			| "purple"
			| "info"
			| "green"
			| "warning"
			| undefined) ?? "default"
	);
};

// ---------------------------------------------------------------------------
// Transcript preview -- collapsed message list logic.
// ---------------------------------------------------------------------------

/** Default number of messages to show before collapsing. */
export const TRANSCRIPT_PREVIEW_COUNT = 2;

/** Max characters to display for a message body in collapsed mode. */
export const MESSAGE_CONTENT_CLAMP_CHARS = 160;

/**
 * Clamp a message body to a maximum character length, adding an
 * ellipsis when truncated.
 */
export const clampContent = (text: string, maxLen: number): string => {
	const trimmed = text.trim();
	if (trimmed.length <= maxLen) {
		return trimmed;
	}
	// Use Array.from to split on code points rather than UTF-16 code
	// units. A plain String.slice can cut a surrogate pair in half,
	// producing a lone high surrogate rendered as U+FFFD.
	const codePoints = Array.from(trimmed);
	if (codePoints.length <= maxLen) {
		return trimmed;
	}
	return `${codePoints.slice(0, maxLen).join("").trimEnd()}…`;
};

/**
 * Returns true when the text is long enough that clampContent would
 * actually truncate it. Uses the same trim+code-point count so callers
 * never offer a "see more" control for text that clampContent returns
 * unchanged.
 */
export const exceedsClampThreshold = (
	text: string,
	maxLen: number,
): boolean => {
	const trimmed = text.trim();
	if (trimmed.length <= maxLen) {
		return false;
	}
	return Array.from(trimmed).length > maxLen;
};

// ---------------------------------------------------------------------------
// Active-status helper (for spinner indicators).
// ---------------------------------------------------------------------------

export const isActiveStatus = (status: string): boolean => {
	return INFO_STATUSES.has(status.trim().toLowerCase());
};
