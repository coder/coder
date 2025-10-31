import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "components/Filter/SelectFilter";

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

export const ProviderFilter = ({ menu }: ProviderFilterProps) => {
	return (
		<SelectFilter
			label={"Select provider"}
			placeholder={"All providers"}
			emptyText="No providers found"
			options={menu.searchOptions}
			onSelect={(option: SelectFilterOption | undefined): void =>
				menu.selectOption(option)
			}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};
