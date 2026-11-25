import { Building2Icon, TagIcon, UserIcon } from "lucide-react";
import { useMemo } from "react";
import { useQueryClient } from "react-query";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import { Avatar } from "#/components/Avatar/Avatar";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import { filterOptionsByText } from "#/components/Filter/FilterCombobox/filterQuery";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import {
	getMeUserFilterOptions,
	getUserFilterOptions,
} from "#/components/Filter/userOptions";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getMemberOrganizations,
	useDashboard,
} from "#/modules/dashboard/useDashboard";

export const CLASSIC_PARAMETER_FLOW_FILTER = "compatibility_mode:true";

type TemplatesFilterProps = { filter: UseFilterResult; error?: unknown };

export const TemplatesFilter: React.FC<TemplatesFilterProps> = ({
	filter,
	error,
}) => {
	const { organizations } = useDashboard();
	const { permissions, user: me } = useAuthenticated();
	const queryClient = useQueryClient();
	const categories = useMemo<FilterCategory[]>(
		() => [
			{
				key: "author",
				optionsKey: "templates.author",
				label: "Author",
				icon: <UserIcon />,
				getOptions: permissions.viewAllUsers
					? (query) => getUserFilterOptions(query, me, queryClient)
					: (query) => getMeUserFilterOptions(query, me),
			},
			{
				key: "organization",
				optionsKey: "templates.organization",
				label: "Organization",
				icon: <Building2Icon />,
				hideWhenSingleOption: true,
				getOptions: async (query) =>
					getMemberOrganizations(organizations, me)
						.filter((org) =>
							`${org.name} ${org.display_name}`
								.toLowerCase()
								.includes(query.trim().toLowerCase()),
						)
						.map((org) => ({
							label: org.display_name || org.name,
							value: org.name,
							startIcon: (
								<Avatar
									size="sm"
									fallback={org.display_name || org.name}
									src={org.icon}
								/>
							),
						})),
			},
			{
				key: "attribute",
				optionsKey: "templates.attribute",
				label: "Attributes",
				icon: <TagIcon />,
				chipKeys: ["deprecated", "compatibility_mode"],
				inlineOptions: true,
				inlineOptionsLabel: "Template is…",
				chipLabelOnly: true,
				getOptions: async (query) =>
					filterOptionsByText(
						[
							{
								label: "Deprecated",
								value: "deprecated",
								token: "deprecated:true",
							},
							{
								label: "Compatibility mode",
								value: "compatibility_mode",
								token: CLASSIC_PARAMETER_FLOW_FILTER,
							},
						],
						query,
					),
			},
		],
		[organizations, permissions.viewAllUsers, me, queryClient],
	);
	return (
		<div className="mb-6 flex min-w-0 flex-col">
			<FilterCombobox
				value={filter.query}
				onChange={filter.update}
				categories={categories}
				placeholder="Search and filter templates…"
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
