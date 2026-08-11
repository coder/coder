import type { SearchFilter } from "./ChatSearchInput";

export const CHAT_SEARCH_FILTER_KEYS = [
	"has_unread",
	"archived",
	"pr_status",
	"diff_url",
] as const;

export type ChatSearchFilterKey = (typeof CHAT_SEARCH_FILTER_KEYS)[number];

const CHAT_SEARCH_KNOWN_FILTER_KEYS: ReadonlySet<string> = new Set(
	CHAT_SEARCH_FILTER_KEYS,
);

const isChatSearchFilterKey = (key: string): key is ChatSearchFilterKey =>
	CHAT_SEARCH_KNOWN_FILTER_KEYS.has(key);

// The backend toggles its quoted state on every `"` and has no escape handling.
// Stripping embedded quotes keeps the wrapper token well-formed.
const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

const addDefaultURLScheme = (value: string): string => {
	return /^[a-z][a-z\d+\-.]*:\/\//i.test(value) ? value : `https://${value}`;
};

export const normalizeChatSearchFilterValue = (
	key: string,
	value: string,
): string => {
	const sanitizedValue = sanitizeChatSearchValue(value).trim();
	if (sanitizedValue === "") {
		return "";
	}
	if (key === "diff_url") {
		return addDefaultURLScheme(sanitizedValue);
	}
	if (key === "pr_status") {
		return sanitizedValue
			.split(/[\s,]+/)
			.filter(Boolean)
			.join(",");
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
	has_unread: (value) => /^(true|false)$/i.test(value),
	archived: (value) => /^(true|false)$/i.test(value),
	pr_status: (value) =>
		value
			.split(",")
			.every((status) => validPRStatuses.has(status.toLowerCase())),
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
): string | undefined => {
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
	if (freeText.trim() !== "") {
		// The wrapper quotes make the value one token for the backend parser and
		// are stripped before FTS, so OR and -negation stay live but typed phrase
		// quotes are lost. Input that sanitizes to nothing (e.g. a lone `"`)
		// still yields no results, not recent chats; the backend rejects an empty
		// search value, so a single space stands in for it (it produces an empty
		// tsquery, which matches nothing).
		parts.push(`search:"${text === "" ? " " : text}"`);
	}

	return parts.length > 0 ? parts.join(" ") : undefined;
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

/**
 * Extracts recognized filters from typed text. Unbalanced-quoted and invalid
 * tokens pass through unchanged. `consumed` is true if any filter token was
 * removed, even when `filters` is empty (a typed value equal to the active
 * pill). The caller owns any separator after a suppressed Space keystroke.
 */
export const extractTypedFilters = (
	text: string,
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

	let tokenIndex = 0;
	while (tokenIndex < tokens.length) {
		const token = tokens[tokenIndex];
		if (!token.quotesBalanced) {
			remainingTokens.push(token.value);
			tokenIndex += 1;
			continue;
		}

		const colonIndex = token.value.indexOf(":");
		if (colonIndex <= 0 || colonIndex === token.value.length - 1) {
			remainingTokens.push(token.value);
			tokenIndex += 1;
			continue;
		}

		const key = token.value.slice(0, colonIndex).toLowerCase();
		let value = token.value
			.slice(colonIndex + 1)
			.replace(/^"|"$/g, "")
			.trim();
		const candidateTokens = [token.value];
		let nextTokenIndex = tokenIndex + 1;
		if (key === "pr_status" && value.endsWith(",")) {
			while (value.endsWith(",") && nextTokenIndex < tokens.length) {
				const nextToken = tokens[nextTokenIndex];
				value = `${value} ${nextToken.value}`;
				candidateTokens.push(nextToken.value);
				nextTokenIndex += 1;
			}
		}

		if (
			!CHAT_SEARCH_KNOWN_FILTER_KEYS.has(key) ||
			value.endsWith(",") ||
			!isValidChatSearchFilterValue(key, value)
		) {
			remainingTokens.push(...candidateTokens);
			tokenIndex = nextTokenIndex;
			continue;
		}

		consumed = true;
		const normalizedValue = normalizeChatSearchFilterValue(key, value);
		if (activeValues.get(key) === normalizedValue) {
			filtersByKey.delete(key);
		} else {
			filtersByKey.set(key, {
				key,
				value: key === "pr_status" ? normalizedValue : value,
			});
		}
		tokenIndex = nextTokenIndex;
	}

	return {
		filters: [...filtersByKey.values()],
		remainingText: remainingTokens.join(" "),
		consumed,
	};
};
