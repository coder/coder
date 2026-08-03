import type { LucideIcon } from "lucide-react";
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
