// The backend's search-query parser toggles its quoted-state on every `"` and
// has no backslash-escape handling, so escaping quotes here would produce a
// query the backend cannot parse. Stripping quotes from bare text keeps the
// resulting `title:"..."` filter well-formed for the backend.
const sanitizeChatSearchValue = (value: string): string => {
	return value.replaceAll('"', "");
};

// Strips surrounding double quotes so URL detection and value normalization
// stay resilient to inputs pasted from JSON, CI logs, or Slack code blocks.
const stripSurroundingQuotes = (value: string): string =>
	value.replace(/^"+|"+$/g, "");

/**
 * Filter keys the chat search backend accepts as `key:value` pairs (besides
 * `title`, which has its own merging logic). The dialog's filter pill list
 * is derived from this so the two stay in sync.
 */
export const CHAT_SEARCH_FILTER_KEYS = [
	"archived",
	"diff_url",
	"has_unread",
	"pr_status",
] as const;

export type ChatSearchFilterKey = (typeof CHAT_SEARCH_FILTER_KEYS)[number];

const passthroughChatSearchFilterKeys = new Set<string>(
	CHAT_SEARCH_FILTER_KEYS,
);

// Common close-typo or shorthand spellings users reach for, mapped to the
// canonical backend keys. Resolving aliases early means a user typing
// `archive:true` or `unread:true` lands on the right filter instead of
// falling through to an always-empty title search.
const chatSearchFilterKeyAliases: Record<string, string> = {
	archive: "archived",
	unread: "has_unread",
	diff: "diff_url",
	diffurl: "diff_url",
	"diff-url": "diff_url",
	prstatus: "pr_status",
	"pr-status": "pr_status",
};

/**
 * Maps a user-typed filter key to its canonical backend equivalent. Unknown
 * keys are returned lowercased so callers can still distinguish recognized
 * filters from free-form text.
 */
export const resolveChatSearchFilterAlias = (key: string): string => {
	const lower = key.toLowerCase();
	return chatSearchFilterKeyAliases[lower] ?? lower;
};

// Matches a host that looks like a URL (e.g. `github.com`, `example.co.uk`).
// Used to detect bare URL-like input so it can be routed to the `diff_url:`
// filter instead of falling back to a useless title search on the URL string.
const HOST_WITH_PATH_PATTERN = /^[a-z0-9][a-z0-9.-]*\.[a-z]{2,}(:\d+)?\/\S*$/i;
const SCHEMED_URL_PATTERN = /^https?:\/\/\S+$/i;
const ANY_SCHEME_PATTERN = /^[a-z][a-z0-9+.-]*:\/\//i;

/**
 * Returns true when the value looks like an HTTP(S) URL or a scheme-less host
 * with a path segment. Conservative on purpose so plain title searches like
 * "fix lint" are never mistaken for URLs.
 */
export const looksLikeChatDiffURL = (value: string): boolean => {
	const trimmed = stripSurroundingQuotes(value.trim());
	if (trimmed === "") {
		return false;
	}
	if (SCHEMED_URL_PATTERN.test(trimmed)) {
		return true;
	}
	return HOST_WITH_PATH_PATTERN.test(trimmed);
};

/**
 * Normalizes a diff URL value for the backend. The backend's `validateDiffURL`
 * rejects values without an `http`/`https` scheme, so any scheme-less URL-like
 * value gets `https://` prepended. Values that already carry a scheme
 * (including non-http ones) are returned unchanged so the backend can produce
 * its usual validation error.
 */
export const normalizeChatDiffURLValue = (value: string): string => {
	const trimmed = stripSurroundingQuotes(value.trim());
	if (trimmed === "") {
		return trimmed;
	}
	if (ANY_SCHEME_PATTERN.test(trimmed)) {
		return trimmed;
	}
	if (HOST_WITH_PATH_PATTERN.test(trimmed)) {
		return `https://${trimmed}`;
	}
	return trimmed;
};

/**
 * Serializes a single `key:value` filter token for the chat search backend.
 *
 * Values containing characters that the backend's parser treats specially
 * (`:`, `"`, or whitespace) are wrapped in double quotes after stripping any
 * internal quotes. Without this, a value like `https://github.com/...` would
 * be split on its `:` and rejected with a "can only contain 1 ':'" error.
 *
 * For `diff_url`, scheme-less URL-like values get `https://` auto-prepended
 * because the backend rejects URLs without a scheme.
 */
export const formatChatSearchFilterToken = (
	key: string,
	rawValue: string,
): string => {
	let value = sanitizeChatSearchValue(rawValue);
	if (key === "diff_url") {
		value = normalizeChatDiffURLValue(value);
	}
	if (/[\s:]/.test(value)) {
		return `${key}:"${value}"`;
	}
	return `${key}:${value}`;
};

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
): { key: string; value: string } | undefined => {
	const delimiterIndex = getKeyValueDelimiterIndex(token);
	if (
		delimiterIndex === undefined ||
		delimiterIndex === 0 ||
		delimiterIndex === token.length - 1
	) {
		return undefined;
	}

	return {
		key: resolveChatSearchFilterAlias(
			token.slice(0, delimiterIndex).replaceAll('"', ""),
		),
		value: token.slice(delimiterIndex + 1).replace(/^"|"$/g, ""),
	};
};

