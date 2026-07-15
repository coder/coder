import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { templates, updateTemplateMeta } from "#/api/queries/templates";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

const TemplatesPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const canManageTemplates =
		permissions.editDeploymentConfig && permissions.updateTemplates;
	const templatesQuery = useQuery({
		...templates(),
		enabled: canManageTemplates,
	});
	const updateTemplateMutation = useMutation(updateTemplateMeta(queryClient));
	const [pendingTemplateIDs, setPendingTemplateIDs] = useState<
		ReadonlySet<string>
	>(new Set());
	// Errors are tracked per template so a failure on one row is neither
	// overwritten nor cleared by a later toggle on another row.
	const [updateErrors, setUpdateErrors] = useState<
		ReadonlyMap<string, unknown>
	>(new Map());

	const toggleAgentsAllowed = (
		template: TypesGen.Template,
		agentsAllowed: boolean,
	) => {
		setPendingTemplateIDs((current) => new Set(current).add(template.id));
		setUpdateErrors((current) => {
			if (!current.has(template.id)) {
				return current;
			}
			const next = new Map(current);
			next.delete(template.id);
			return next;
		});
		updateTemplateMutation.mutate(
			{
				template,
				data: { agents_allowed: agentsAllowed },
			},
			{
				onError: (error) => {
					setUpdateErrors((current) =>
						new Map(current).set(template.id, error),
					);
				},
				onSettled: () => {
					setPendingTemplateIDs((current) => {
						const next = new Set(current);
						next.delete(template.id);
						return next;
					});
				},
			},
		);
	};

	return (
		<RequirePermission isFeatureVisible={canManageTemplates}>
			<title>{pageTitle("Templates", "AI Settings")}</title>

			<TemplatesPageView
				templates={templatesQuery.data}
				isLoading={templatesQuery.isLoading}
				error={templatesQuery.error}
				onRetry={() => void templatesQuery.refetch()}
				onToggleAgentsAllowed={toggleAgentsAllowed}
				pendingTemplateIDs={pendingTemplateIDs}
				updateErrors={updateErrors}
			/>
		</RequirePermission>
	);
};

export default TemplatesPage;
