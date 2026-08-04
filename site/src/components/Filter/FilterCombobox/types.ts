import type { ReactNode } from "react";

export type FilterOption = {
	label: string;
	value: string;
	startIcon?: ReactNode;
	subtitle?: string;
	/**
	 * Explicit chip token committed when this option is selected, overriding the
	 * default `${categoryKey}:${value}`. Used by categories that group several
	 * query keys, e.g. an "Attributes" category whose options commit
	 * `outdated:true`, `dormant:true`, or `shared:true`.
	 */
	token?: string;
};

export type FilterCategory = {
	key: string;
	label: string;
	getOptions: (query: string) => Promise<FilterOption[]>;
	icon?: ReactNode;
	/** Extra typed prefixes that enter this category, e.g. `user` for `owner`. */
	aliases?: readonly string[];
	/**
	 * Query keys this category owns for chip parsing. Defaults to `[key]`. A
	 * category that commits several distinct boolean keys (e.g. Attributes
	 * committing `outdated`, `dormant`, `shared`) lists them all so the query
	 * round-trips them as chips instead of free text.
	 */
	chipKeys?: readonly string[];
};

/** Live resource preview row shown while typing free-text search. */
export type SearchResult = {
	label: string;
	value: string;
	startIcon?: ReactNode;
	subtitle?: string;
	/** Renders an avatar when `startIcon` is not provided. */
	imageUrl?: string;
	/** Opaque payload for `onSearchResultSelect`, e.g. a workspace URL path. */
	href?: string;
};

const SEARCH_RESULT_TOKEN_PREFIX = "__search:";

export const searchResultToken = (value: string) =>
	`${SEARCH_RESULT_TOKEN_PREFIX}${value}`;

export const parseSearchResultToken = (token: string): string | null => {
	if (!token.startsWith(SEARCH_RESULT_TOKEN_PREFIX)) {
		return null;
	}
	return token.slice(SEARCH_RESULT_TOKEN_PREFIX.length);
};
