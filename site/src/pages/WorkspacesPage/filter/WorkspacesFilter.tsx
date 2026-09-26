import {
	Building2Icon,
	CircleDotIcon,
	LayoutPanelTopIcon,
	TagIcon,
	UserIcon,
	UserKeyIcon,
} from "lucide-react";
import { type FC, useMemo } from "react";
import { useQueryClient } from "react-query";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	ATTRIBUTE_CHIP_KEYS,
	getAttributeFilterOptions,
	getOrganizationFilterOptions,
	getSelfUserFilterOptions,
	getStatusFilterOptions,
	getTemplateFilterOptions,
	getUserFilterOptions,
} from "./categoryOptions";

type WorkspaceFilterProps = Readonly<{
	filter: UseFilterResult;
	error: unknown;
}>;

export const WorkspacesFilter: FC<WorkspaceFilterProps> = ({
	filter,
	error,
}) => {
	const { showOrganizations, entitlements } = useDashboard();
	const { permissions, user: me } = useAuthenticated();
	// TODO(DEVEX-421 follow-up): `viewDeploymentConfig` is the wrong capability
	// for listing users. It is carried over from the legacy page; replace it with
	// a list-users capability check. Users without it still get User and Owner
	// categories scoped to themselves (below) so `user:me` keeps working.
	const canListUsers = permissions.viewDeploymentConfig;
	const canFilterDormant =
		entitlements.features.advanced_template_scheduling.enabled;
	const queryClient = useQueryClient();

	const categories = useMemo(() => {
		// Always expose User and Owner so both stay recognized chip keys and the
		// page's default `user:me` renders as a chip rather than free text.
		// Users who cannot list others only see themselves.
		const getUserOptions = canListUsers
			? (query: string) => getUserFilterOptions(query, me, queryClient)
			: (query: string) => getSelfUserFilterOptions(query, me);
		const next: FilterCategory[] = [
			{
				key: "owner",
				label: "Owner",
				icon: <UserKeyIcon />,
				getOptions: getUserOptions,
			},
			{
				// Workspaces the user owns or that are shared with them.
				key: "user",
				label: "User",
				icon: <UserIcon />,
				getOptions: getUserOptions,
			},
			{
				key: "status",
				label: "Status",
				icon: <CircleDotIcon />,
				inlineOptions: true,
				inlineOptionsIcons: true,
				getOptions: getStatusFilterOptions,
			},
			{
				key: "attribute",
				label: "Attributes",
				aliases: ["attributes"],
				icon: <TagIcon />,
				// Boolean workspace filters live under their own keys, so the
				// category owns them for chip parsing.
				chipKeys: ATTRIBUTE_CHIP_KEYS,
				inlineOptions: true,
				inlineOptionsLabel: "Workspace is…",
				inlineOptionsExclusive: true,
				chipLabelOnly: true,
				getOptions: (query) =>
					getAttributeFilterOptions(query, { canFilterDormant }),
			},
			{
				key: "template",
				label: "Template",
				// Deprecated templates are not offered, so the row can hide while
				// its one active template would still narrow the results.
				hideWhenSingleOption: true,
				icon: <LayoutPanelTopIcon />,
				getOptions: (query) => getTemplateFilterOptions(query, queryClient),
			},
		];

		if (showOrganizations) {
			next.push({
				key: "organization",
				label: "Organization",
				// Only organizations with `audit_log:read` are offered, so the row
				// can hide while its one option would still narrow the results.
				hideWhenSingleOption: true,
				icon: <Building2Icon />,
				getOptions: (query) => getOrganizationFilterOptions(query, queryClient),
			});
		}

		return next;
	}, [canListUsers, canFilterDormant, me, showOrganizations, queryClient]);

	// The page hides its ErrorAlert for API validation errors, so the filter
	// owns surfacing the actionable "invalid query" message.
	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="flex min-w-0 flex-col gap-2">
			<FilterCombobox
				value={filter.query}
				onChange={filter.update}
				categories={categories}
				placeholder="Search and filter workspaces…"
				// Full width on mobile. From `sm` up it starts at a compact width
				// and widens to fit chips before wrapping.
				className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
				errorMessage={
					showValidationError ? getValidationErrorMessage(error) : undefined
				}
			/>
		</div>
	);
};
