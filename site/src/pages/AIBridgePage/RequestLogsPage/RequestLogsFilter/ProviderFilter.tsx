import type { FC } from "react";
import { API } from "#/api/api";
import type { AIProviderType } from "#/api/typesGenerated";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import { AIBridgeProviderIcon } from "../icons/AIBridgeProviderIcon";

const toOption = (provider: {
	id: string;
	name: string;
	display_name: string;
	type: AIProviderType;
}): SelectFilterOption => ({
	label: provider.display_name || provider.name,
	value: provider.id,
	startIcon: (
		<AIBridgeProviderIcon provider={provider.type} className="size-icon-sm" />
	),
});

export const useProviderFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "provider",
		value,
		onChange,
		enabled,
		getSelectedOption: async () => {
			if (!value) {
				return null;
			}
			const providers = await API.experimental.listAIProviders();
			const match = providers.find((p) => p.id === value);
			return match ? toOption(match) : null;
		},
		getOptions: async () => {
			const providers = await API.experimental.listAIProviders();
			return providers.map(toOption);
		},
	});
};

export type ProviderFilterMenu = ReturnType<typeof useProviderFilterMenu>;

interface ProviderFilterProps {
	menu: ProviderFilterMenu;
}

export const ProviderFilter: FC<ProviderFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label="Select provider"
			placeholder="All providers"
			emptyText="No providers found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};
