import { useMemo } from "react";
import uFuzzy from "ufuzzy";

const fuzzyFinder = new uFuzzy({
	intraMode: 1,
	intraIns: 1,
	intraSub: 1,
	intraTrn: 1,
	intraDel: 1,
});

type UseFuzzySearchOptions<T> = {
	allItems: Readonly<Array<T>>;
	searchText: string;

	/**
	 * The item properties to fuzzy search against
	 */
	searchProperties: Array<keyof T>;
};

export function useFuzzySearch<T>({
	allItems,
	searchText,
	searchProperties,
}: UseFuzzySearchOptions<T>) {
	const query = searchText.trim();

	const searchedItems = useMemo(() => {
		if (!query) {
			return allItems;
		}

		// Search several string fields by concatenating them together
		// https://github.com/leeoniya/uFuzzy/issues/7
		const allItemsAsStrings = allItems.map((item) =>
			searchProperties.map((p) => item[p]).join("|"),
		);

		const [map, info, sorted] = fuzzyFinder.search(allItemsAsStrings, query);

		// We hit an invalid state somehow
		if (!map || !info || !sorted) {
			return [];
		}

		return sorted.map((i) => allItems[info.idx[i]]);
	}, [allItems, query, searchProperties]);

	return searchedItems;
}
