import { API } from "api/api";
import { ComboboxInput } from "components/Combobox/Combobox";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import { SelectFilter } from "components/Filter/SelectFilter";
import { AIBridgeClientIcon } from "../icons/AIBridgeClientIcon";

export const useClientFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "client",
		getSelectedOption: async () => {
			const clientsRes = await API.getAIBridgeClients({
				q: value,
				limit: 1,
			});
			const firstClient = clientsRes.at(0);

			if (firstClient) {
				return {
					label: firstClient,
					value: firstClient,
					startIcon: (
						<AIBridgeClientIcon client={firstClient} className="size-icon-sm" />
					),
				};
			}

			return null;
		},
		getOptions: async (query) => {
			const clientsRes = await API.getAIBridgeClients({
				q: query,
				limit: 25,
			});
			return clientsRes.map((client) => ({
				label: client,
				value: client,
				startIcon: (
					<AIBridgeClientIcon client={client} className="size-icon-sm" />
				),
			}));
		},
		value,
		onChange,
		enabled,
	});
};

export type ClientFilterMenu = ReturnType<typeof useClientFilterMenu>;

interface ClientFilterProps {
	menu: ClientFilterMenu;
}

export const ClientFilter: React.FC<ClientFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label="Select client"
			placeholder="All clients"
			emptyText="No clients found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
			selectFilterSearch={
				<ComboboxInput
					placeholder="Search client..."
					value={menu.query}
					onValueChange={menu.setQuery}
				/>
			}
		/>
	);
};
