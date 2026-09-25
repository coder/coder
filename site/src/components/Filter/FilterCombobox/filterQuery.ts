import type { ReactNode } from "react";
import {
	FILTER_TOKEN_RE,
	needsQuotes,
	parseFilterTokens,
} from "#/components/Filter/filterQuery";
import {
	categoryChipKeys,
	type FilterCategory,
	type FilterOption,
} from "./types";

export const chipToken = (key: string, value: string) => `${key}:${value}`;

/** Token an option commits under `key`: its explicit token, or `key:value`. */
export const optionToken = (
	key: string,
	option: Pick<FilterOption, "token" | "value">,
) => option.token ?? chipToken(key, option.value);

type ChipDisplaySource = Pick<
	FilterCategory,
	"key" | "chipKeys" | "scopeToggle"
>;

/**
 * Key and value to display for a chip token. Tokens owned by a multi-key
 * category (`outdated:true` under Attributes) display under the category key
 * (`attribute:outdated`). A token under the category's `scopeToggle.widenedKey`
 * keeps its value and displays under the category key (`user:bob` displays as
 * `owner:bob`). The query string itself is unchanged.
 */
export const chipDisplay = (
	token: string,
	categories: readonly ChipDisplaySource[],
): { key: string; value: string } => {
	const separatorIndex = token.indexOf(":");
	if (separatorIndex <= 0) {
		return { key: "", value: token };
	}
	const key = token.slice(0, separatorIndex);
	const value = token.slice(separatorIndex + 1);
	const owner = categories.find(
		(category) =>
			category.key !== key.toLowerCase() &&
			categoryChipKeys(category).includes(key.toLowerCase()),
	);
	if (owner?.scopeToggle?.widenedKey === key.toLowerCase()) {
		return { key: owner.key, value };
	}
	if (owner) {
		return { key: owner.key, value: key.toLowerCase() };
	}
	return { key, value };
};

// Collapses a stream of key/value pairs to one chip per group, keeping each
// group's first-seen position and its last pair, key and value alike. Shared by
// `queryToChips` (pairs from a query string) and `dedupeChips` (pairs from
// existing tokens).
const dedupeInOrder = (
	pairs: Iterable<{ key: string; value: string }>,
	groupKey: (key: string) => string = (key) => key,
): string[] => {
	const byGroup = new Map<string, { key: string; value: string }>();
	for (const pair of pairs) {
		byGroup.set(groupKey(pair.key), pair);
	}
	return [...byGroup.values()].map(({ key, value }) => chipToken(key, value));
};

// The key of the scope-toggle category that owns `key`, or undefined when no
// such category owns it.
const scopeCategoryKey = (
	key: string,
	categories: readonly ChipDisplaySource[],
) =>
	categories.find(
		(category) =>
			category.scopeToggle && categoryChipKeys(category).includes(key),
	)?.key;

// Keys of one scope-toggle category (`owner` and `user`) form one group;
// every other key is its own group.
const scopeGroupKey = (key: string, categories: readonly ChipDisplaySource[]) =>
	scopeCategoryKey(key, categories) ?? key;

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
 * appear. One chip per key; if a key repeats, the last value wins but the chip
 * keeps its first-seen position. The keys of a scope-toggle category in
 * `categories` share one chip, taken from the category's first token; its
 * other tokens stay in `extractFreeText` so the query keeps applying them.
 */
export const queryToChips = (
	query: string,
	chipKeys: readonly string[],
	categories: readonly ChipDisplaySource[] = [],
): string[] => {
	const pairs: { key: string; value: string }[] = [];
	const seenScopeCategories = new Set<string>();
	for (const { key, value } of parseFilterTokens(query)) {
		const normalizedKey = key.toLowerCase();
		if (!chipKeys.includes(normalizedKey)) {
			continue;
		}
		const scopeKey = scopeCategoryKey(normalizedKey, categories);
		if (scopeKey !== undefined) {
			if (seenScopeCategories.has(scopeKey)) {
				continue;
			}
			seenScopeCategories.add(scopeKey);
		}
		pairs.push({ key: normalizedKey, value });
	}
	return dedupeInOrder(pairs);
};

/**
 * De-duplicates chip tokens, preserving each group's first-seen position and
 * taking its last token. Each key is its own group, except that the keys of a
 * scope-toggle category in `categories` form one.
 */
