import type { QueryClient } from "react-query";
import { permittedOrganizations } from "#/api/queries/organizations";
import type { AuthorizationCheck } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "./FilterCombobox/types";

/** Suggests organizations permitted for the requested action, filtered by name. */
export const getOrganizationFilterOptions = async (
	query: string,
	queryClient: Pick<QueryClient, "fetchQuery">,
	check: AuthorizationCheck,
): Promise<FilterOption[]> => {
	const permitted = await queryClient.fetchQuery(permittedOrganizations(check));

	const normalized = query.trim().toLowerCase();
	const mapped = permitted.map((organization) => ({
		label: organization.display_name || organization.name,
		value: organization.name,
		startIcon: (
			<Avatar
				key={organization.id}
				size="sm"
				fallback={organization.display_name || organization.name}
				src={organization.icon}
			/>
		),
	}));

	if (normalized.length === 0) {
		return mapped;
	}
	return mapped.filter(
		(option) =>
			option.label.toLowerCase().includes(normalized) ||
			option.value.toLowerCase().includes(normalized),
	);
};
