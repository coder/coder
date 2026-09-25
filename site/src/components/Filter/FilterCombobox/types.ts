import type { ReactNode } from "react";

export type FilterOption = {
	label: string;
	/** Label used once the option is applied; defaults to `label`. */
	appliedLabel?: string;
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
	/** Render this category's options as top-level toggle rows instead of a submenu. */
	inlineOptions?: boolean;
	/** Heading shown above top-level options. Defaults to `${label} is…`. */
	inlineOptionsLabel?: string;
	/** Keep option icons when rendering the category as top-level rows. */
	inlineOptionsIcons?: boolean;
	/** Selecting an option replaces another selected option from this category. */
	inlineOptionsExclusive?: boolean;
	/** Applied chips show only the option label, without the category prefix. */
	chipLabelOnly?: boolean;
	/**
	 * Leave the category out of the menu while `getOptions("")` returns at most
	 * one option. When `getOptions("")` omits values the results can contain,
	 * the row can hide while its one option would still narrow the results. Its
	 * empty-query options are fetched when the filter renders. Until every
	 * category with this flag finishes its first load, successfully or not, the
	 * unnarrowed menu shows placeholder rows in place of all submenu rows. An
	 * applied chip or a failed lookup keeps it in the menu; a retry that returns
	 * at most one option removes it. Does not apply to inline categories.
	 */
	hideWhenSingleOption?: boolean;
};
