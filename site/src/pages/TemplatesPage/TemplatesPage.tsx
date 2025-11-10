import { workspacePermissionsByOrganization } from "api/queries/organizations";
import { templateExamples, templates } from "api/queries/templates";
import { type UseFilterResult, useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { useAuthenticated } from "hooks";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

const TemplatesPage: FC = () => {
	const { permissions, user: me } = useAuthenticated();
	const { showOrganizations } = useDashboard();

	const [searchParams, setSearchParams] = useSearchParams();
	const filterState = useTemplatesFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
	});

	const templatesQuery = useQuery(templates({ q: filterState.filter.query }));
	const examplesQuery = useQuery({
		...templateExamples(),
		enabled: permissions.createTemplates,
	});

	const workspacePermissionsQuery = useQuery(
		workspacePermissionsByOrganization(
			templatesQuery.data?.map((template) => template.organization_id),
			me.id,
		),
	);

	const error =
		templatesQuery.error ||
		examplesQuery.error ||
		workspacePermissionsQuery.error;

	return (
		<>
			<title>{pageTitle("Templates")}</title>
			<TemplatesPageView
				error={error}
				filterState={filterState}
				showOrganizations={showOrganizations}
				canCreateTemplates={permissions.createTemplates}
				examples={examplesQuery.data}
				templates={templatesQuery.data}
				workspacePermissions={workspacePermissionsQuery.data}
			/>
		</>
	);
};

export default TemplatesPage;

export type TemplateFilterState = {
	filter: UseFilterResult;
	menus: {
		user?: ReturnType<typeof useUserFilterMenu>;
	};
};

type UseTemplatesFilterOptions = {
	searchParams: URLSearchParams;
	onSearchParamsChange: (params: URLSearchParams) => void;
};

const useTemplatesFilter = ({
	searchParams,
	onSearchParamsChange,
}: UseTemplatesFilterOptions): TemplateFilterState => {
	const filter = useFilter({
		searchParams,
		onSearchParamsChange,
	});

	const { permissions } = useAuthenticated();
	const canFilterByUser = permissions.viewAllUsers;
	const userMenu = useUserFilterMenu({
		value: filter.values.author,
		onChange: (option) =>
			filter.update({ ...filter.values, author: option?.value }),
		enabled: canFilterByUser,
	});

	return {
		filter,
		menus: {
			user: canFilterByUser ? userMenu : undefined,
		},
	};
};
