import { API } from "#/api/api";
import type { WorkspaceStatus } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "#/components/Filter/FilterCombobox";
import {
	StatusIndicatorDot,
	type StatusIndicatorDotProps,
} from "#/components/StatusIndicator/StatusIndicator";
import { getDisplayWorkspaceStatus } from "#/utils/workspace";

const STATUS_OPTIONS: WorkspaceStatus[] = [
	"running",
	"stopped",
	"failed",
	"pending",
];

const getStatusIndicatorVariant = (
	status: WorkspaceStatus,
): StatusIndicatorDotProps["variant"] => {
	switch (status) {
		case "running":
			return "success";
		case "starting":
		case "pending":
			return "pending";
		case undefined:
		case "canceling":
		case "canceled":
		case "stopping":
		case "stopped":
			return "inactive";
		case "deleting":
		case "deleted":
			return "warning";
		case "failed":
			return "failed";
	}
};

export const getStatusFilterOptions = async (
	query: string,
): Promise<FilterOption[]> => {
	const normalized = query.trim().toLowerCase();
	const options = STATUS_OPTIONS.map((status) => {
		const display = getDisplayWorkspaceStatus(status);
		return {
			label: display.text,
			value: status,
			startIcon: (
				<StatusIndicatorDot variant={getStatusIndicatorVariant(status)} />
			),
		} satisfies FilterOption;
	});

	if (normalized.length === 0) {
		return options;
	}

	return options.filter(
		(option) =>
			option.label.toLowerCase().includes(normalized) ||
			option.value.toLowerCase().includes(normalized),
	);
};

export const getTemplateFilterOptions = async (
	query: string,
): Promise<FilterOption[]> => {
	const templates = await API.getTemplates();
	const normalized = query.trim().toLowerCase();
	const filtered =
		normalized.length === 0
			? templates
			: templates.filter(
					(template) =>
						template.name.toLowerCase().includes(normalized) ||
						template.display_name.toLowerCase().includes(normalized),
				);

	return filtered.map((template) => ({
		label: template.display_name || template.name,
		value: template.name,
		startIcon: (
			<Avatar
				size="md"
				variant="icon"
				src={template.icon}
				fallback={template.display_name || template.name}
			/>
		),
	}));
};

export const getOwnerFilterOptions = async (
	query: string,
	me: Readonly<{ username: string; avatar_url?: string }>,
): Promise<FilterOption[]> => {
	const usersRes = await API.getUsers({ q: query, limit: 25 });
	const options = usersRes.users
		.filter((user) => user.username !== me.username)
		.map<FilterOption>((user) => ({
			label: user.username,
			value: user.username,
			startIcon: (
				<Avatar fallback={user.username} src={user.avatar_url} size="md" />
			),
		}));

	return [
		{
			label: me.username,
			value: me.username,
			startIcon: (
				<Avatar fallback={me.username} src={me.avatar_url} size="md" />
			),
		},
		...options,
	];
};

export const getOrganizationFilterOptions = async (): Promise<
	FilterOption[]
> => {
	const organizations = await API.getOrganizations();
	const permissions = await API.checkAuthorization({
		checks: Object.fromEntries(
			organizations.map((organization) => [
				organization.id,
				{
					object: {
						resource_type: "audit_log",
						organization_id: organization.id,
					},
					action: "read",
				},
			]),
		),
	});

	return organizations
		.filter((organization) => permissions[organization.id])
		.map((organization) => ({
			label: organization.display_name || organization.name,
			value: organization.name,
			startIcon: (
				<Avatar
					key={organization.id}
					size="md"
					fallback={organization.display_name || organization.name}
					src={organization.icon}
				/>
			),
		}));
};
