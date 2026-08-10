import type { SearchFilter } from "./ChatSearchInput";

// The backend toggles its quoted state on every `"` and has no escape handling.
// Stripping embedded quotes keeps the wrapper token well-formed.
const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

const addDefaultURLScheme = (value: string): string => {
	return /^[a-z][a-z\d+\-.]*:\/\//i.test(value) ? value : `https://${value}`;
};

// The backend splits on unquoted whitespace and colons, so filter values that
// contain either delimiter must be wrapped in quotes.
const formatChatSearchFilterToken = (key: string, value: string): string => {
	const sanitizedValue = sanitizeChatSearchValue(value).trim();
	const formattedValue =
		key === "diff_url" ? addDefaultURLScheme(sanitizedValue) : sanitizedValue;
	return formattedValue.includes(":") || formattedValue.includes(" ")
		? `${key}:"${formattedValue}"`
		: `${key}:${formattedValue}`;
};

export const buildChatSearchQuery = (
	filters: readonly SearchFilter[],
	freeText: string,
): { query: string | undefined; hasSearchText: boolean } => {
	const parts: string[] = [];

	for (const filter of filters) {
		if (
			filter.value !== null &&
			sanitizeChatSearchValue(filter.value).trim() !== ""
		) {
			parts.push(formatChatSearchFilterToken(filter.key, filter.value));
		}
	}

	const text = sanitizeChatSearchValue(freeText).trim();
	const hasSearchText = /[\p{L}\p{N}]/u.test(text);
	if (hasSearchText) {
		// Quotes make the complete search value one backend token. The backend
		// strips them during tokenization, then websearch_to_tsquery interprets
		// the text, so OR and -negation remain active. The backend matches the
		// value against chat titles, PR titles, and message bodies, and against
		// an exact PR number when the value is numeric.
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

export const extractTypedFilters = (
	text: string,
	knownKeys: ReadonlySet<string>,
	activeKeys: ReadonlySet<string>,
): {
	filters: SearchFilter[];
	remainingText: string;
	consumed: boolean;
} => {
	const tokens = splitSearchInput(text.trim());
	const normalizedActiveKeys = new Set(
		[...activeKeys].map((key) => key.toLowerCase()),
	);
	const filters: SearchFilter[] = [];
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

		consumedTokenIndexes.add(index);
		if (!normalizedActiveKeys.has(key)) {
			filters.push({
				key,
				value: stripSurroundingQuotes(token.value.slice(colonIndex + 1)),
			});
			normalizedActiveKeys.add(key);
		}
	}

	const consumed = consumedTokenIndexes.size > 0;
	const needsTrailingSeparator =
		remainingTokens.length > 0 && consumedTokenIndexes.has(tokens.length - 1);

	return {
		filters,
		remainingText: `${remainingTokens.join(" ")}${needsTrailingSeparator ? " " : ""}`,
		consumed,
	};
};
