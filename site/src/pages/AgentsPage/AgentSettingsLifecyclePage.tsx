import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatRetentionDays,
	chatWorkspaceTTL,
	updateChatRetentionDays,
	updateChatWorkspaceTTL,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsLifecyclePageView } from "./AgentSettingsLifecyclePageView";

const AgentSettingsLifecyclePage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const workspaceTTLQuery = useQuery({
		...chatWorkspaceTTL(),
		enabled: permissions.editDeploymentConfig,
	});
	const retentionDaysQuery = useQuery({
		...chatRetentionDays(),
		enabled: permissions.editDeploymentConfig,
	});
	const saveWorkspaceTTLMutation = useMutation(
		updateChatWorkspaceTTL(queryClient),
	);
	const saveRetentionDaysMutation = useMutation(
		updateChatRetentionDays(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsLifecyclePageView
				workspaceTTLData={workspaceTTLQuery.data}
				isWorkspaceTTLLoading={workspaceTTLQuery.isLoading}
				isWorkspaceTTLLoadError={workspaceTTLQuery.isError}
				onSaveWorkspaceTTL={saveWorkspaceTTLMutation.mutate}
				isSavingWorkspaceTTL={saveWorkspaceTTLMutation.isPending}
				isSaveWorkspaceTTLError={saveWorkspaceTTLMutation.isError}
				retentionDaysData={retentionDaysQuery.data}
				isRetentionDaysLoading={retentionDaysQuery.isLoading}
				isRetentionDaysLoadError={retentionDaysQuery.isError}
				onSaveRetentionDays={saveRetentionDaysMutation.mutate}
				isSavingRetentionDays={saveRetentionDaysMutation.isPending}
				isSaveRetentionDaysError={saveRetentionDaysMutation.isError}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsLifecyclePage;
