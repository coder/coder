import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import { workspacePermissionsByOrganization } from "#/api/queries/organizations";
import {
	templateExamples,
	templates,
	templateUpdatePermissionsByOrganization,
} from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { useTemplatesFilter } from "./TemplatesFilter";
import { TemplatesPageView } from "./TemplatesPageView";

const TemplatesPage: FC = () => {
	const { permissions, user: me } = useAuthenticated();
	const { organizations, showOrganizations } = useDashboard();

	const [searchParams, setSearchParams] = useSearchParams();
	const filterState = useTemplatesFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
	});

	const templatesQuery = useQuery(templates({ q: filterState.filter.query }));
	const templateUpdatePermissionsQuery = useQuery(
		templateUpdatePermissionsByOrganization(
			organizations.map((organization) => organization.id),
		),
	);
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

	const deploymentConfigQuery = useQuery({
		...deploymentConfig(),
		enabled: permissions.createTemplates,
	});
	const templateBuilderEnabled =
		deploymentConfigQuery.isSuccess &&
		!deploymentConfigQuery.data?.config?.template_builder?.disabled &&
		permissions.createTemplates;

	const error =
		templatesQuery.error ||
		examplesQuery.error ||
		templateUpdatePermissionsQuery.error ||
		workspacePermissionsQuery.error;

	return (
		<>
			<title>{pageTitle("Templates")}</title>
			<TemplatesPageView
				error={error}
				filterState={filterState}
				showOrganizations={showOrganizations}
				canCreateTemplates={permissions.createTemplates}
				templateBuilderEnabled={templateBuilderEnabled}
				examples={examplesQuery.data}
				templates={templatesQuery.data}
				templateUpdatePermissions={
					templateUpdatePermissionsQuery.data ??
					(organizations.length === 0 ? {} : undefined)
				}
				workspacePermissions={workspacePermissionsQuery.data}
			/>
		</>
	);
};

export default TemplatesPage;
