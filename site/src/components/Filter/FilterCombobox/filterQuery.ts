import type { ReactNode } from "react";
import type { FilterOption } from "./types";

const FILTER_TOKEN_RE = /(\w+):"([^"]+)"|(\w+):(\S+)/g;

export const chipToken = (key: string, value: string) => `${key}:${value}`;

export const parseChipToken = (
	token: string,
	chipKeys: readonly string[],
): { key: string; value: string } | null => {
	const separatorIndex = token.indexOf(":");
	if (separatorIndex <= 0) {
		return null;
	}

	const key = token.slice(0, separatorIndex);
	const value = token.slice(separatorIndex + 1);
	if (!chipKeys.includes(key) || value.length === 0) {
		return null;
	}

	return { key, value };
};

/** Structured `key:value` tokens from a query string for known chip keys. */
const parseFilterValues = (
	query: string,
	chipKeys: readonly string[],
): Record<string, string | undefined> => {
	const values: Record<string, string | undefined> = {};
	for (const key of chipKeys) {
		values[key] = undefined;
	}

	for (const match of query.matchAll(FILTER_TOKEN_RE)) {
		const key = match[1] ?? match[3];
		const value = match[2] ?? match[4];
		if (!key || !value || !chipKeys.includes(key)) {
			continue;
		}
		values[key] = value;
	}

	return values;
};

export const filterValuesToChips = (
	values: Record<string, string | undefined>,
	chipKeys: readonly string[],
): string[] => {
	const chips: string[] = [];
	for (const key of chipKeys) {
		const value = values[key];
		if (value) {
			chips.push(chipToken(key, value));
		}
	}
	return chips;
};

export const queryToChips = (
	query: string,
	chipKeys: readonly string[],
): string[] => {
	return filterValuesToChips(parseFilterValues(query, chipKeys), chipKeys);
};

const dedupeChipsByFacet = (
	tokens: readonly string[],
	chipKeys: readonly string[],
): string[] => {
	const byKey = new Map<string, string>();
	for (const token of tokens) {
		const parsed = parseChipToken(token, chipKeys);
		if (!parsed) {
			continue;
		}
		byKey.set(parsed.key, parsed.value);
	}
	return chipKeys.flatMap((key) => {
		const value = byKey.get(key);
		return value ? [chipToken(key, value)] : [];
	});
};

export const chipsToValues = (
	tokens: readonly string[],
	chipKeys: readonly string[],
): Record<string, string | undefined> => {
	const next: Record<string, string | undefined> = {};
	for (const key of chipKeys) {
		next[key] = undefined;
	}
	for (const token of dedupeChipsByFacet(tokens, chipKeys)) {
		const parsed = parseChipToken(token, chipKeys);
		if (!parsed) {
			continue;
		}
		next[parsed.key] = parsed.value;
	}
	return next;
};

/** Structured `key:value` tokens only; bare text is treated as free-text search. */
export const extractFreeText = (query: string): string => {
	return query.replace(FILTER_TOKEN_RE, " ").replace(/\s+/g, " ").trim();
};

export const stringifyChipValues = (
	values: Record<string, string | undefined>,
	chipKeys: readonly string[],
): string => {
	const parts: string[] = [];
	for (const key of chipKeys) {
		const value = values[key];
		if (!value) {
			continue;
		}
		parts.push(value.includes(" ") ? `${key}:"${value}"` : `${key}:${value}`);
	}
	return parts.join(" ");
};

export const composeFilterQuery = (
	values: Record<string, string | undefined>,
	chipKeys: readonly string[],
	freeText: string,
): string => {
	return [stringifyChipValues(values, chipKeys), freeText.trim()]
		.filter((part) => part.length > 0)
		.join(" ");
};

type CategoryMatchSource = {
	key: string;
	label: string;
	aliases?: readonly string[];
};

export const parseTypedCategoryPrefix = (
	raw: string,
	categories: readonly CategoryMatchSource[],
): { categoryKey: string; query: string; freeText: string } | null => {
	// Allow `owner:` at the start, or after name search text: `pink owner:`.
	const match = /^(.*?)\s*(\w+)\s*:(.*)$/.exec(raw);
	if (!match) {
		return null;
	}

	const typedKey = match[2]?.toLowerCase();
	if (!typedKey) {
		return null;
	}

	const category = categories.find((entry) => {
		if (entry.key === typedKey || entry.label.toLowerCase() === typedKey) {
			return true;
		}
		return entry.aliases?.some((alias) => alias.toLowerCase() === typedKey);
	});
	if (!category) {
		return null;
	}

	return {
		categoryKey: category.key,
		query: match[3] ?? "",
		freeText: (match[1] ?? "").trim(),
	};
};

/** Categories whose key, label, or alias starts with the typed query. */
export const matchCategories = <T extends CategoryMatchSource>(
	query: string,
	categories: readonly T[],
): T[] => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return [];
	}

	return categories.filter((category) => {
		if (category.key.toLowerCase().startsWith(normalized)) {
			return true;
		}
		if (category.label.toLowerCase().startsWith(normalized)) {
			return true;
		}
		return (
			category.aliases?.some((alias) =>
				alias.toLowerCase().startsWith(normalized),
			) ?? false
		);
	});
};

type CategoryValueSuggestion = {
	categoryKey: string;
	categoryLabel: string;
	option: {
		label: string;
		value: string;
		startIcon?: ReactNode;
	};
	token: string;
};

const DEFAULT_SUGGESTIONS_PER_CATEGORY = 5;
const DEFAULT_SUGGESTIONS_TOTAL = 15;

/** Matching `key:value` options across categories for free-text typeahead. */
export const collectValueSuggestions = (
	query: string,
	categories: readonly CategoryMatchSource[],
	optionsByKey: ReadonlyMap<string, readonly FilterOption[]>,
	selectedTokens: readonly string[],
	limits?: Readonly<{ perCategory?: number; total?: number }>,
): CategoryValueSuggestion[] => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return [];
	}

	const perCategory = limits?.perCategory ?? DEFAULT_SUGGESTIONS_PER_CATEGORY;
	const total = limits?.total ?? DEFAULT_SUGGESTIONS_TOTAL;
	const selected = new Set(selectedTokens);
	const suggestions: CategoryValueSuggestion[] = [];

	for (const category of categories) {
		const options = optionsByKey.get(category.key);
		if (!options || suggestions.length >= total) {
			continue;
		}

		let taken = 0;
		for (const option of options) {
			if (taken >= perCategory || suggestions.length >= total) {
				break;
			}

			const token = chipToken(category.key, option.value);
			if (selected.has(token)) {
				continue;
			}

			if (
				!option.label.toLowerCase().includes(normalized) &&
				!option.value.toLowerCase().includes(normalized)
			) {
				continue;
			}

			suggestions.push({
				categoryKey: category.key,
				categoryLabel: category.label,
				option,
				token,
			});
			taken += 1;
		}
	}

	return suggestions;
};
