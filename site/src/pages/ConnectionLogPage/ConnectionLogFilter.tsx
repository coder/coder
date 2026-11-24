import capitalize from "lodash/capitalize";
import {
	Building2Icon,
	CircleDotIcon,
	PlugIcon,
	Settings2Icon,
	UserIcon,
} from "lucide-react";
import { useMemo } from "react";
import { useQueryClient } from "react-query";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import {
	ConnectionLogMethods,
	ConnectionLogStatuses,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import {
	extractFreeText,
	filterOptionsByText,
} from "#/components/Filter/FilterCombobox/filterQuery";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { parseFilterQuery } from "#/components/Filter/filterQuery";
import { getOrganizationFilterOptions } from "#/components/Filter/organizationOptions";
import {
	getMeUserFilterOptions,
	getUserFilterOptions,
} from "#/components/Filter/userOptions";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { connectionLogMethodLabels } from "./connectionLogMethodLabels";

const statusOptions: FilterOption[] = ConnectionLogStatuses.map((status) => ({
	value: status,
	label: capitalize(status),
}));
const methodOptions: FilterOption[] = ConnectionLogMethods.map((method) => ({
	value: method,
	label: connectionLogMethodLabels[method],
}));
const presets: FilterOption[] = [
	{
		label: "Active SSH connections",
		value: "active_ssh_connections",
		startIcon: <Settings2Icon />,
		query: "status:ongoing method:ssh",
	},
];

type ConnectionLogFilterProps = {
	filter: UseFilterResult;
	error?: unknown;
};

/** Filters connection events while retaining support for saved legacy queries. */
export const ConnectionLogFilter: React.FC<ConnectionLogFilterProps> = ({
	filter,
	error,
}) => {
	const { permissions, user: me } = useAuthenticated();
	const { showOrganizations } = useDashboard();
	const queryClient = useQueryClient();
	const categories = useMemo(() => {
		const next: FilterCategory[] = [
			{
				key: "workspace_owner",
				optionsKey: "connectionLogs.workspace_owner",
				label: "Owner",
				icon: <UserIcon />,
				getOptions: permissions.viewAllUsers
					? (query) => getUserFilterOptions(query, me, queryClient)
					: (query) => getMeUserFilterOptions(query, me),
			},
			{
				key: "status",
				optionsKey: "connectionLogs.status",
				label: "Status",
				icon: <CircleDotIcon />,
				getOptions: async (query) => filterOptionsByText(statusOptions, query),
			},
			{
				key: "method",
				optionsKey: "connectionLogs.method",
				label: "Method",
				icon: <PlugIcon />,
				getOptions: async (query) => filterOptionsByText(methodOptions, query),
			},
		];
		if (showOrganizations) {
			next.push({
				key: "organization",
				optionsKey: "connectionLogs.organization",
				label: "Organization",
				icon: <Building2Icon />,
				getOptions: (query) =>
					getOrganizationFilterOptions(query, queryClient, {
						object: { resource_type: "audit_log" },
						action: "read",
					}),
			});
		}
		next.push({
			key: "preset",
			optionsKey: "connectionLogs.presets",
			label: "Presets",
			chipKeys: [],
			inlineOptions: true,
			inlineOptionsLabel: "Show…",
			inlineOptionsIcons: true,
			getOptions: async (query) => filterOptionsByText(presets, query),
		});
		return next.map((category) => ({ ...category, singleSelect: true }));
	}, [me, permissions.viewAllUsers, queryClient, showOrganizations]);

	const handleChange = (query: string) => {
		// The API rejects method combined with the deprecated type filter.
		filter.update(
			parseFilterQuery(query).method ? extractFreeText(query, ["type"]) : query,
		);
	};

	return (
		<div className="mb-6 flex min-w-0 flex-col">
			{filter.values.type && (
				<Alert severity="warning" className="mb-4">
					The <code>type</code> filter is deprecated and will be removed in a
					future release. Use <code>method</code> and <code>app</code> instead.
				</Alert>
			)}
			<FilterCombobox
				value={filter.query}
				onChange={handleChange}
				categories={categories}
				placeholder="Search and filter connection logs…"
				className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
				errorMessage={
					hasError(error) && isApiValidationError(error)
						? getValidationErrorMessage(error)
						: undefined
				}
			/>
		</div>
	);
};
