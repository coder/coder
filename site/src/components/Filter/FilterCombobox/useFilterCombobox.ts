import { useMemo, useRef, useState } from "react";
import type { UseFilterResult } from "#/components/Filter/Filter";
import type { SelectFilterOption } from "#/components/Filter/SelectFilter";
import {
	chipsToValues,
	chipToken,
	composeFilterQuery,
	extractFreeText,
	filterValuesToChips,
	parseTypedFacetPrefix,
} from "./filterQuery";
import type { FilterFacet } from "./types";

type UseFilterComboboxOptions<Id extends string> = {
	filter: UseFilterResult;
	facets: readonly FilterFacet<Id>[];
	/** Stable chip key order for serialization. Defaults to facet ids. */
	chipKeys?: readonly Id[];
};

export const useFilterCombobox = <Id extends string>({
	filter,
	facets,
	chipKeys: chipKeysProp,
}: UseFilterComboboxOptions<Id>) => {
	const chipKeys = chipKeysProp ?? facets.map((facet) => facet.id);
	const [open, setOpen] = useState(false);
	const [activeFacet, setActiveFacet] = useState<Id | null>(null);
	const [inputValue, setInputValue] = useState(() =>
		extractFreeText(filter.query),
	);
	const [committedFreeText, setCommittedFreeText] = useState(() =>
		extractFreeText(filter.query),
	);
	const committedFreeTextRef = useRef(committedFreeText);
	const facetModeRef = useRef(false);

	const setCommittedNameSearch = (value: string) => {
		const next = value.trim();
		committedFreeTextRef.current = next;
		setCommittedFreeText(next);
	};

	const activeFacetMeta = facets.find((facet) => facet.id === activeFacet);
	const activeOptions = activeFacetMeta?.menu.searchOptions;
	const chipValues = filterValuesToChips(filter.values, chipKeys);

	const optionItems = useMemo(() => {
		if (!activeFacet || !activeOptions) {
			return [] as string[];
		}
		return activeOptions.map((option) => chipToken(activeFacet, option.value));
	}, [activeFacet, activeOptions]);

	const optionByToken = useMemo(() => {
		const map = new Map<string, SelectFilterOption>();
		if (!activeFacet || !activeOptions) {
			return map;
		}
		for (const option of activeOptions) {
			map.set(chipToken(activeFacet, option.value), option);
		}
		return map;
	}, [activeFacet, activeOptions]);

	const restoreFreeTextInput = () => {
		setInputValue(committedFreeTextRef.current);
	};

	const exitFacetMode = () => {
		facetModeRef.current = false;
		setActiveFacet(null);
		restoreFreeTextInput();
	};

	const enterFacetMode = (facetId: Id, query = "") => {
		facetModeRef.current = true;
		setActiveFacet(facetId);
		setInputValue(query);
		setOpen(true);
		const facet = facets.find((entry) => entry.id === facetId);
		facet?.menu.setQuery(query);
	};

	const updateFromChips = (tokens: string[]) => {
		filter.cancelDebounce();
		filter.update(
			composeFilterQuery(
				chipsToValues(tokens, chipKeys),
				chipKeys,
				committedFreeTextRef.current,
			),
		);
	};

	const selectFacet = (facetId: Id) => {
		filter.cancelDebounce();
		if (activeFacet === null) {
			setCommittedNameSearch(inputValue);
			filter.update(composeFilterQuery(filter.values, chipKeys, inputValue));
		}
		enterFacetMode(facetId);
	};

	const handleInputValueChange = (
		nextValue: string,
		eventDetails: { reason: string },
	) => {
		if (eventDetails.reason === "input-clear") {
			if (activeFacet === null) {
				restoreFreeTextInput();
				return;
			}
			setInputValue("");
			return;
		}

		const typedFacet = parseTypedFacetPrefix(nextValue, facets);
		if (typedFacet) {
			filter.cancelDebounce();
			setCommittedNameSearch(typedFacet.freeText);
			filter.update(
				composeFilterQuery(filter.values, chipKeys, typedFacet.freeText),
			);
			enterFacetMode(typedFacet.facetId, typedFacet.query);
			return;
		}

		if (activeFacetMeta) {
			setInputValue(nextValue);
			activeFacetMeta.menu.setQuery(nextValue);
			facetModeRef.current = true;
			setOpen(true);
			return;
		}

		setCommittedNameSearch(nextValue);
		setInputValue(nextValue);
		filter.debounceUpdate(
			composeFilterQuery(filter.values, chipKeys, nextValue),
		);
	};

	const handleOpenChange = (
		nextOpen: boolean,
		eventDetails: { reason: string },
	) => {
		if (
			nextOpen &&
			!facetModeRef.current &&
			eventDetails.reason !== "trigger-press"
		) {
			return;
		}

		setOpen(nextOpen);
		if (!nextOpen) {
			exitFacetMode();
		} else if (eventDetails.reason === "trigger-press") {
			facetModeRef.current = false;
			setActiveFacet(null);
		}
	};

	const handleValueChange = (nextTokens: string[]) => {
		updateFromChips(nextTokens);
		facetModeRef.current = false;
		setActiveFacet(null);
		setInputValue(committedFreeTextRef.current);
		setOpen(false);
	};

	const exitActiveFacet = () => {
		activeFacetMeta?.menu.setQuery("");
		facetModeRef.current = false;
		setActiveFacet(null);
		restoreFreeTextInput();
		setOpen(false);
	};

	return {
		open,
		inputValue,
		committedFreeText,
		activeFacet,
		activeFacetMeta,
		activeOptions,
		chipValues,
		optionItems,
		optionByToken,
		selectFacet,
		exitActiveFacet,
		handleInputValueChange,
		handleOpenChange,
		handleValueChange,
	};
};
