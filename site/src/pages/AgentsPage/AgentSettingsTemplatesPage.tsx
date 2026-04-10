import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatTemplateAllowlist,
	updateChatTemplateAllowlist,
} from "#/api/queries/chats";
import { templates } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsTemplatesPageView } from "./AgentSettingsTemplatesPageView";

const AgentSettingsTemplatesPage: FC = () => {
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
			<AgentSettingsTemplatesPageView
				templatesData={templatesQuery.data}
				allowlistData={allowlistQuery.data}
				isLoading={isLoading}
				hasError={Boolean(templatesQuery.error || allowlistQuery.error)}
				onRetry={() => {
					void templatesQuery.refetch();
					void allowlistQuery.refetch();
				}}
				onSaveAllowlist={saveAllowlistMutation.mutate}
				isSaving={saveAllowlistMutation.isPending}
				isSaveError={saveAllowlistMutation.isError}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsTemplatesPage;
