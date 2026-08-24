export type FilterValues = Record<string, string | undefined>;

export const parseFilterQuery = (filterQuery: string): FilterValues => {
	if (filterQuery === "") {
		return {};
	}

	const result: FilterValues = {};
	const keyValuePair = /(\w+):"([^"]+)"|(\w+):(\S+)/g;

	for (const match of filterQuery.matchAll(keyValuePair)) {
		const key = match[1] ?? match[3];
		const value = match[2] ?? match[4];
		if (key && value) {
			result[key] = value;
		}
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
