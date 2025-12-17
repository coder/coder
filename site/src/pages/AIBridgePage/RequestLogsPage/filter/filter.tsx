import { ExternalImage } from "components/ExternalImage/ExternalImage";
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
		startIcon: (
			<ExternalImage src="/icon/openai.svg" className="size-icon-sm" />
		),
	},
	{
		label: "Anthropic",
		value: "anthropic",
		startIcon: (
			<ExternalImage src="/icon/claude-device.svg" className="size-icon-sm" />
		),
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
