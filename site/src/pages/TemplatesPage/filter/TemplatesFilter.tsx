import { Building2Icon, SlidersHorizontalIcon, UserIcon } from "lucide-react";
import { type FC, useCallback, useMemo } from "react";
import { useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import { templates } from "#/api/queries/templates";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type {
	FilterCategory,
	SearchResult,
} from "#/components/Filter/FilterCombobox/types";
import {
	getSelfUserFilterOptions,
	getUserFilterOptions,
} from "#/components/Filter/userFilterOptions";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import {
	ATTRIBUTE_CHIP_KEYS,
	getAttributeFilterOptions,
	getOrganizationFilterOptions,
} from "./categoryOptions";

const TEMPLATE_PREVIEW_LIMIT = 5;

type TemplatesFilterProps = Readonly<{
	filter: UseFilterResult;
	error: unknown;
}>;

export const TemplatesFilter: FC<TemplatesFilterProps> = ({
	filter,
	error,
}) => {
	const { showOrganizations, organizations } = useDashboard();
	const { permissions, user: me } = useAuthenticated();
	const canListUsers = permissions.viewAllUsers;
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const getLink = useLinks();

	const categories = useMemo(() => {
		const next: FilterCategory[] = [
			{
				key: "attributes",
				label: "Attributes",
				icon: <SlidersHorizontalIcon />,
				chipKeys: ATTRIBUTE_CHIP_KEYS,
				getOptions: getAttributeFilterOptions,
			},
		];

		if (showOrganizations) {
			next.push({
				key: "organization",
				label: "Organization",
				icon: <Building2Icon />,
				getOptions: (query) =>
					getOrganizationFilterOptions(query, organizations),
			});
		}

		// Always expose Author so `author` stays a recognized chip key and
		// `author:me` renders as a chip rather than free text. Users who cannot
		// list others only see themselves.
		next.push({
			key: "author",
			label: "Author",
			icon: <UserIcon />,
			getOptions: canListUsers
				? (query) => getUserFilterOptions(query, me, queryClient)
				: (query) => getSelfUserFilterOptions(query, me),
		});

		return next;
	}, [canListUsers, me, organizations, showOrganizations, queryClient]);

	const getSearchResults = useCallback(
		async (query: string): Promise<SearchResult[]> => {
			const matched = await queryClient.fetchQuery(templates({ q: query }));

			return matched.slice(0, TEMPLATE_PREVIEW_LIMIT).map((template) => ({
				value: template.id,
				label: template.display_name || template.name,
				subtitle:
					template.organization_display_name || template.organization_name,
				imageUrl: template.icon,
				href: getLink(
					linkToTemplate(template.organization_name, template.name),
				),
			}));
		},
		[getLink, queryClient],
	);

	const onSearchResultSelect = useCallback(
		(result: SearchResult) => {
			if (result.href) {
				navigate(result.href);
			}
		},
		[navigate],
	);

	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="flex flex-col gap-2">
			<FilterCombobox
				value={filter.query}
				onChange={filter.update}
				categories={categories}
				placeholder="Search and filter templates…"
				className="max-w-lg"
				errorMessage={
					showValidationError ? getValidationErrorMessage(error) : undefined
				}
				getSearchResults={getSearchResults}
				onSearchResultSelect={onSearchResultSelect}
				searchResultsLabel="Jump to template"
				searchResultsErrorMessage="Couldn't load template previews."
			/>
		</div>
	);
};
