import capitalize from "lodash/capitalize";
import {
	ActivityIcon,
	BoxIcon,
	Building2Icon,
	HammerIcon,
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
	AuditActions,
	BuildReasons,
	type ResourceType,
	ResourceTypes,
} from "#/api/typesGenerated";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import { filterOptionsByText } from "#/components/Filter/FilterCombobox/filterQuery";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { getOrganizationFilterOptions } from "#/components/Filter/organizationOptions";
import {
	getMeUserFilterOptions,
	getUserFilterOptions,
} from "#/components/Filter/userOptions";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getBuildReasonLabel } from "#/utils/workspace";

const presets: FilterOption[] = [
	{
		label: "Created workspaces",
		value: "created_workspaces",
		startIcon: <Settings2Icon />,
		query: "resource_type:workspace action:create",
	},
	{
		label: "Added templates",
		value: "added_templates",
		startIcon: <Settings2Icon />,
		query: "resource_type:template action:create",
	},
	{
		label: "Deleted users",
		value: "deleted_users",
		startIcon: <Settings2Icon />,
		query: "resource_type:user action:delete",
	},
	{
		label: "Builds started by a user",
		value: "user_builds",
		startIcon: <Settings2Icon />,
		query: "resource_type:workspace_build action:start build_reason:initiator",
	},
	{
		label: "User logins",
		value: "user_logins",
		startIcon: <Settings2Icon />,
		query: "resource_type:api_key action:login",
	},
];

// These historical actions remain queryable but are no longer recorded.
const actionOptions: FilterOption[] = AuditActions.filter(
	(action) => !["connect", "disconnect", "open", "close"].includes(action),
).map((action) => ({ value: action, label: capitalize(action) }));

const resourceTypeLabels: Partial<Record<ResourceType, string>> = {
	api_key: "API key",
	git_ssh_key: "Git SSH key",
	template_version: "Template version",
	workspace_build: "Workspace build",
	chat_instruction_settings: "Chat instruction settings",
	chat_organization_system_prompt: "Chat organization system prompt",
	chat_project_instructions: "Chat project instructions",
	chat_project_skill: "Chat project skill",
	chat_operational_settings: "Chat operational settings",
	experiment_rule: "Experiment rule",
};

const resourceOptions: FilterOption[] = ResourceTypes.map((type) => ({
	value: type,
	label: resourceTypeLabels[type] ?? capitalize(type),
}));

type AuditFilterProps = { filter: UseFilterResult; error?: unknown };

export const AuditFilter: React.FC<AuditFilterProps> = ({ filter, error }) => {
	const { permissions, user: me } = useAuthenticated();
	const { showOrganizations } = useDashboard();
	const queryClient = useQueryClient();
	const categories = useMemo(() => {
		const next: FilterCategory[] = [
			{
				key: "resource_type",
				optionsKey: "audit.resource_type",
				label: "Resource type",
				aliases: ["resource"],
				icon: <BoxIcon />,
				getOptions: async (query) =>
					filterOptionsByText(resourceOptions, query),
			},
			{
				key: "action",
				optionsKey: "audit.action",
				label: "Action",
				icon: <ActivityIcon />,
				getOptions: async (query) => filterOptionsByText(actionOptions, query),
			},
			{
				key: "username",
				optionsKey: "audit.username",
				label: "User",
				aliases: ["user"],
				icon: <UserIcon />,
				getOptions: permissions.viewAllUsers
					? (query) => getUserFilterOptions(query, me, queryClient)
					: (query) => getMeUserFilterOptions(query, me),
			},
			{
				key: "build_reason",
				optionsKey: "audit.build_reason",
				label: "Build reason",
				icon: <HammerIcon />,
				getOptions: async (query) =>
					filterOptionsByText(
						BuildReasons.map((reason) => ({
							value: reason,
							label: getBuildReasonLabel(reason) ?? reason,
						})),
						query,
					),
			},
		];
		if (showOrganizations) {
			next.push({
				key: "organization",
				optionsKey: "audit.organization",
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
			optionsKey: "audit.presets",
			label: "Presets",
			chipKeys: [],
			inlineOptions: true,
			inlineOptionsLabel: "Show…",
			inlineOptionsIcons: true,
			getOptions: async (query) => filterOptionsByText(presets, query),
		});
		return next.map((category) => ({ ...category, singleSelect: true }));
	}, [me, permissions.viewAllUsers, queryClient, showOrganizations]);
	return (
		<div className="mb-6 flex min-w-0 flex-col">
			<FilterCombobox
				value={filter.query}
				onChange={filter.update}
				categories={categories}
				placeholder="Search and filter audit logs…"
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
