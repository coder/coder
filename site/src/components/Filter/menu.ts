import type { SelectFilterOption } from "components/Filter/SelectFilter";
import { useMemo, useRef, useState } from "react";
import { useQuery } from "react-query";

export type UseFilterMenuOptions = {
	id: string;
	value: string | undefined;
	// Using null because of react-query
	// https://tanstack.com/query/v4/docs/react/guides/migrating-to-react-query-4#undefined-is-an-illegal-cache-value-for-successful-queries
	getSelectedOption: () => Promise<SelectFilterOption | null>;
	getOptions: (query: string) => Promise<SelectFilterOption[]>;
	onChange: (option: SelectFilterOption | undefined) => void;
	enabled?: boolean;
};

export const useFilterMenu = ({
	id,
	value,
	getSelectedOption,
	getOptions,
	onChange,
	enabled,
}: UseFilterMenuOptions) => {
	const selectedOptionsCacheRef = useRef<Record<string, SelectFilterOption>>(
		{},
	);
	const [query, setQuery] = useState("");
	const selectedOptionQuery = useQuery({
		queryKey: [id, "autocomplete", "selected", value],
		queryFn: () => {
			if (!value) {
				return null;
			}

			const cachedOption = selectedOptionsCacheRef.current[value];
			if (cachedOption) {
				return cachedOption;
			}

			return getSelectedOption();
		},
		enabled,
		keepPreviousData: true,
	});
	const selectedOption = selectedOptionQuery.data;
	const searchOptionsQuery = useQuery({
		queryKey: [id, "autocomplete", "search", query],
		queryFn: () => getOptions(query),
		enabled,
	});
	const searchOptions = useMemo(() => {
		const isDataLoaded =
			searchOptionsQuery.isFetched && selectedOptionQuery.isFetched;

		if (!isDataLoaded) {
			return undefined;
		}

		let options = searchOptionsQuery.data ?? [];

		if (selectedOption) {
			options = options.filter(
				(option) => option.value !== selectedOption.value,
			);
			options = [selectedOption, ...options];
		}

		options = options.filter(
			(option) =>
				option.label.toLowerCase().includes(query.toLowerCase()) ||
				option.value.toLowerCase().includes(query.toLowerCase()),
		);

		return options;
	}, [
		selectedOptionQuery.isFetched,
		query,
		searchOptionsQuery.data,
		searchOptionsQuery.isFetched,
		selectedOption,
	]);

	const selectOption = (option: SelectFilterOption | undefined) => {
		if (option) {
			selectedOptionsCacheRef.current[option.value] = option;
		}

		setQuery("");
		onChange(option);
	};

	return {
		query,
		setQuery,
		selectedOption,
		selectOption,
		searchOptions,
		isInitializing: selectedOptionQuery.isInitialLoading,
		initialOption: selectedOptionQuery.data,
		isSearching: searchOptionsQuery.isFetching,
	};
};

export type UseFilterMenuResult = ReturnType<typeof useFilterMenu>;
