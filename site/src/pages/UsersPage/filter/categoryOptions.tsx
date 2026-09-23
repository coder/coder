import type { ComponentProps } from "react";
import type { QueryClient } from "react-query";
import { roles } from "#/api/queries/roles";
import type { UserStatus } from "#/api/typesGenerated";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";

type OptionsQueryClient = Pick<QueryClient, "fetchQuery">;

const matches = (option: FilterOption, query: string): boolean => {
	const normalized = query.trim().toLowerCase();
	return (
		normalized.length === 0 ||
		option.label.toLowerCase().includes(normalized) ||
		option.value.toLowerCase().includes(normalized)
	);
};

const STATUS_OPTIONS: ReadonlyArray<{
	value: UserStatus;
	label: string;
	variant: ComponentProps<typeof StatusIndicatorDot>["variant"];
}> = [
	{ value: "active", label: "Active", variant: "success" },
	{ value: "dormant", label: "Dormant", variant: "warning" },
	{ value: "suspended", label: "Suspended", variant: "inactive" },
];

export const getStatusFilterOptions = async (
	query: string,
): Promise<FilterOption[]> =>
	STATUS_OPTIONS.map(
		(status): FilterOption => ({
			label: status.label,
			value: status.value,
			startIcon: <StatusIndicatorDot variant={status.variant} size="md" />,
		}),
	).filter((option) => matches(option, query));

export const getRoleFilterOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const siteRoles = await queryClient.fetchQuery(roles());
	return siteRoles
		.map(
			(role): FilterOption => ({
				label: role.display_name || role.name,
				value: role.name,
			}),
		)
		.filter((option) => matches(option, query));
};

/** Query keys shown as User type chips. */
export const USER_TYPE_CHIP_KEYS: readonly string[] = ["service_account"];

export const getUserTypeFilterOptions = async (
	query: string,
): Promise<FilterOption[]> =>
	[
		{
			label: "Service account",
			value: "service_account",
			token: "service_account:true",
		},
	].filter((option) => matches(option, query));
