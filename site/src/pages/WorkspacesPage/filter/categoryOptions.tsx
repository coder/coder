import { MoonIcon, RefreshCwOffIcon, Share2Icon } from "lucide-react";
import type { ReactNode } from "react";
import type { QueryClient } from "react-query";
import { permittedOrganizations } from "#/api/queries/organizations";
import { templates } from "#/api/queries/templates";
import { users } from "#/api/queries/users";
import type { WorkspaceStatus } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";
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

type AttributeDefinition = {
	label: string;
	value: string;
	icon: ReactNode;
	/** Hidden when the deployment lacks the entitlement gating this attribute. */
	requiresDormantEntitlement: boolean;
};

const attributeIcon = (icon: ReactNode): ReactNode => (
	<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
		{icon}
	</span>
);

const ATTRIBUTE_DEFINITIONS: readonly AttributeDefinition[] = [
	{
		label: "Outdated",
		value: "outdated",
		icon: <RefreshCwOffIcon className="size-icon-sm" />,
		requiresDormantEntitlement: false,
	},
	{
		label: "Dormant",
		value: "dormant",
		icon: <MoonIcon className="size-icon-sm" />,
		requiresDormantEntitlement: true,
	},
	{
		label: "Shared",
		value: "shared",
		icon: <Share2Icon className="size-icon-sm" />,
		requiresDormantEntitlement: false,
	},
];

/**
 * Query keys the Attributes category owns, derived from its option definitions
 * so a new attribute cannot commit a chip token the parser silently drops.
 */
export const ATTRIBUTE_CHIP_KEYS: readonly string[] = ATTRIBUTE_DEFINITIONS.map(
	(attribute) => attribute.value,
);

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

	return ATTRIBUTE_DEFINITIONS.filter(
		(attribute) =>
			!attribute.requiresDormantEntitlement || options.canFilterDormant,
	)
		.filter(
			(attribute) =>
				normalized.length === 0 ||
				attribute.label.toLowerCase().includes(normalized) ||
				attribute.value.toLowerCase().includes(normalized),
		)
		.map((attribute) => ({
			label: attribute.label,
			value: attribute.value,
			token: `${attribute.value}:true`,
			startIcon: attributeIcon(attribute.icon),
		}));
};

export const getOrganizationFilterOptions = async (
	query: string,
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

	const normalized = query.trim().toLowerCase();
	const mapped = permitted.map((organization) => ({
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

	if (normalized.length === 0) {
		return mapped;
	}
	return mapped.filter(
		(option) =>
			option.label.toLowerCase().includes(normalized) ||
			option.value.toLowerCase().includes(normalized),
	);
};
