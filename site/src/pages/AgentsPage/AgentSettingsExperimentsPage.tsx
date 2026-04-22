import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatDebugLogging,
	chatDesktopEnabled,
	updateChatDebugLogging,
	updateChatDesktopEnabled,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsExperimentsPageView } from "./AgentSettingsExperimentsPageView";

const AgentSettingsExperimentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const desktopEnabledQuery = useQuery({
		...chatDesktopEnabled(),
		enabled: permissions.editDeploymentConfig,
	});
	const debugLoggingQuery = useQuery({
		...chatDebugLogging(),
		enabled: permissions.editDeploymentConfig,
	});
	const saveDesktopEnabledMutation = useMutation(
		updateChatDesktopEnabled(queryClient),
	);
	const saveDebugLoggingMutation = useMutation(
		updateChatDebugLogging(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsExperimentsPageView
				desktopEnabledData={desktopEnabledQuery.data}
				onSaveDesktopEnabled={saveDesktopEnabledMutation.mutate}
				isSavingDesktopEnabled={saveDesktopEnabledMutation.isPending}
				isSaveDesktopEnabledError={saveDesktopEnabledMutation.isError}
				debugLoggingData={debugLoggingQuery.data}
				onSaveDebugLogging={saveDebugLoggingMutation.mutate}
				isSavingDebugLogging={saveDebugLoggingMutation.isPending}
				isSaveDebugLoggingError={saveDebugLoggingMutation.isError}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsExperimentsPage;