// Sentinel placeholder so we can reserve the position of the merged title
// filter in the output while still collecting title terms from the rest of
// the input. The placeholder is replaced with the formatted `title:"..."`
// token (or removed) at the end of normalization.
const TITLE_PLACEHOLDER = "\u0000__TITLE__\u0000";

/**
 * Normalizes raw search input into a query string the chat search API accepts.
 *
 * Behaviors:
 *   - Bare text and `title:` filters merge into a single `title:"..."` filter
 *     because the backend rejects a parameter that appears more than once.
 *     The merged title takes the position of the first title-like token so
 *     callers that pass an already-well-formed query get a stable string
 *     back.
 *   - Bare tokens that look like HTTP(S) URLs are routed to `diff_url:` so a
 *     pasted diff link finds the matching chat instead of running an
 *     always-empty title search on the URL string.
 *   - Filter keys are resolved through {@link resolveChatSearchFilterAlias}
 *     so common aliases and near-miss typos land on the canonical backend
 *     key.
 *   - Recognized `key:value` filters are re-serialized through
 *     {@link formatChatSearchFilterToken} so values containing `:` are quoted
 *     and `diff_url` values without a scheme get `https://` prepended.
 */
export const normalizeChatSearchInput = (
	rawInput: string,
): string | undefined => {
	const trimmedInput = rawInput.trim();
	if (trimmedInput === "") {
		return undefined;
	}

	const tokens = splitSearchInput(trimmedInput);
	const outputTokens: string[] = [];
	const filterKeys = new Set<string>();
	const titleTerms: string[] = [];
	let diffURLValue: string | undefined;
	let diffURLIndex: number | undefined;

	const reserveTitlePlaceholder = (): void => {
		if (!outputTokens.includes(TITLE_PLACEHOLDER)) {
			outputTokens.push(TITLE_PLACEHOLDER);
		}
	};

	const collectTitleTerm = (value: string): void => {
		const sanitized = sanitizeChatSearchValue(value);
		if (sanitized === "") {
			return;
		}
		titleTerms.push(sanitized);
		reserveTitlePlaceholder();
	};

	const collectDiffURL = (value: string): void => {
		// The backend rejects a repeated parameter, so only the first diff_url
		// survives; later URL-like tokens fall back to title text.
		if (diffURLValue !== undefined) {
			collectTitleTerm(value);
			return;
		}
		diffURLValue = value;
		diffURLIndex = outputTokens.length;
		outputTokens.push("");
		filterKeys.add("diff_url");
	};

	for (const token of tokens) {
		const keyValuePair = getKeyValuePair(token);

		if (!keyValuePair) {
			if (looksLikeChatDiffURL(token)) {
				collectDiffURL(token);
				continue;
			}
			collectTitleTerm(token);
			continue;
		}

		if (keyValuePair.key === "title") {
			collectTitleTerm(keyValuePair.value);
			continue;
		}

		if (!passthroughChatSearchFilterKeys.has(keyValuePair.key)) {
			// Unknown key. The most common shape is a bare URL like
			// `https://github.com/...` whose first `:` makes the splitter
			// think it has a key (`https`, `gitlab.com`, etc.). Route any
			// token that still looks like a URL to `diff_url:` instead of
			// letting it fall through to a title search that will never
			// match.
			if (looksLikeChatDiffURL(token)) {
				collectDiffURL(token);
				continue;
			}
			collectTitleTerm(token);
			continue;
		}

		if (keyValuePair.key === "diff_url") {
			collectDiffURL(keyValuePair.value);
			continue;
		}

		if (filterKeys.has(keyValuePair.key)) {
			continue;
		}
		filterKeys.add(keyValuePair.key);
		outputTokens.push(
			formatChatSearchFilterToken(keyValuePair.key, keyValuePair.value),
		);
	}

	if (diffURLValue !== undefined && diffURLIndex !== undefined) {
		outputTokens[diffURLIndex] = formatChatSearchFilterToken(
			"diff_url",
			diffURLValue,
		);
	}

	const titleTerm = titleTerms.join(" ").trim();
	const titleToken =
		titleTerm === "" ? "" : formatChatSearchFilterToken("title", titleTerm);

	const finalTokens = outputTokens
		.map((token) => (token === TITLE_PLACEHOLDER ? titleToken : token))
		.filter((token) => token !== "");

	if (finalTokens.length === 0) {
		return undefined;
	}
	return finalTokens.join(" ");
};
