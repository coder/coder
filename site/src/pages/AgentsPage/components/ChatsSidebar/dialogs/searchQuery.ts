import type { SearchFilter } from "./ChatSearchInput";

// The backend toggles its quoted state on every `"` and has no escape handling.
// Stripping embedded quotes keeps the wrapper token well-formed.
export const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

const addDefaultURLScheme = (value: string): string => {
	return /^[a-z][a-z\d+\-.]*:\/\//i.test(value) ? value : `https://${value}`;
};

// The backend splits on unquoted whitespace and colons, so filter values that
// contain either delimiter must be wrapped in quotes.
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
			.split(/[\s,]+/)
			.filter(Boolean)
			.join(",");
	}
	return sanitizedValue;
};

const formatChatSearchFilterToken = (key: string, value: string): string => {
	const formattedValue = normalizeChatSearchFilterValue(key, value);
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
			normalizeChatSearchFilterValue(filter.key, filter.value) !== ""
		) {
			parts.push(formatChatSearchFilterToken(filter.key, filter.value));
		}
	}

	const text = sanitizeChatSearchValue(freeText).trim();
	const hasSearchText = /[\p{L}\p{N}]/u.test(text);
	if (hasSearchText) {
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
 * Extracts complete recognized filters from typed text. Unbalanced quoted
 * tokens pass through unchanged. `consumed` reports whether any filter token
 * was removed. It can be true while `filters` is empty when the typed value
 * already matches the active pill. When the last token is consumed,
 * `remainingText` keeps a trailing space so a suppressed Space keystroke still
 * separates the next word.
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
		activeFilters.map((filter) => [filter.key.toLowerCase(), filter.value]),
	);
	const filtersByKey = new Map<string, SearchFilter>();
	const remainingTokens: string[] = [];
	const consumedTokenIndexes = new Set<number>();

	for (const [index, token] of tokens.entries()) {
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
		if (!knownKeys.has(key)) {
			remainingTokens.push(token.value);
			continue;
		}

		const value = stripSurroundingQuotes(
			token.value.slice(colonIndex + 1),
		).trim();
		if (sanitizeChatSearchValue(value).trim() === "") {
			remainingTokens.push(token.value);
			continue;
		}

		consumedTokenIndexes.add(index);
		if (activeValues.get(key) === value) {
			filtersByKey.delete(key);
		} else {
			filtersByKey.set(key, { key, value });
		}
	}

	const consumed = consumedTokenIndexes.size > 0;
	const needsTrailingSeparator =
		remainingTokens.length > 0 && consumedTokenIndexes.has(tokens.length - 1);

	return {
		filters: [...filtersByKey.values()],
		remainingText: `${remainingTokens.join(" ")}${needsTrailingSeparator ? " " : ""}`,
		consumed,
	};
};
