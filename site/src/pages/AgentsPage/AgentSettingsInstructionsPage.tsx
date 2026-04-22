import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatPlanModeInstructions,
	chatSystemPrompt,
	updateChatPlanModeInstructions,
	updateChatSystemPrompt,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsInstructionsPageView } from "./AgentSettingsInstructionsPageView";

const AgentSettingsInstructionsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const systemPromptQuery = useQuery({
		...chatSystemPrompt(),
		enabled: permissions.editDeploymentConfig,
	});
	const planModeInstructionsQuery = useQuery({
		...chatPlanModeInstructions(),
		enabled: permissions.editDeploymentConfig,
	});
	const saveSystemPromptMutation = useMutation(
		updateChatSystemPrompt(queryClient),
	);
	const savePlanModeInstructionsMutation = useMutation(
		updateChatPlanModeInstructions(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsInstructionsPageView
				systemPromptData={systemPromptQuery.data}
				planModeInstructionsData={planModeInstructionsQuery.data}
				onSaveSystemPrompt={saveSystemPromptMutation.mutate}
				isSavingSystemPrompt={saveSystemPromptMutation.isPending}
				isSaveSystemPromptError={saveSystemPromptMutation.isError}
				onSavePlanModeInstructions={savePlanModeInstructionsMutation.mutate}
				isSavingPlanModeInstructions={
					savePlanModeInstructionsMutation.isPending
				}
				isSavePlanModeInstructionsError={
					savePlanModeInstructionsMutation.isError
				}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsInstructionsPage;
