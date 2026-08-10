import type { SearchFilter } from "./ChatSearchInput";

export const CHAT_SEARCH_FILTER_KEYS = [
	"has_unread",
	"archived",
	"pr_status",
	"diff_url",
] as const;

export type ChatSearchFilterKey = (typeof CHAT_SEARCH_FILTER_KEYS)[number];

export const KNOWN_FILTER_KEYS: ReadonlySet<string> = new Set(
	CHAT_SEARCH_FILTER_KEYS,
);

const isChatSearchFilterKey = (key: string): key is ChatSearchFilterKey =>
	KNOWN_FILTER_KEYS.has(key);

// The backend toggles its quoted state on every `"` and has no escape handling.
// Stripping embedded quotes keeps the wrapper token well-formed.
const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

const addDefaultURLScheme = (value: string): string => {
	return /^[a-z][a-z\d+\-.]*:\/\//i.test(value) ? value : `https://${value}`;
};

const normalizeChatSearchFilterValue = (key: string, value: string): string => {
	const sanitizedValue = sanitizeChatSearchValue(value).trim();
	if (sanitizedValue === "") {
		return "";
	}
	if (key === "diff_url") {
		return addDefaultURLScheme(sanitizedValue);
	}
	if (key === "pr_status") {
		return sanitizedValue
			.toLowerCase()
			.split(/[\s,]+/)
			.filter(Boolean)
			.join(",");
	}
	if (key === "has_unread" || key === "archived") {
		return sanitizedValue.toLowerCase();
	}
	return sanitizedValue;
};

const validPRStatuses = new Set(["draft", "open", "merged", "closed"]);

const isValidDiffURL = (value: string): boolean => {
	if (!/^https?:\/\/[^/?#\s]+/i.test(value)) {
		return false;
	}
	try {
		const url = new URL(value);
		return (
			(url.protocol === "http:" || url.protocol === "https:") &&
			Boolean(url.host)
		);
	} catch {
		return false;
	}
};

const CHAT_SEARCH_FILTER_VALIDATORS: Readonly<
	Record<ChatSearchFilterKey, (value: string) => boolean>
> = {
	has_unread: (value) => value === "true" || value === "false",
	archived: (value) => value === "true" || value === "false",
	pr_status: (value) =>
		value.split(",").every((status) => validPRStatuses.has(status)),
	diff_url: isValidDiffURL,
};

export const isValidChatSearchFilterValue = (
	key: string,
	value: string,
): boolean => {
	if (!isChatSearchFilterKey(key)) {
		return false;
	}
	const normalizedValue = normalizeChatSearchFilterValue(key, value);
	if (normalizedValue === "") {
		return false;
	}
	return CHAT_SEARCH_FILTER_VALIDATORS[key](normalizedValue);
};

const formatChatSearchFilterToken = (key: string, value: string): string => {
	const formattedValue = normalizeChatSearchFilterValue(key, value);
	// The backend splits on unquoted whitespace and colons, so filter values that
	// contain either delimiter must be wrapped in quotes.
	return formattedValue.includes(":") || formattedValue.includes(" ")
		? `${key}:"${formattedValue}"`
		: `${key}:${formattedValue}`;
};

// Frontend-emitted query shapes must match TestSearchChatsFrontendEmitted in
// coderd/searchquery/search_test.go.
export const buildChatSearchQuery = (
	filters: readonly SearchFilter[],
	freeText: string,
): { query: string | undefined; hasSearchText: boolean } => {
	const parts: string[] = [];

	for (const filter of filters) {
		if (
			filter.value !== null &&
			isValidChatSearchFilterValue(filter.key, filter.value)
		) {
			parts.push(formatChatSearchFilterToken(filter.key, filter.value));
		}
	}

	const text = sanitizeChatSearchValue(freeText).trim();
	const hasSearchText = text
		.split(/\s+/)
		.some((token) => token !== "OR" && /[\p{L}\p{N}]/u.test(token));
	if (text !== "") {
		// Quotes make the complete search value one backend token. OR and
		// -negation remain live; quoted phrases are flattened to AND-of-words
		// because the backend tokenizer cannot carry embedded quotes.
		parts.push(`search:"${text}"`);
	}

	return {
		query: parts.length > 0 ? parts.join(" ") : undefined,
		hasSearchText,
	};
};

type SearchInputToken = {
	readonly value: string;
	readonly quotesBalanced: boolean;
};

const splitSearchInput = (input: string): SearchInputToken[] => {
	const tokens: SearchInputToken[] = [];
	let token = "";
	let quoted = false;

	for (const character of input) {
		if (character === '"') {
			quoted = !quoted;
		}

		if (/\s/.test(character) && !quoted) {
			if (token !== "") {
				tokens.push({ value: token, quotesBalanced: true });
				token = "";
			}
			continue;
		}

		token += character;
	}

	if (token !== "") {
		tokens.push({ value: token, quotesBalanced: !quoted });
	}

	return tokens;
};

const stripSurroundingQuotes = (value: string): string => {
	return value.startsWith('"') && value.endsWith('"')
		? value.slice(1, -1)
		: value;
};

/**
 * Extracts complete recognized filters from typed text. Unbalanced quoted and
 * invalid filter tokens pass through unchanged. `consumed` reports whether any
 * filter token was removed. It can be true while `filters` is empty when the
 * typed value already matches the active pill. The caller owns any separator
 * needed after suppressing the triggering Space keystroke.
 */
export const extractTypedFilters = (
	text: string,
	knownKeys: ReadonlySet<string>,
	activeFilters: readonly SearchFilter[],
): {
	filters: SearchFilter[];
	remainingText: string;
	consumed: boolean;
} => {
	const tokens = splitSearchInput(text.trim());
	const activeValues = new Map(
		activeFilters.map((filter) => [
			filter.key.toLowerCase(),
			filter.value === null
				? null
				: normalizeChatSearchFilterValue(filter.key, filter.value),
		]),
	);
	const filtersByKey = new Map<string, SearchFilter>();
	const remainingTokens: string[] = [];
	let consumed = false;

	for (const token of tokens) {
		if (!token.quotesBalanced) {
			remainingTokens.push(token.value);
			continue;
		}

		const colonIndex = token.value.indexOf(":");
		if (colonIndex <= 0 || colonIndex === token.value.length - 1) {
			remainingTokens.push(token.value);
			continue;
		}

		const key = token.value.slice(0, colonIndex).toLowerCase();
		const value = stripSurroundingQuotes(
			token.value.slice(colonIndex + 1),
		).trim();
		if (!knownKeys.has(key) || !isValidChatSearchFilterValue(key, value)) {
			remainingTokens.push(token.value);
			continue;
		}

		consumed = true;
		const normalizedValue = normalizeChatSearchFilterValue(key, value);
		if (activeValues.get(key) === normalizedValue) {
			filtersByKey.delete(key);
		} else {
			filtersByKey.set(key, { key, value });
		}
	}

	return {
		filters: [...filtersByKey.values()],
		remainingText: remainingTokens.join(" "),
		consumed,
	};
};
