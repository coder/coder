import {
	Building2Icon,
	CircleDotIcon,
	LayoutGridIcon,
	UserIcon,
} from "lucide-react";
import { type FC, useCallback, useMemo } from "react";
import { useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { workspaces } from "#/api/queries/workspaces";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	type FilterCategory,
	FilterCombobox,
	type SearchResult,
} from "#/components/Filter/FilterCombobox";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	getOrganizationFilterOptions,
	getOwnerFilterOptions,
	getStatusFilterOptions,
	getTemplateFilterOptions,
} from "./categoryOptions";

const WORKSPACE_PREVIEW_LIMIT = 5;

export type WorkspaceFilterState = {
	filter: UseFilterResult;
	error?: unknown;
};

type WorkspaceFilterProps = Readonly<{
	filter: UseFilterResult;
	error: unknown;
}>;

export const WorkspacesFilter: FC<WorkspaceFilterProps> = ({
	filter,
	error: _error,
}) => {
	const { showOrganizations } = useDashboard();
	const { permissions, user: me } = useAuthenticated();
	const canFilterByUser = permissions.viewDeploymentConfig;
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
				getOptions: getTemplateFilterOptions,
			},
		];

		if (showOrganizations) {
			next.push({
				key: "organization",
				label: "Organization",
				icon: <Building2Icon />,
				getOptions: getOrganizationFilterOptions,
			});
		}

		if (canFilterByUser) {
			next.push({
				key: "owner",
				label: "Owner",
				aliases: ["user"],
				icon: <UserIcon />,
				getOptions: (query) => getOwnerFilterOptions(query, me),
			});
		}

		return next;
	}, [canFilterByUser, me, showOrganizations]);

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

	return (
		<FilterCombobox
			value={filter.query}
			onChange={filter.update}
			categories={categories}
			placeholder="Search and filter workspaces…"
			className="max-w-lg"
			getSearchResults={getSearchResults}
			onSearchResultSelect={onSearchResultSelect}
			searchResultsLabel="Workspaces"
		/>
	);
};
