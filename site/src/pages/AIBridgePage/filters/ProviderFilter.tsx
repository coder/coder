import type { FC } from "react";
import { API } from "#/api/api";
import type { AIBridgeProvider } from "#/api/typesGenerated";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";

const toFilterOption = (provider: AIBridgeProvider): SelectFilterOption => ({
	value: provider.name,
	label: provider.display_name || provider.name,
	startIcon: <ProviderIcon provider={provider.type} icon={provider.icon} />,
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
			const providers = await API.getAIBridgeProviders();
			const match = providers.find((p) => p.name === value);
			return match ? toFilterOption(match) : null;
		},
		// The provider list is small and useFilterMenu filters options
		// client-side by label and value, so the query is not sent upstream.
		getOptions: async () => {
			const providers = await API.getAIBridgeProviders();
			return providers.map(toFilterOption);
		},
		value,
		onChange,
		enabled,
	});
};

export type ProviderFilterMenu = ReturnType<typeof useProviderFilterMenu>;

type ProviderFilterProps = {
	menu: ProviderFilterMenu;
	width?: number;
};

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
