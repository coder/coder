import { API } from "api/api";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import {
	SelectFilter,
	SelectFilterSearch,
} from "components/Filter/SelectFilter";
import type { FC } from "react";

export const useModelFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "model",
		getSelectedOption: async () => {
			const modelsRes = await API.experimental.getAIBridgeModels({
				q: value,
				limit: 1,
			});
			const firstModel = modelsRes.at(0);

			if (firstModel) {
				return {
					label: firstModel,
					value: firstModel,
				};
			}

			return null;
		},
		getOptions: async (query) => {
			const modelsRes = await API.experimental.getAIBridgeModels({
				q: query,
				limit: 25,
			});
			return modelsRes.map((model) => ({
				label: model,
				value: model,
			}));
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
			selectFilterSearch={
				<SelectFilterSearch
					inputProps={{ "aria-label": "Search model" }}
					placeholder="Search model..."
					value={menu.query}
					onChange={menu.setQuery}
				/>
			}
		/>
	);
};
