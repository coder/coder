import { ComboboxInput } from "components/Combobox/Combobox";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import { SelectFilter } from "components/Filter/SelectFilter";
import type { FC } from "react";
import {
	type AIBridgeClient,
	AIBridgeClientIcon,
} from "../../RequestLogsPage/icons/AIBridgeClientIcon";

const AIBRIDGE_CLIENTS: AIBridgeClient[] = [
	"Claude Code",
	"Codex",
	"Kilo Code",
	"Roo Code",
	"Mux",
	"Zed",
	"Cursor",
	"GitHub Copilot (VS Code)",
	"GitHub Copilot (CLI)",
];

export const useClientFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "client",
		getSelectedOption: async () => {
			if (!value) {
				return null;
			}

			const client = AIBRIDGE_CLIENTS.find((client) => client === value);

			if (!client) {
				// In the case that the value is not in the list of clients (which
				// could happen if the value is from an old search param that no
				// longer exists), we return a placeholder option with the value as
				// the label so that the UI doesn't break and the user can still
				// see the value that is being filtered on.
				return {
					label: value,
					value,
					startIcon: (
						<AIBridgeClientIcon client={value} className="size-icon-sm" />
					),
				};
			}

			return {
				label: client,
				value: client,
				startIcon: (
					<AIBridgeClientIcon client={client} className="size-icon-sm" />
				),
			};
		},
		getOptions: async (query) => {
			return AIBRIDGE_CLIENTS.map((client) => ({
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

export const ClientFilter: FC<ClientFilterProps> = ({ menu }) => {
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
					placeholder="Search model..."
					value={menu.query}
					onValueChange={menu.setQuery}
				/>
			}
		/>
	);
};
