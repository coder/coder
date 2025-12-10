import { API } from "api/api";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "components/Filter/SelectFilter";
import type { FC } from "react";

const AIBRIDGE_PROVIDERS: SelectFilterOption[] = [
	{
		label: "OpenAI",
		value: "openai",
	},
	{
		label: "Anthropic",
		value: "anthropic",
	},
];

export const useProviderFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "provider",
		getSelectedOption: async () =>
			AIBRIDGE_PROVIDERS.find((option) => option.value === value) ?? null,
		getOptions: async () => {
			return AIBRIDGE_PROVIDERS;
		},
		value,
		onChange,
		enabled,
	});
};

export type ProviderFilterMenu = ReturnType<typeof useProviderFilterMenu>;

interface ProviderFilterProps {
	menu: ProviderFilterMenu;
}

export const ProviderFilter: FC<ProviderFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label={"Select provider"}
			placeholder={"All providers"}
			emptyText="No providers found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};

interface UseModelFilterMenuOptions
	extends Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled"> {
	provider: string | undefined;
}

export const useModelFilterMenu = ({
	value,
	onChange,
	enabled,
	provider,
}: UseModelFilterMenuOptions) => {
	// Fetch models from API, optionally filtered by provider.
	const fetchModels = async (): Promise<SelectFilterOption[]> => {
		try {
			const response = await API.experimental.getAIBridgeModels(provider);
			return response.models.map((model) => ({
				label: model,
				value: model,
			}));
		} catch {
			return [];
		}
	};

	return useFilterMenu({
		id: "model",
		getSelectedOption: async () => {
			if (!value) {
				return null;
			}
			// Return the selected value directly without fetching all models.
			return { label: value, value };
		},
		getOptions: fetchModels,
		value,
		onChange,
		enabled,
	});
};

export type ModelFilterMenu = ReturnType<typeof useModelFilterMenu>;

interface ModelFilterProps {
	menu: ModelFilterMenu;
}

export const ModelFilter: FC<ModelFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label={"Select model"}
			placeholder={"All models"}
			emptyText="No models found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};
