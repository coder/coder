import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatTemplateAllowlist,
	updateChatTemplateAllowlist,
} from "#/api/queries/chats";
import { templates } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

const TemplatesPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const templatesQuery = useQuery(templates());
	const allowlistQuery = useQuery(chatTemplateAllowlist());
	const saveAllowlistMutation = useMutation(
		updateChatTemplateAllowlist(queryClient),
	);

	const isLoading = templatesQuery.isLoading || allowlistQuery.isLoading;

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Templates", "AI Settings")}</title>

			<TemplatesPageView
				templatesData={templatesQuery.data}
				allowlistData={allowlistQuery.data}
				isLoading={isLoading}
				templatesError={templatesQuery.error}
				allowlistError={allowlistQuery.error}
				onRetry={() => {
					void templatesQuery.refetch();
					void allowlistQuery.refetch();
				}}
				onSaveAllowlist={saveAllowlistMutation.mutate}
				isSaving={saveAllowlistMutation.isPending}
				saveError={saveAllowlistMutation.error}
			/>
		</RequirePermission>
	);
};

export default TemplatesPage;
