import type { ReactNode } from "react";

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

export const dedupeChipsByFacet = (
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

export const parseTypedFacetPrefix = <Id extends string>(
	raw: string,
	facets: readonly {
		id: Id;
		label: string;
		aliases?: readonly string[];
	}[],
): { facetId: Id; query: string; freeText: string } | null => {
	// Allow `owner:` at the start, or after name search text: `pink owner:`.
	const match = /^(.*?)\s*(\w+)\s*:(.*)$/.exec(raw);
	if (!match) {
		return null;
	}

	const typedKey = match[2]?.toLowerCase();
	if (!typedKey) {
		return null;
	}

	const facet = facets.find((entry) => {
		if (entry.id === typedKey || entry.label.toLowerCase() === typedKey) {
			return true;
		}
		return entry.aliases?.some((alias) => alias.toLowerCase() === typedKey);
	});
	if (!facet) {
		return null;
	}

	return {
		facetId: facet.id,
		query: match[3] ?? "",
		freeText: (match[1] ?? "").trim(),
	};
};

type FacetMatchSource<Id extends string> = {
	id: Id;
	label: string;
	aliases?: readonly string[];
};

/** Categories whose id, label, or alias starts with the typed query. */
export const matchFacets = <Id extends string, T extends FacetMatchSource<Id>>(
	query: string,
	facets: readonly T[],
): T[] => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return [];
	}

	return facets.filter((facet) => {
		if (facet.id.toLowerCase().startsWith(normalized)) {
			return true;
		}
		if (facet.label.toLowerCase().startsWith(normalized)) {
			return true;
		}
		return (
			facet.aliases?.some((alias) =>
				alias.toLowerCase().startsWith(normalized),
			) ?? false
		);
	});
};

export type FacetValueSuggestion<Id extends string = string> = {
	facetId: Id;
	facetLabel: string;
	option: {
		label: string;
		value: string;
		startIcon?: ReactNode;
	};
	token: string;
};

const DEFAULT_SUGGESTIONS_PER_FACET = 5;
const DEFAULT_SUGGESTIONS_TOTAL = 15;

/** Matching `key:value` options across facets for free-text typeahead. */
export const collectValueSuggestions = <Id extends string>(
	query: string,
	facets: readonly {
		id: Id;
		label: string;
		menu: {
			searchOptions:
				| readonly {
						label: string;
						value: string;
						startIcon?: ReactNode;
				  }[]
				| undefined;
		};
	}[],
	selectedTokens: readonly string[],
	limits?: Readonly<{ perFacet?: number; total?: number }>,
): FacetValueSuggestion<Id>[] => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return [];
	}

	const perFacet = limits?.perFacet ?? DEFAULT_SUGGESTIONS_PER_FACET;
	const total = limits?.total ?? DEFAULT_SUGGESTIONS_TOTAL;
	const selected = new Set(selectedTokens);
	const suggestions: FacetValueSuggestion<Id>[] = [];

	for (const facet of facets) {
		const options = facet.menu.searchOptions;
		if (!options || suggestions.length >= total) {
			continue;
		}

		let taken = 0;
		for (const option of options) {
			if (taken >= perFacet || suggestions.length >= total) {
				break;
			}

			const token = chipToken(facet.id, option.value);
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
				facetId: facet.id,
				facetLabel: facet.label,
				option,
				token,
			});
			taken += 1;
		}
	}

	return suggestions;
};
