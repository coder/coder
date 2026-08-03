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
	FilterCombobox,
	type FilterFacet,
	type FilterSearchResult,
} from "#/components/Filter/FilterCombobox";
import type { UserFilterMenu } from "#/components/Filter/UserFilter";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import type { OrganizationsFilterMenu } from "#/modules/tableFiltering/options";
import type { StatusFilterMenu, TemplateFilterMenu } from "./menus";

type FilterFacetId = "status" | "template" | "organization" | "owner";

const WORKSPACE_CHIP_KEYS = [
	"owner",
	"status",
	"template",
	"organization",
] as const satisfies readonly FilterFacetId[];

const WORKSPACE_PREVIEW_LIMIT = 5;

export type WorkspaceFilterState = {
	filter: UseFilterResult;
	error?: unknown;
	menus: {
		user?: UserFilterMenu;
		template: TemplateFilterMenu;
		status: StatusFilterMenu;
		organizations?: OrganizationsFilterMenu;
	};
};

type WorkspaceFilterProps = Readonly<{
	filter: UseFilterResult;
	error: unknown;
	templateMenu: TemplateFilterMenu;
	statusMenu: StatusFilterMenu;
	userMenu?: UserFilterMenu;
	organizationsMenu?: OrganizationsFilterMenu;
}>;

export const WorkspacesFilter: FC<WorkspaceFilterProps> = ({
	filter,
	error: _error,
	templateMenu,
	statusMenu,
	userMenu,
	organizationsMenu,
}) => {
	const { showOrganizations } = useDashboard();
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const facets = useMemo(() => {
		const next: FilterFacet<FilterFacetId>[] = [
			{ id: "status", label: "Status", icon: CircleDotIcon, menu: statusMenu },
			{
				id: "template",
				label: "Template",
				icon: LayoutGridIcon,
				menu: templateMenu,
			},
		];

		if (showOrganizations && organizationsMenu) {
			next.push({
				id: "organization",
				label: "Organization",
				icon: Building2Icon,
				menu: organizationsMenu,
			});
		}

		if (userMenu) {
			next.push({
				id: "owner",
				label: "Owner",
				aliases: ["user"],
				icon: UserIcon,
				menu: userMenu,
			});
		}

		return next;
	}, [
		organizationsMenu,
		showOrganizations,
		statusMenu,
		templateMenu,
		userMenu,
	]);

	const getSearchResults = useCallback(
		async (query: string): Promise<FilterSearchResult[]> => {
			const response = await queryClient.fetchQuery(
				workspaces({
					q: query,
					limit: WORKSPACE_PREVIEW_LIMIT,
					offset: 0,
				}),
			);

			return response.workspaces.map((workspace) => ({
				id: workspace.id,
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
		(result: FilterSearchResult) => {
			if (result.href) {
				navigate(result.href);
			}
		},
		[navigate],
	);

	return (
		<FilterCombobox
			filter={filter}
			facets={facets}
			chipKeys={WORKSPACE_CHIP_KEYS}
			placeholder="Search and filter workspaces…"
			className="max-w-lg"
			getSearchResults={getSearchResults}
			onSearchResultSelect={onSearchResultSelect}
			searchResultsLabel="Workspaces"
		/>
	);
};
