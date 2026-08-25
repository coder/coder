import {
	Building2Icon,
	CircleDotIcon,
	LayoutGridIcon,
	SlidersHorizontalIcon,
	UserIcon,
} from "lucide-react";
import { type FC, useCallback, useMemo } from "react";
import { useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import { workspaces } from "#/api/queries/workspaces";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type {
	FilterCategory,
	SearchResult,
} from "#/components/Filter/FilterCombobox/types";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	ATTRIBUTE_CHIP_KEYS,
	getAttributeFilterOptions,
	getOrganizationFilterOptions,
	getOwnerFilterOptions,
	getStatusFilterOptions,
	getTemplateFilterOptions,
} from "./categoryOptions";

const WORKSPACE_PREVIEW_LIMIT = 5;

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
	// TODO(DEVEX-421 follow-up): `viewDeploymentConfig` gates Owner on the wrong
	// capability. Listing users (which the Owner options need) is not the same as
	// reading the deployment config. Carried over from the legacy page; replace
	// with a list-users capability check.
	const canFilterByUser = permissions.viewDeploymentConfig;
	const canFilterDormant =
		entitlements.features.advanced_template_scheduling.enabled;
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const categories = useMemo(() => {
		const next: FilterCategory[] = [
			{
				key: "status",
				label: "Status",
				icon: <CircleDotIcon />,
				getOptions: getStatusFilterOptions,
			},
			{
				key: "template",
				label: "Template",
				icon: <LayoutGridIcon />,
				getOptions: (query) => getTemplateFilterOptions(query, queryClient),
			},
			{
				key: "attributes",
				label: "Attributes",
				icon: <SlidersHorizontalIcon />,
				// Boolean workspace filters live under their own keys, so the
				// category owns them for chip parsing.
				chipKeys: ATTRIBUTE_CHIP_KEYS,
				getOptions: (query) =>
					getAttributeFilterOptions(query, { canFilterDormant }),
			},
		];

		if (showOrganizations) {
			next.push({
				key: "organization",
				label: "Organization",
				icon: <Building2Icon />,
				getOptions: (query) => getOrganizationFilterOptions(query, queryClient),
			});
		}

		if (canFilterByUser) {
			next.push({
				key: "owner",
				label: "Owner",
				aliases: ["user"],
				icon: <UserIcon />,
				getOptions: (query) => getOwnerFilterOptions(query, me, queryClient),
			});
		}

		return next;
	}, [canFilterByUser, canFilterDormant, me, showOrganizations, queryClient]);

	const getSearchResults = useCallback(
		async (query: string): Promise<SearchResult[]> => {
			const response = await queryClient.fetchQuery(
				workspaces({
					q: query,
					limit: WORKSPACE_PREVIEW_LIMIT,
					offset: 0,
				}),
			);

			return response.workspaces.map((workspace) => ({
				value: workspace.id,
				label: workspace.name,
				subtitle: [
					workspace.owner_name,
					workspace.template_display_name || workspace.template_name,
				]
					.filter(Boolean)
					.join(" · "),
				imageUrl: workspace.owner_avatar_url,
				href: `/@${workspace.owner_name}/${workspace.name}`,
			}));
		},
		[queryClient],
	);

	const onSearchResultSelect = useCallback(
		(result: SearchResult) => {
			if (result.href) {
				navigate(result.href);
			}
		},
		[navigate],
	);

	// The page hides its ErrorAlert for API validation errors, so the filter
	// owns surfacing the actionable "invalid query" message.
	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="flex flex-col gap-2">
			<FilterCombobox
				value={filter.query}
				onChange={filter.update}
				categories={categories}
				placeholder="Search and filter workspaces…"
				className="max-w-lg"
				errorMessage={
					showValidationError ? getValidationErrorMessage(error) : undefined
				}
				getSearchResults={getSearchResults}
				onSearchResultSelect={onSearchResultSelect}
				searchResultsLabel="Jump to workspace"
			/>
		</div>
	);
};
