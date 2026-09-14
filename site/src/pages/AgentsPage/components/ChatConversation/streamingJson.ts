/**
 * Incremental JSON parser for streaming tool call arguments.
 *
 * LLM tool calls arrive as partial JSON fragments via server-sent events.
 * This module provides utilities to extract usable data from incomplete
 * JSON strings without waiting for the full payload.
 *
 * Guarantees:
 * - Partial object recovery (returns fields parsed so far).
 * - Graceful handling of truncated strings, numbers, and booleans.
 *
 * Known limitations:
 * - Does not handle partial arrays.
 * - Does not handle \uXXXX unicode escape sequences in strings.
 */

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
	if (input[startIndex] !== '"') {
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
		if (char !== '"') {
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
	char === undefined ||
	char === "," ||
	char === "}" ||
	char === "]" ||
	/\s/.test(char);

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
			if (char === '"') {
				inString = false;
			}
			continue;
		}

		switch (char) {
			case '"':
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
	if (char === '"') {
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
		if (
			end === input.length &&
			/^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?)?$/.test(token)
		) {
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
			switch (char) {
				case '"':
					result += '"';
					break;
				case "\\":
					result += "\\";
					break;
				case "/":
					result += "/";
					break;
				case "n":
					result += "\n";
					break;
				case "r":
					result += "\r";
					break;
				case "t":
					result += "\t";
					break;
				default:
					result += `\\${char}`;
					break;
			}
			escaped = false;
			continue;
		}
		if (char === "\\") {
			escaped = true;
			continue;
		}
		if (char === '"') {
			return result;
		}
		result += char;
	}
	return result.length > 0 ? result : null;
};

const parsePartialJSONObject = (
	value: string,
): Record<string, unknown> | null => {
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

export const parseStreamingJSON = (value: string): unknown | null => {
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

export const mergeStreamPayload = (
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

	const base =
		existingRawText ?? (typeof existingValue === "string" ? existingValue : "");
	const rawText = `${base}${chunk}`;
	const parsed = parseStreamingJSON(rawText);

	return {
		value: parsed ?? rawText,
		rawText,
	};
};
