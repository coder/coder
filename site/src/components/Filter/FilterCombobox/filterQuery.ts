import type { ReactNode } from "react";
import type { FilterOption } from "./types";

const FILTER_TOKEN_RE = /([\w-]+):"([^"]+)"|([\w-]+):(\S+)/g;

export const chipToken = (key: string, value: string) => `${key}:${value}`;

// Collapses a stream of key/value pairs to one chip per key, keeping each key's
// first-seen position and its last-seen value. Shared by `queryToChips` (pairs
// from a query string) and `dedupeChips` (pairs from existing tokens).
const dedupeInOrder = (
	pairs: Iterable<{ key: string; value: string }>,
): string[] => {
	const order: string[] = [];
	const byKey = new Map<string, string>();
	for (const { key, value } of pairs) {
		if (!byKey.has(key)) {
			order.push(key);
		}
		byKey.set(key, value);
	}
	return order.map((key) => chipToken(key, byKey.get(key) as string));
};

export const parseChipToken = (
	token: string,
	chipKeys: readonly string[],
): { key: string; value: string } | null => {
	const separatorIndex = token.indexOf(":");
	if (separatorIndex <= 0) {
		return null;
	}

	// Chip keys are canonical lowercase; normalize so `Owner:me` round-trips the
	// same as `owner:me` (matches the case-insensitive typed-prefix matching).
	const key = token.slice(0, separatorIndex).toLowerCase();
	const value = token.slice(separatorIndex + 1);
	if (!chipKeys.includes(key) || value.length === 0) {
		return null;
	}

	return { key, value };
};

/**
 * Chip tokens from a query string for known chip categories, in the order they
 * appear. One chip per category; if a category repeats, the last value wins but
 * the chip keeps its first-seen position.
 */
export const queryToChips = (
	query: string,
	chipKeys: readonly string[],
): string[] => {
	const pairs: { key: string; value: string }[] = [];
	for (const match of query.matchAll(FILTER_TOKEN_RE)) {
		const key = (match[1] ?? match[3])?.toLowerCase();
		const value = match[2] ?? match[4];
		if (!key || !value || !chipKeys.includes(key)) {
			continue;
		}
		pairs.push({ key, value });
	}
	return dedupeInOrder(pairs);
};

/**
 * De-duplicates chip tokens by category, preserving the first-seen position of
 * each category and taking the last value provided for it.
 */
export const dedupeChips = (
	tokens: readonly string[],
	chipKeys: readonly string[],
): string[] => {
	const pairs: { key: string; value: string }[] = [];
	for (const token of tokens) {
		const parsed = parseChipToken(token, chipKeys);
		if (parsed) {
			pairs.push(parsed);
		}
	}
	return dedupeInOrder(pairs);
};

/**
 * Everything that is not a recognized chip token, preserved verbatim.
 *
 * Only `key:value` tokens whose key is a known chip category are stripped; bare
 * words and unrecognized `key:value` tokens (documented backend filters such as
 * `dormant:true` or `has-agent:connected`) are carried through unchanged so the
 * query round-trips instead of being silently dropped or corrupted.
 */
export const extractFreeText = (
	query: string,
	chipKeys: readonly string[],
): string => {
	return query
		.replace(FILTER_TOKEN_RE, (match, quotedKey, _quoted, bareKey) => {
			const key = (quotedKey ?? bareKey)?.toLowerCase();
			return key && chipKeys.includes(key) ? " " : match;
		})
		.replace(/\s+/g, " ")
		.trim();
};

/**
 * Serializes committed chip tokens plus trailing free text back into a single
 * query string. Round-trip partner of `queryToChips` / `extractFreeText`.
 */
export const composeFilterQuery = (
	tokens: readonly string[],
	chipKeys: readonly string[],
	freeText: string,
): string => {
	const parts = dedupeChips(tokens, chipKeys).map((token) => {
		const separatorIndex = token.indexOf(":");
		const key = token.slice(0, separatorIndex);
		const value = token.slice(separatorIndex + 1);
		return value.includes(" ") ? `${key}:"${value}"` : `${key}:${value}`;
	});

	const trimmedFreeText = freeText.trim();
	if (trimmedFreeText.length > 0) {
		parts.push(trimmedFreeText);
	}
	return parts.join(" ");
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
	const resolveCategory = (typedKey: string) =>
		categories.find((entry) => {
			if (entry.key === typedKey || entry.label.toLowerCase() === typedKey) {
				return true;
			}
			return entry.aliases?.some((alias) => alias.toLowerCase() === typedKey);
		});

	// Scan every `key:` fragment and keep the last one that resolves to a
	// category: that is the prefix the user is actively typing. Anchoring to the
	// tail (rather than the first lazy match) means prior non-category tokens
	// like `has-agent:connected` become free text instead of aborting the parse.
	// `[\w-]+` keeps hyphenated keys consistent with the rest of this module.
	let chosen: {
		category: CategoryMatchSource;
		index: number;
		end: number;
	} | null = null;
	for (const match of raw.matchAll(/([\w-]+)\s*:/g)) {
		const typedKey = match[1]?.toLowerCase();
		if (!typedKey) {
			continue;
		}
		const category = resolveCategory(typedKey);
		if (category) {
			const index = match.index ?? 0;
			chosen = { category, index, end: index + match[0].length };
		}
	}
	if (!chosen) {
		return null;
	}

	return {
		categoryKey: chosen.category.key,
		query: raw.slice(chosen.end),
		freeText: raw.slice(0, chosen.index).trim(),
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

			const token = option.token ?? chipToken(category.key, option.value);
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
