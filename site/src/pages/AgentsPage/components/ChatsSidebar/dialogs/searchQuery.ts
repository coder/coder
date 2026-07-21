// The backend's search-query parser toggles its quoted-state on every `"` and
// has no backslash-escape handling, so escaping quotes here would produce a
// query the backend cannot parse. Stripping quotes from structured filter
// values keeps the resulting `key:"..."` token well-formed for the backend.
// Bare free text is not sanitized this way so that FTS quoted phrases survive.
const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

const addDefaultURLScheme = (value: string): string => {
	return /^[a-z][a-z\d+\-.]*:\/\//i.test(value) ? value : `https://${value}`;
};

// Bare free text may contain websearch operators (quoted phrases, OR,
// -negation). Detect a leading/trailing quote pair so those pass through
// unmodified; everything else gets wrapped in a single quoted phrase.
const hasWebSearchQuotes = (value: string): boolean => {
	const first = value.indexOf('"');
	const last = value.lastIndexOf('"');
	return (
		first !== -1 && last > first && /\S/.test(value.slice(first + 1, last))
	);
};

// Wrap bare free text in a quoted phrase so multi-word input reaches the
// backend's FTS filter as a single token. Quotes are stripped first because
// the backend's query parser has no escape handling for embedded quotes.
const toSearchPhrase = (terms: string): string => {
	const joined = terms.trim();
	if (hasWebSearchQuotes(joined)) {
		return joined;
	}
	return `"${sanitizeChatSearchValue(joined)}"`;
};

// Filter keys that may pass through to the backend unchanged. `title` is not
// listed here because bare text and `title:` filters are merged into a single
// FTS `search:` filter; see the search-handling branch in
// normalizeChatSearchInput.
const passthroughChatSearchFilterKeys = new Set([
	"archived",
	"diff_url",
	"has_unread",
	"pr_status",
]);

const splitSearchInput = (input: string): string[] => {
	const tokens: string[] = [];
	let token = "";
	let quoted = false;

	for (const character of input) {
		if (character === '"') {
			quoted = !quoted;
		}

		if (/\s/.test(character) && !quoted) {
			if (token !== "") {
				tokens.push(token);
				token = "";
			}
			continue;
		}

		token += character;
	}

	if (token !== "") {
		tokens.push(token);
	}

	return tokens;
};

const getKeyValueDelimiterIndex = (token: string): number | undefined => {
	let quoted = false;

	for (const [index, character] of [...token].entries()) {
		if (character === '"') {
			quoted = !quoted;
		}

		if (character === ":" && !quoted) {
			return index;
		}
	}

	return undefined;
};

const getKeyValuePair = (
	token: string,
): { key: string; rawKey: string; value: string } | undefined => {
	const delimiterIndex = getKeyValueDelimiterIndex(token);
	if (
		delimiterIndex === undefined ||
		delimiterIndex === 0 ||
		delimiterIndex === token.length - 1
	) {
		return undefined;
	}

	const rawKey = token.slice(0, delimiterIndex).replaceAll('"', "");
	return {
		key: rawKey.toLowerCase(),
		rawKey,
		value: token.slice(delimiterIndex + 1).replace(/^"|"$/g, ""),
	};
};

// The backend splits on unquoted whitespace and colons, so values containing
// either (e.g. a diff URL) must be quoted.
const normalizePassthroughChatSearchFilter = ({
	key,
	rawKey,
	value,
}: {
	readonly key: string;
	readonly rawKey: string;
	readonly value: string;
}): string => {
	const sanitizedValue =
		key === "diff_url"
			? addDefaultURLScheme(sanitizeChatSearchValue(value))
			: sanitizeChatSearchValue(value);
	return sanitizedValue.includes(":") || sanitizedValue.includes(" ")
		? `${rawKey}:"${sanitizedValue}"`
		: `${rawKey}:${sanitizedValue}`;
};

/**
 * Normalizes raw search input into a query string the chat search API accepts.
 *
 * Bare text and `title:` filters are merged into a single `search:` FTS
 * filter (the backend rejects a parameter that appears more than once).
 * Recognized `key:value` filters are normalized for backend syntax.
 */
export const normalizeChatSearchInput = (
	rawInput: string,
): string | undefined => {
	const trimmedInput = rawInput.trim();
	if (trimmedInput === "") {
		return undefined;
	}

	const tokens = splitSearchInput(trimmedInput);
	const passthroughFilters: string[] = [];
	const normalizedTokens: string[] = [];
	const searchTerms: string[] = [];
	let hasBareSearchText = false;

	for (const token of tokens) {
		const keyValuePair = getKeyValuePair(token);
		if (!keyValuePair) {
			searchTerms.push(token);
			hasBareSearchText = true;
			continue;
		}

		if (keyValuePair.key === "title") {
			normalizedTokens.push(token);
			searchTerms.push(keyValuePair.value);
			continue;
		}

		if (!passthroughChatSearchFilterKeys.has(keyValuePair.key)) {
			searchTerms.push(token);
			hasBareSearchText = true;
			continue;
		}

		const normalizedFilter = normalizePassthroughChatSearchFilter(keyValuePair);
		passthroughFilters.push(normalizedFilter);
		normalizedTokens.push(normalizedFilter);
	}

	// Multiple search values must be merged into a single search filter because
	// the backend's query parser rejects the same key appearing more than once.
	if (searchTerms.length > 1) {
		hasBareSearchText = true;
	}

	if (!hasBareSearchText) {
		return normalizedTokens.join(" ");
	}

	// Free text defaults to the backend's full-text search filter, which
	// matches chat titles, PR titles, and message bodies.
	return [
		...passthroughFilters,
		`search:${toSearchPhrase(searchTerms.join(" "))}`,
	].join(" ");
};
