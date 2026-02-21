import { API } from "api/api";
import { ComboboxInput } from "components/Combobox/Combobox";
import type { SelectFilterOption } from "components/Filter/SelectFilter";
import { SelectFilter } from "components/Filter/SelectFilter";
import { useState } from "react";
import { useQuery } from "react-query";
import { AIBridgeClientIcon } from "../icons/AIBridgeClientIcon";

type UseClientFilterMenuOptions = {
	value: string | undefined;
	onChange: (value: string | undefined) => void;
	enabled?: boolean;
};

export const useClientFilterMenu = ({
	value,
	onChange,
	enabled,
}: UseClientFilterMenuOptions) => {
	const [query, setQuery] = useState("");

	const selectedClients = value
		? new Set(value.split(",").filter(Boolean))
		: new Set<string>();

	const searchOptionsQuery = useQuery({
		queryKey: ["client", "autocomplete", "search", query],
		queryFn: () => API.getAIBridgeClients({ q: query, limit: 25 }),
		enabled,
	});

	const searchOptions = searchOptionsQuery.data?.map((client) => ({
		label: client,
		value: client,
		startIcon: <AIBridgeClientIcon client={client} className="size-icon-sm" />,
	}));

	const toggleOption = (option: SelectFilterOption | undefined) => {
		if (!option) return;
		const next = new Set(selectedClients);
		if (next.has(option.value)) {
			next.delete(option.value);
		} else {
			next.add(option.value);
		}
		onChange(next.size > 0 ? [...next].join(",") : undefined);
	};

	return {
		query,
		setQuery,
		selectedClients,
		searchOptions,
		toggleOption,
		isInitializing: false,
		isSearching: searchOptionsQuery.isFetching,
	};
};

export type ClientFilterMenu = ReturnType<typeof useClientFilterMenu>;

interface ClientFilterProps {
	menu: ClientFilterMenu;
}

export const ClientFilter: React.FC<ClientFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label="Select client"
			placeholder="All clients"
			emptyText="No clients found"
			options={menu.searchOptions}
			onSelect={(option) => menu.toggleOption(option)}
			value={menu.selectedClients}
			selectFilterSearch={
				<ComboboxInput
					placeholder="Search client..."
					value={menu.query}
					onValueChange={menu.setQuery}
				/>
			}
		/>
	);
};
