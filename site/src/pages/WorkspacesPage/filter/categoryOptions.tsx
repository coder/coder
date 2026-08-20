import { MoonIcon, RefreshCwOffIcon, Share2Icon } from "lucide-react";
import type { QueryClient } from "react-query";
import { permittedOrganizations } from "#/api/queries/organizations";
import { templates } from "#/api/queries/templates";
import { users } from "#/api/queries/users";
import type { WorkspaceStatus } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "#/components/Filter/FilterCombobox";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import { variantByStatusType } from "#/modules/workspaces/WorkspaceStatusIndicator/WorkspaceStatusIndicator";
import { getDisplayWorkspaceStatus } from "#/utils/workspace";

// Owner suggestions are capped; the picker is a prefix search, not a full list.
const OWNER_SUGGESTIONS_LIMIT = 25;

const STATUS_OPTIONS: WorkspaceStatus[] = [
	"running",
	"stopped",
	"failed",
	"pending",
];

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
				<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
					<StatusIndicatorDot
						variant={variantByStatusType[display.type]}
						size="md"
					/>
				</span>
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
	queryClient: QueryClient,
): Promise<FilterOption[]> => {
	// Fetch through the shared `templates` query so the dropdown and the page's
	// own template list read from one cache instead of diverging.
	const allTemplates = await queryClient.fetchQuery(templates());
	const normalized = query.trim().toLowerCase();
	const filtered =
		normalized.length === 0
			? allTemplates
			: allTemplates.filter(
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
	queryClient: QueryClient,
): Promise<FilterOption[]> => {
	const usersRes = await queryClient.fetchQuery(
		users({ q: query, limit: OWNER_SUGGESTIONS_LIMIT }),
	);
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
			label: `${me.username} (you)`,
			// Commit the backend's per-session `owner:me` sentinel, matching the
			// page's `owner:me` fallback, rather than a static `owner:<username>`.
			value: "me",
			startIcon: (
				<Avatar fallback={me.username} src={me.avatar_url} size="md" />
			),
		},
		...options,
	];
};

type AttributeOption = FilterOption & {
	/** Hidden when the deployment lacks the entitlement gating this attribute. */
	entitled: boolean;
};

/**
 * Boolean workspace attributes exposed as a single "Attributes" category. Each
 * option commits its own `key:true` chip (e.g. `outdated:true`) rather than a
 * shared `attributes:` key, matching the backend workspace search filters.
 */
export const getAttributeFilterOptions = async (
	query: string,
	options: Readonly<{ canFilterDormant: boolean }>,
): Promise<FilterOption[]> => {
	const normalized = query.trim().toLowerCase();
	const attributes: AttributeOption[] = [
		{
			label: "Outdated",
			value: "outdated",
			token: "outdated:true",
			startIcon: (
				<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
					<RefreshCwOffIcon className="size-icon-sm" />
				</span>
			),
			entitled: true,
		},
		{
			label: "Dormant",
			value: "dormant",
			token: "dormant:true",
			startIcon: (
				<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
					<MoonIcon className="size-icon-sm" />
				</span>
			),
			entitled: options.canFilterDormant,
		},
		{
			label: "Shared",
			value: "shared",
			token: "shared:true",
			startIcon: (
				<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
					<Share2Icon className="size-icon-sm" />
				</span>
			),
			entitled: true,
		},
	];

	return attributes
		.filter((attribute) => attribute.entitled)
		.filter(
			(attribute) =>
				normalized.length === 0 ||
				attribute.label.toLowerCase().includes(normalized) ||
				attribute.value.toLowerCase().includes(normalized),
		)
		.map(({ entitled: _entitled, ...option }) => option);
};

export const getOrganizationFilterOptions = async (
	queryClient: QueryClient,
): Promise<FilterOption[]> => {
	// Reuse the shared `permittedOrganizations` query, which fetches the org list
	// and applies the `audit_log:read` authorization gate, rather than duplicating
	// that logic under a private key.
	// TODO(DEVEX-421 follow-up): the `audit_log:read` gate is carried over from the
	// old `useOrganizationsFilterMenu` and is wrong for workspace filtering. A
	// plain member sees an empty org list here. Replace it with a workspace-read /
	// org-membership check.
	const permitted = await queryClient.fetchQuery(
		permittedOrganizations({
			object: { resource_type: "audit_log" },
			action: "read",
		}),
	);

	return permitted.map((organization) => ({
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
