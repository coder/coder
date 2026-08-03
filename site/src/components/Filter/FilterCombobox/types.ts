import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { SelectFilterOption } from "#/components/Filter/SelectFilter";

export type FilterFacetMenu = {
	searchOptions: SelectFilterOption[] | undefined;
	setQuery: (query: string) => void;
};

export type FilterFacet<Id extends string = string> = {
	id: Id;
	label: string;
	icon: LucideIcon;
	/** Extra typed prefixes that enter this facet, e.g. `user` for `owner`. */
	aliases?: readonly string[];
	menu: FilterFacetMenu;
};

/** Live resource preview row shown while typing free-text search. */
export type FilterSearchResult = {
	id: string;
	label: string;
	subtitle?: string;
	startIcon?: ReactNode;
	/** Renders an avatar when `startIcon` is not provided. */
	imageUrl?: string;
	/** Opaque payload for `onSearchResultSelect`, e.g. a workspace URL path. */
	href?: string;
};

export const SEARCH_RESULT_TOKEN_PREFIX = "__search:";

export const searchResultToken = (id: string) =>
	`${SEARCH_RESULT_TOKEN_PREFIX}${id}`;

export const parseSearchResultToken = (token: string): string | null => {
	if (!token.startsWith(SEARCH_RESULT_TOKEN_PREFIX)) {
		return null;
	}
	return token.slice(SEARCH_RESULT_TOKEN_PREFIX.length);
};
