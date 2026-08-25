export type FilterValues = Record<string, string | undefined>;

// Matches `key:"quoted value"` or `key:bareValue` tokens. Keys allow hyphens so
// documented backend filters like `has-agent:connected` parse as one token.
export const FILTER_TOKEN_RE = /([\w-]+):"([^"]+)"|([\w-]+):(\S+)/g;

/** Yields each `key:value` token in a query, in order, with values unquoted. */
export function* iterateFilterTokens(
	query: string,
): Generator<{ key: string; value: string }> {
	for (const match of query.matchAll(FILTER_TOKEN_RE)) {
		const key = match[1] ?? match[3];
		const value = match[2] ?? match[4];
		if (key && value) {
			yield { key, value };
		}
	}
}

export const parseFilterQuery = (filterQuery: string): FilterValues => {
	if (filterQuery === "") {
		return {};
	}

	const result: FilterValues = {};
	for (const { key, value } of iterateFilterTokens(filterQuery)) {
		result[key] = value;
	}

	return result;
};

// Values containing spaces or colons must be quoted: the backend query
// parser splits unquoted elements on ':' and rejects more than one colon.
export const needsQuotes = (value: string): boolean =>
	value.includes(" ") || value.includes(":");

export const stringifyFilter = (filterValue: FilterValues): string => {
	let result = "";

	for (const key in filterValue) {
		const value = filterValue[key];
		if (value) {
			result += needsQuotes(value) ? `${key}:"${value}" ` : `${key}:${value} `;
		}
	}

	return result.trim();
};
