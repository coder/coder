import { useQueryClient } from "react-query";
import type { AuthorizationCheck } from "#/api/typesGenerated";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import { getOrganizationFilterOptions } from "#/components/Filter/organizationOptions";
import { SelectFilter } from "#/components/Filter/SelectFilter";

// Matches the organization filter on the Connection Logs and Audit pages.
const organizationCheck: AuthorizationCheck = {
	object: { resource_type: "audit_log" },
	action: "read",
};

/** Filter menu state for the organizations the user can read audit logs in. */
export const useOrganizationFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	const queryClient = useQueryClient();
	return useFilterMenu({
		id: "organization",
		getSelectedOption: async () => {
			const options = await getOrganizationFilterOptions(
				"",
				queryClient,
				organizationCheck,
			);
			return options.find((option) => option.value === value) ?? null;
		},
		getOptions: (query) =>
			getOrganizationFilterOptions(query, queryClient, organizationCheck),
		value,
		onChange,
		enabled,
	});
};

export type OrganizationFilterMenu = ReturnType<
	typeof useOrganizationFilterMenu
>;

type OrganizationFilterProps = {
	menu: OrganizationFilterMenu;
	width?: number;
};

/** Organization dropdown for the AI Gateway sessions filter bar. */
export const OrganizationFilter: React.FC<OrganizationFilterProps> = ({
	menu,
	width,
}) => {
	return (
		<SelectFilter
			label="Select an organization"
			placeholder="All organizations"
			emptyText="No organizations found"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
			searchLabel="Search organization"
			selectFilterSearch={
				<ComboboxInput
					placeholder="Search organization..."
					value={menu.query}
					onValueChange={menu.setQuery}
				/>
			}
		/>
	);
};
