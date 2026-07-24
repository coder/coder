// The backend's search-query parser toggles its quoted-state on every `"` and
// has no backslash-escape handling, so escaping quotes here would produce a
// query the backend cannot parse. Stripping quotes from bare text keeps the
// resulting `title:"..."` filter well-formed for the backend.
const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

const addDefaultURLScheme = (value: string): string => {
	return /^[a-z][a-z\d+\-.]*:\/\//i.test(value) ? value : `https://${value}`;
};

// Filter keys that may pass through to the backend unchanged. `title` is not
// listed here because bare text and `title:` filters are merged into a single
// title filter; see the title-handling branch in normalizeChatSearchInput.
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
 * Bare text and `title:` filters are merged into a single `title:"..."`
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
	const titleTerms: string[] = [];
	let hasBareTitleText = false;

	for (const token of tokens) {
		const keyValuePair = getKeyValuePair(token);
		if (!keyValuePair) {
			titleTerms.push(token);
			hasBareTitleText = true;
			continue;
		}

		if (keyValuePair.key === "title") {
			normalizedTokens.push(token);
			titleTerms.push(keyValuePair.value);
			continue;
		}

		if (!passthroughChatSearchFilterKeys.has(keyValuePair.key)) {
			titleTerms.push(token);
			hasBareTitleText = true;
			continue;
		}

		const normalizedFilter = normalizePassthroughChatSearchFilter(keyValuePair);
		passthroughFilters.push(normalizedFilter);
		normalizedTokens.push(normalizedFilter);
	}

	// Multiple title values must be merged into a single title filter because
	// the backend's query parser rejects the same key appearing more than once.
	if (titleTerms.length > 1) {
		hasBareTitleText = true;
	}

	if (!hasBareTitleText) {
		return normalizedTokens.join(" ");
	}

	return [
		...passthroughFilters,
		`title:"${sanitizeChatSearchValue(titleTerms.join(" "))}"`,
	].join(" ");
};
