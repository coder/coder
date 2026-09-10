import type { FC } from "react";
import { API } from "#/api/api";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import { AIBridgeProviderIcon } from "../icons/AIBridgeProviderIcon";
import { getProviderDisplayName } from "../utils";

const toFilterOption = (providerName: string): SelectFilterOption => ({
	value: providerName,
	label: getProviderDisplayName(providerName),
	startIcon: (
		<AIBridgeProviderIcon provider={providerName} className="size-icon-sm" />
	),
});

export const useProviderFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "provider_name",
		getSelectedOption: async () => {
			if (!value) {
				return null;
			}
			const providers = await API.getAIBridgeProviders({
				q: value,
				limit: 1,
			});
			const firstProvider = providers.at(0);
			if (firstProvider && firstProvider === value) {
				return toFilterOption(firstProvider);
			}
			return null;
		},
		getOptions: async (query) => {
			const providers = await API.getAIBridgeProviders({
				q: query,
				limit: 25,
			});
			return providers.map(toFilterOption);
		},
		value,
		onChange,
		enabled,
	});
};

export type ProviderFilterMenu = ReturnType<typeof useProviderFilterMenu>;

interface ProviderFilterProps {
	menu: ProviderFilterMenu;
	width?: number;
}

export const ProviderFilter: FC<ProviderFilterProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Select provider"
			placeholder="All providers"
			emptyText="No providers found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
			selectFilterSearch={
				<ComboboxInput
					placeholder="Search provider..."
					value={menu.query}
					onValueChange={menu.setQuery}
				/>
			}
		/>
	);
};
