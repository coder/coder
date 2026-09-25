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
	 * Keep the category in the menu while it has at most one option. Such
	 * categories are left out by default, since filtering by them would not
	 * narrow the results. Does not apply to inline categories.
	 */
	showWhenSingleOption?: boolean;
	/**
	 * Switch shown below the category's options, on by default. While on,
	 * options commit under `chipKey` instead of the category key, e.g. Owner
	 * committing `user:alice` (owned by or shared with alice) instead of
	 * `owner:alice`. While the category has a chip, a pill after it shows
	 * `pillLabel` while the switch is on, and removing the pill turns it off.
	 * `chipKey` must also be listed in `chipKeys` so it parses as this
	 * category's chip.
	 */
	scopeToggle?: {
		/** Switch label for the category's applied value, if there is one. */
		label: (value: string | undefined) => string;
		chipKey: string;
		pillLabel: string;
	};
};
