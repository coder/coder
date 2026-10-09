import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	organizationChatSystemPrompt,
	updateOrganizationChatSystemPrompt,
} from "#/api/queries/chats";
import type { Organization } from "#/api/typesGenerated";
import { splitModelQueryErrors } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { OrganizationInstructionsSettings } from "./OrganizationInstructionsSettings";

type OrganizationInstructionsProps = {
	organization: Organization;
	canEdit: boolean;
};

export const OrganizationInstructions: React.FC<
	OrganizationInstructionsProps
> = ({ organization, canEdit }) => {
	const queryClient = useQueryClient();
	const systemPromptQuery = useQuery(
		organizationChatSystemPrompt(organization.id),
	);
	const systemPromptMutation = useMutation(
		updateOrganizationChatSystemPrompt(queryClient, organization.id),
	);
	const { loadError, refetchError } = splitModelQueryErrors(systemPromptQuery);

	return (
		<OrganizationInstructionsSettings
			systemPrompt={systemPromptQuery.data?.system_prompt}
			isLoading={systemPromptQuery.isLoading}
			loadError={loadError}
			refetchError={refetchError}
			canEdit={canEdit}
			onSave={systemPromptMutation.mutate}
			isSaving={systemPromptMutation.isPending}
			saveError={systemPromptMutation.error}
			onResetSave={systemPromptMutation.reset}
		/>
	);
};
