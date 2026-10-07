import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModels,
	organizationChatModelOverrides,
	organizationChatSystemPrompt,
	updateChatModel,
	updateOrganizationChatModelOverride,
	updateOrganizationChatSystemPrompt,
} from "#/api/queries/chats";
import type {
	ChatModelOverrideContext,
	Organization,
} from "#/api/typesGenerated";
import {
	filterModelsWithEnabledProvider,
	providerInfoByIDFromDescriptors,
} from "#/pages/AgentsPage/utils/modelOptions";
import { splitModelQueryErrors } from "../ModelsPage/organizationModels";
import OrganizationAgentSettingsView, {
	type SaveModelOverride,
} from "./OrganizationAgentSettingsView";

const contexts: readonly ChatModelOverrideContext[] = [
	"general",
	"explore",
	"title_generation",
	"advisor",
];

type OrganizationAgentSettingsProps = {
	organization: Organization;
	canEdit: boolean;
	canViewInstructions: boolean;
	showAdvisor: boolean;
};

export const OrganizationAgentSettings: React.FC<
	OrganizationAgentSettingsProps
> = ({ organization, canEdit, canViewInstructions, showAdvisor }) => (
	<OrganizationAgentSettingsContent
		key={organization.id}
		organization={organization}
		canEdit={canEdit}
		canViewInstructions={canViewInstructions}
		showAdvisor={showAdvisor}
	/>
);

const OrganizationAgentSettingsContent: React.FC<
	OrganizationAgentSettingsProps
> = ({ organization, canEdit, canViewInstructions, showAdvisor }) => {
	const queryClient = useQueryClient();
	const systemPromptQuery = useQuery({
		...organizationChatSystemPrompt(organization.id),
		enabled: canViewInstructions,
	});
	const systemPromptMutation = useMutation(
		updateOrganizationChatSystemPrompt(queryClient, organization.id),
	);
	const modelsQuery = useQuery(chatModels(organization.id));
	const overridesQuery = useQuery(
		organizationChatModelOverrides(organization.id),
	);
	const generalMutation = useMutation(
		updateOrganizationChatModelOverride(
			queryClient,
			organization.id,
			"general",
		),
	);
	const exploreMutation = useMutation(
		updateOrganizationChatModelOverride(
			queryClient,
			organization.id,
			"explore",
		),
	);
	const titleMutation = useMutation(
		updateOrganizationChatModelOverride(
			queryClient,
			organization.id,
			"title_generation",
		),
	);
	const advisorMutation = useMutation(
		updateOrganizationChatModelOverride(
			queryClient,
			organization.id,
			"advisor",
		),
	);
	const mutations = [
		generalMutation,
		exploreMutation,
		titleMutation,
		advisorMutation,
	] as const;
	const defaultModelMutation = useMutation(updateChatModel(queryClient));
	const providerInfoByID = providerInfoByIDFromDescriptors(
		modelsQuery.data?.providers,
	);
	const enabledModels = filterModelsWithEnabledProvider(
		(modelsQuery.data?.models ?? []).filter((model) => model.enabled),
		providerInfoByID,
	);
	// Only the overrides request gates the override rows: when the model
	// catalog fails, the rows must stay rendered with the error inline so a
	// stale override can still be cleared without the catalog.
	const { loadError, refetchError } = splitModelQueryErrors(overridesQuery);
	const systemPromptErrors = splitModelQueryErrors(systemPromptQuery);
	const saveByContext = new Map<ChatModelOverrideContext, SaveModelOverride>();
	for (const [index, context] of contexts.entries()) {
		const mutation = mutations[index];
		if (mutation) {
			saveByContext.set(context, mutation.mutate);
		}
	}

	return (
		<OrganizationAgentSettingsView
			defaultModelID={
				modelsQuery.data?.models.find((model) => model.is_default)?.id
			}
			onSaveDefaultModel={(modelID, options) =>
				defaultModelMutation.mutate(
					{
						organizationId: organization.id,
						modelId: modelID,
						req: { is_default: true },
					},
					options,
				)
			}
			isSavingDefaultModel={defaultModelMutation.isPending}
			isSaveDefaultModelError={defaultModelMutation.isError}
			overrides={overridesQuery.data?.overrides}
			enabledModels={enabledModels}
			providerInfoByID={providerInfoByID}
			isModelsLoading={modelsQuery.isLoading}
			isOverridesLoading={overridesQuery.isLoading}
			overridesLoadError={loadError}
			overridesRefetchError={refetchError}
			modelsError={modelsQuery.error}
			canEdit={canEdit}
			showAdvisor={showAdvisor}
			saveByContext={saveByContext}
			savingContexts={
				new Set(contexts.filter((_, index) => mutations[index]?.isPending))
			}
			errorContexts={
				new Set(contexts.filter((_, index) => mutations[index]?.isError))
			}
			canViewInstructions={canViewInstructions}
			systemPrompt={systemPromptQuery.data?.system_prompt}
			isSystemPromptLoading={systemPromptQuery.isLoading}
			systemPromptLoadError={systemPromptErrors.loadError}
			systemPromptRefetchError={systemPromptErrors.refetchError}
			onSaveSystemPrompt={systemPromptMutation.mutate}
			isSavingSystemPrompt={systemPromptMutation.isPending}
			saveSystemPromptError={systemPromptMutation.error}
			onResetSaveSystemPrompt={systemPromptMutation.reset}
		/>
	);
};
