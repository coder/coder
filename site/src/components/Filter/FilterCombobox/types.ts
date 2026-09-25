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
	 * `outdated:true` or `dormant:true`.
	 */
	token?: string;
};

export type FilterCategory = {
	key: string;
	label: string;
	getOptions: (query: string) => Promise<FilterOption[]>;
	icon?: ReactNode;
	/** Extra typed prefixes that enter this category. */
	aliases?: readonly string[];
	/**
	 * Query keys this category owns for chip parsing. Defaults to `[key]`. A
	 * category that commits distinct boolean keys (e.g. Attributes committing
	 * `outdated` and `dormant`) lists them all so the query round-trips them as
	 * chips instead of free text.
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
	/**
	 * Switch shown below the category's options. It is disabled until the
	 * category has a chip, and each pick turns it on: options commit under
	 * `widenedKey` instead of the category key, e.g. Owner committing
	 * `user:alice` (owned by or shared with alice) instead of `owner:alice`.
	 * While it is on, a pill after the chip reads `pillPrefix` and the chip's
	 * value, and removing the pill turns the switch off. Applies only to
	 * submenu categories, not inline ones.
	 */
	scopeToggle?: {
		/** Switch label for the category's applied value, if there is one. */
		label: (value: string | undefined) => string;
		widenedKey: string;
		/** Pill text before the applied value, e.g. `+ shared with`. */
		pillPrefix: string;
		/** Accessible name of the pill's remove button for the applied value. */
		pillRemoveLabel: (value: string) => string;
		/**
		 * A 3+ character prefix of this phrase lists the category and opens its
		 * flyout.
		 */
		searchPhrase: string;
	};
};

export const categoryChipKeys = (
	category: Pick<FilterCategory, "key" | "chipKeys" | "scopeToggle">,
): readonly string[] => [
	...new Set([
		...(category.chipKeys ?? [category.key]),
		...(category.scopeToggle ? [category.scopeToggle.widenedKey] : []),
	]),
];
