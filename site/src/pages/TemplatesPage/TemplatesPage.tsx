import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { checkAuthorization } from "#/api/queries/authCheck";
import { deploymentConfig } from "#/api/queries/deployment";
import { workspacePermissionsByOrganization } from "#/api/queries/organizations";
import { templateExamples, templates } from "#/api/queries/templates";
import type { AuthorizationRequest } from "#/api/typesGenerated";
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
	const templateUpdateChecks: AuthorizationRequest["checks"] = {};
	for (const organization of organizations) {
		templateUpdateChecks[organization.id] = {
			object: {
				resource_type: "template",
				organization_id: organization.id,
			},
			action: "update",
		};
	}
	const templateUpdatePermissionsQuery = useQuery({
		...checkAuthorization({ checks: templateUpdateChecks }),
		enabled: organizations.length > 0,
	});
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
				templateUpdatePermissions={templateUpdatePermissionsQuery.data ?? {}}
				workspacePermissions={workspacePermissionsQuery.data}
			/>
		</>
	);
};

export default TemplatesPage;