export const dedupeChips = (
	tokens: readonly string[],
	chipKeys: readonly string[],
	categories: readonly ChipDisplaySource[] = [],
): string[] => {
	const pairs: { key: string; value: string }[] = [];
	for (const token of tokens) {
		const parsed = parseChipToken(token, chipKeys);
		if (parsed) {
			pairs.push(parsed);
		}
	}
	return dedupeInOrder(pairs, (key) => scopeGroupKey(key, categories));
};

/**
 * Everything that is not a recognized chip token, preserved verbatim.
 *
 * Only `key:value` tokens whose key is a known chip category are stripped; bare
 * words and unrecognized `key:value` tokens (documented backend filters such as
 * `dormant:true` or `has-agent:connected`) are carried through unchanged so the
 * query round-trips instead of being silently dropped or corrupted. A
 * scope-toggle category's tokens after its first are kept too, matching
 * `queryToChips`.
 */
export const extractFreeText = (
	query: string,
	chipKeys: readonly string[],
	categories: readonly ChipDisplaySource[] = [],
): string => {
	const seenScopeCategories = new Set<string>();
	return query
		.replace(FILTER_TOKEN_RE, (match, quotedKey, _quoted, bareKey) => {
			const key = (quotedKey ?? bareKey)?.toLowerCase();
			if (!key || !chipKeys.includes(key)) {
				return match;
			}
			const scopeKey = scopeCategoryKey(key, categories);
			if (scopeKey !== undefined) {
				if (seenScopeCategories.has(scopeKey)) {
					return match;
				}
				seenScopeCategories.add(scopeKey);
			}
			return " ";
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
	categories: readonly ChipDisplaySource[] = [],
): string => {
	const parts = dedupeChips(tokens, chipKeys, categories).map((token) => {
		const separatorIndex = token.indexOf(":");
		const key = token.slice(0, separatorIndex);
		const value = token.slice(separatorIndex + 1);
		return needsQuotes(value) ? `${key}:"${value}"` : `${key}:${value}`;
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
	/** Query key options commit under when it differs from `key`. */
	chipKey?: string;
};

export const parseTypedCategoryPrefix = (
	raw: string,
	categories: readonly CategoryMatchSource[],
): {
	categoryKey: string;
	query: string;
	freeText: string;
	typedKey: string;
} | null => {
	const resolveCategory = (typedKey: string) =>
		categories.find((entry) => {
			if (entry.key === typedKey || entry.label.toLowerCase() === typedKey) {
				return true;
			}
			return (
				entry.aliases?.some((alias) => alias.toLowerCase() === typedKey) ??
				false
			);
		});

	// Scan every `key:` fragment and keep the last one that resolves to a
	// category: that is the prefix the user is actively typing. Anchoring to the
	// tail (rather than the first lazy match) means prior non-category tokens
	// like `has-agent:connected` become free text instead of aborting the parse.
	// `[\w-]+` keeps hyphenated keys consistent with the rest of this module.
	let chosen: {
		category: CategoryMatchSource;
		typedKey: string;
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
			chosen = { category, typedKey, index, end: index + match[0].length };
		}
	}
	if (!chosen) {
		return null;
	}

	return {
		categoryKey: chosen.category.key,
		typedKey: chosen.typedKey,
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
	selected: boolean;
	token: string;
};

// `normalized` is trimmed and lowercased.
const optionMatches = (
	option: Pick<FilterOption, "label" | "value">,
	normalized: string,
) =>
	option.label.toLowerCase().includes(normalized) ||
	option.value.toLowerCase().includes(normalized);

/** Options whose label or value contains `text`, ignoring case. */
export const filterOptionsByText = (
	options: readonly FilterOption[],
	text: string,
): readonly FilterOption[] => {
	const normalized = text.trim().toLowerCase();
	if (normalized.length === 0) {
		return options;
	}
	return options.filter((option) => optionMatches(option, normalized));
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

			const token = optionToken(category.chipKey ?? category.key, option);

			if (!optionMatches(option, normalized)) {
				continue;
			}

			suggestions.push({
				categoryKey: category.key,
				categoryLabel: category.label,
				option,
				selected: selected.has(token),
				token,
			});
			taken += 1;
		}
	}

	return suggestions;
};
