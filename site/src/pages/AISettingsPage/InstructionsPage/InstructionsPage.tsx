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
import { pageTitle } from "#/utils/page";
import { InstructionsPageView } from "./InstructionsPageView";

const InstructionsPage: FC = () => {
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
			<title>{pageTitle("Instructions", "AI Settings")}</title>

			<InstructionsPageView
				systemPromptData={systemPromptQuery.data}
				planModeInstructionsData={planModeInstructionsQuery.data}
				onSaveSystemPrompt={saveSystemPromptMutation.mutateAsync}
				onSavePlanModeInstructions={
					savePlanModeInstructionsMutation.mutateAsync
				}
				onResetSystemPromptSave={saveSystemPromptMutation.reset}
				onResetPlanModeInstructionsSave={savePlanModeInstructionsMutation.reset}
				isSaving={
					saveSystemPromptMutation.isPending ||
					savePlanModeInstructionsMutation.isPending
				}
				isSaveSystemPromptError={saveSystemPromptMutation.isError}
				isSavePlanModeInstructionsError={
					savePlanModeInstructionsMutation.isError
				}
			/>
		</RequirePermission>
	);
};

export default InstructionsPage;
