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
import { AIBridgeModelIcon } from "../icons/AIBridgeModelIcon";

const toFilterOption = (model: string): SelectFilterOption => ({
	label: model,
	value: model,
	startIcon: <AIBridgeModelIcon model={model} className="size-icon-sm" />,
});

export const useModelFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "model",
		// The search matches substrings, so a request cannot single out the
		// selected model. Its identifier is all that the option shows.
		getSelectedOption: async () => (value ? toFilterOption(value) : null),
		getOptions: async (query) => {
			const modelsRes = await API.getAIBridgeModels({
				model: query,
				limit: 25,
			});
			return modelsRes.map(toFilterOption);
		},
		value,
		onChange,
		enabled,
	});
};

export type ModelFilterMenu = ReturnType<typeof useModelFilterMenu>;

type ModelFilterProps = {
	menu: ModelFilterMenu;
	width?: number;
};

export const ModelFilter: React.FC<ModelFilterProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Select model"
			placeholder="All models"
			emptyText="No models found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
			searchLabel="Search model"
			selectFilterSearch={
				<ComboboxInput
					placeholder="Search model..."
					value={menu.query}
					onValueChange={menu.setQuery}
				/>
			}
		/>
	);
};
