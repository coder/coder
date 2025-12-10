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

// Model options organized by provider
const MODELS_BY_PROVIDER: Record<string, SelectFilterOption[]> = {
	openai: [
		{ label: "GPT-4o", value: "gpt-4o" },
		{ label: "GPT-4o Mini", value: "gpt-4o-mini" },
		{ label: "GPT-4 Turbo", value: "gpt-4-turbo" },
		{ label: "GPT-4", value: "gpt-4" },
		{ label: "GPT-3.5 Turbo", value: "gpt-3.5-turbo" },
		{ label: "o1", value: "o1" },
		{ label: "o1 Mini", value: "o1-mini" },
		{ label: "o1 Preview", value: "o1-preview" },
		{ label: "o3 Mini", value: "o3-mini" },
	],
	anthropic: [
		{ label: "Claude Sonnet 4", value: "claude-sonnet-4-20250514" },
		{ label: "Claude 3.7 Sonnet", value: "claude-3-7-sonnet-20250219" },
		{ label: "Claude 3.5 Sonnet", value: "claude-3-5-sonnet-20241022" },
		{ label: "Claude 3.5 Haiku", value: "claude-3-5-haiku-20241022" },
		{ label: "Claude 3 Opus", value: "claude-3-opus-20240229" },
		{ label: "Claude 3 Sonnet", value: "claude-3-sonnet-20240229" },
		{ label: "Claude 3 Haiku", value: "claude-3-haiku-20240307" },
	],
};

// Get all models as a flat list
const ALL_MODELS: SelectFilterOption[] = Object.values(MODELS_BY_PROVIDER).flat();

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
	// Get models based on selected provider, or all models if none selected
	const getModelsForProvider = (): SelectFilterOption[] => {
		if (!provider) {
			return ALL_MODELS;
		}
		return MODELS_BY_PROVIDER[provider] ?? [];
	};

	return useFilterMenu({
		id: "model",
		getSelectedOption: async () => {
			const models = getModelsForProvider();
			return models.find((option) => option.value === value) ?? null;
		},
		getOptions: async () => {
			return getModelsForProvider();
		},
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
