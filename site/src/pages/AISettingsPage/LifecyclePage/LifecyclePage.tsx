import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatAutoArchiveDays,
	chatDebugLogging,
	chatDebugRetentionDays,
	chatRetentionDays,
	chatWorkspaceTTL,
	updateChatAutoArchiveDays,
	updateChatDebugLogging,
	updateChatDebugRetentionDays,
	updateChatRetentionDays,
	updateChatWorkspaceTTL,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { LifecyclePageView } from "./LifecyclePageView";

const LifecyclePage: FC = () => {
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
	const autoArchiveDaysQuery = useQuery({
		...chatAutoArchiveDays(),
		enabled: permissions.editDeploymentConfig,
	});
	const debugRetentionDaysQuery = useQuery({
		...chatDebugRetentionDays(),
		enabled: permissions.editDeploymentConfig,
	});
	const debugLoggingQuery = useQuery({
		...chatDebugLogging(),
		enabled: permissions.editDeploymentConfig,
	});
	const saveWorkspaceTTLMutation = useMutation(
		updateChatWorkspaceTTL(queryClient),
	);
	const saveRetentionDaysMutation = useMutation(
		updateChatRetentionDays(queryClient),
	);
	const saveAutoArchiveDaysMutation = useMutation(
		updateChatAutoArchiveDays(queryClient),
	);
	const saveDebugRetentionDaysMutation = useMutation(
		updateChatDebugRetentionDays(queryClient),
	);
	const saveDebugLoggingMutation = useMutation(
		updateChatDebugLogging(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Lifecycle", "AI Settings")}</title>
			<LifecyclePageView
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
				debugRetentionDaysData={debugRetentionDaysQuery.data}
				isDebugRetentionDaysLoading={debugRetentionDaysQuery.isLoading}
				isDebugRetentionDaysLoadError={debugRetentionDaysQuery.isError}
				onSaveDebugRetentionDays={saveDebugRetentionDaysMutation.mutate}
				isSavingDebugRetentionDays={saveDebugRetentionDaysMutation.isPending}
				isSaveDebugRetentionDaysError={saveDebugRetentionDaysMutation.isError}
				autoArchiveDaysData={autoArchiveDaysQuery.data}
				isAutoArchiveDaysLoading={autoArchiveDaysQuery.isLoading}
				isAutoArchiveDaysLoadError={autoArchiveDaysQuery.isError}
				onSaveAutoArchiveDays={saveAutoArchiveDaysMutation.mutate}
				isSavingAutoArchiveDays={saveAutoArchiveDaysMutation.isPending}
				isSaveAutoArchiveDaysError={saveAutoArchiveDaysMutation.isError}
				debugLoggingData={debugLoggingQuery.data}
				isDebugLoggingLoading={debugLoggingQuery.isLoading}
				onSaveDebugLogging={saveDebugLoggingMutation.mutate}
				isSavingDebugLogging={saveDebugLoggingMutation.isPending}
				isSaveDebugLoggingError={saveDebugLoggingMutation.isError}
			/>
		</RequirePermission>
	);
};

export default LifecyclePage;
