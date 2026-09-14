import type { FC } from "react";
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

// Runtime provider types recorded on interceptions. Configured provider
// types collapse into these: azure, google, openai-compat, openrouter and
// vercel route through openai; bedrock routes through anthropic. Matching
// the session rows, which display the same values, means no provider
// configuration lookup (owner-only) is needed to populate the filter.
const PROVIDERS = ["anthropic", "openai", "copilot"] as const;

const toFilterOption = (provider: string): SelectFilterOption => ({
	value: provider,
	label: getProviderDisplayName(provider),
	startIcon: (
		<AIBridgeProviderIcon provider={provider} className="size-icon-sm" />
	),
});

const providerOptions = PROVIDERS.map(toFilterOption);

export const useProviderFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "provider",
		getSelectedOption: async () => {
			if (!value) {
				return null;
			}
			return providerOptions.find((option) => option.value === value) ?? null;
		},
		getOptions: async () => providerOptions,
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
		/>
	);
};
