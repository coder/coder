import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { templates, updateTemplateMeta } from "#/api/queries/templates";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { useTemplatesFilter } from "#/pages/TemplatesPage/TemplatesFilter";
import { pageTitle } from "#/utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

const TemplatesPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const canManageTemplates =
		permissions.editDeploymentConfig && permissions.updateTemplates;
	const [searchParams, setSearchParams] = useSearchParams();
	const filterState = useTemplatesFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		enabled: canManageTemplates,
	});
	const templatesQuery = useQuery({
		...templates({ q: filterState.filter.query }),
		enabled: canManageTemplates,
	});
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
				templates={templatesQuery.data}
				isLoading={templatesQuery.isLoading}
				error={templatesQuery.error}
				onRetry={() => void templatesQuery.refetch()}
				onToggleAgentsAllowed={toggleAgentsAllowed}
				pendingTemplateIDs={pendingTemplateIDs}
			/>
		</RequirePermission>
	);
};

export default TemplatesPage;
