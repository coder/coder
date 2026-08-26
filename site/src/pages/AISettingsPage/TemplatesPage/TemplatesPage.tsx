import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { organizationsPermissions } from "#/api/queries/organizations";
import { templates, updateTemplateMeta } from "#/api/queries/templates";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { useTemplatesFilter } from "#/pages/TemplatesPage/TemplatesFilter";
import { pageTitle } from "#/utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

const TemplatesPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const queryClient = useQueryClient();
	const canManageTemplates = permissions.updateAnyTemplate;
	const organizationPermissionsQuery = useQuery({
		...organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
		enabled: canManageTemplates,
	});
	const authorizedOrganizationIDs = new Set(
		organizations
			.filter(
				(organization) =>
					organizationPermissionsQuery.data?.[organization.id]?.updateTemplates,
			)
			.map((organization) => organization.id),
	);
	const [searchParams, setSearchParams] = useSearchParams();
	const filterState = useTemplatesFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		enabled: canManageTemplates,
	});
	const templatesQuery = useQuery({
		...templates({ q: filterState.filter.query }),
		enabled: canManageTemplates && organizationPermissionsQuery.isSuccess,
	});
	const authorizedTemplates = templatesQuery.data?.filter((template) =>
		authorizedOrganizationIDs.has(template.organization_id),
	);
	const updateTemplateMutation = useMutation(updateTemplateMeta(queryClient));
	const [pendingTemplateIDs, setPendingTemplateIDs] = useState<
		ReadonlySet<string>
	>(new Set());

	const toggleAgentsAllowed = async (
		template: TypesGen.Template,
		agentsAllowed: boolean,
	) => {
		setPendingTemplateIDs((current) => new Set(current).add(template.id));
		try {
			await updateTemplateMutation.mutateAsync({
				template,
				data: { agents_allowed: agentsAllowed },
			});
		} catch (error) {
			toast.error(
				`${template.display_name || template.name} in ${template.organization_display_name || template.organization_name}: ${getErrorMessage(error, "Failed to update whether Coder Agents can create workspaces.")}`,
				{
					description: getErrorDetail(error),
					duration: Number.POSITIVE_INFINITY,
				},
			);
		} finally {
			setPendingTemplateIDs((current) => {
				const next = new Set(current);
				next.delete(template.id);
				return next;
			});
		}
	};

	return (
		<RequirePermission isFeatureVisible={canManageTemplates}>
			<title>{pageTitle("Templates", "AI Settings")}</title>

			<TemplatesPageView
				filterState={filterState}
				templates={authorizedTemplates}
				isLoading={
					organizationPermissionsQuery.isLoading || templatesQuery.isLoading
				}
				error={organizationPermissionsQuery.error ?? templatesQuery.error}
				onRetry={() => void templatesQuery.refetch()}
				onToggleAgentsAllowed={toggleAgentsAllowed}
				pendingTemplateIDs={pendingTemplateIDs}
			/>
		</RequirePermission>
	);
};

export default TemplatesPage;
