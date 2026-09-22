import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModels,
	organizationChatModelOverrides,
	updateChatModel,
	updateOrganizationChatModelOverride,
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
	"compaction",
	"advisor",
];

type OrganizationAgentSettingsProps = {
	organization: Organization;
	canEdit: boolean;
	showAdvisor: boolean;
};

export const OrganizationAgentSettings: FC<OrganizationAgentSettingsProps> = ({
	organization,
	canEdit,
	showAdvisor,
}) => (
	<OrganizationAgentSettingsContent
		key={organization.id}
		organization={organization}
		canEdit={canEdit}
		showAdvisor={showAdvisor}
	/>
);

const OrganizationAgentSettingsContent: FC<OrganizationAgentSettingsProps> = ({
	organization,
	canEdit,
	showAdvisor,
}) => {
	const queryClient = useQueryClient();
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
	const compactionMutation = useMutation(
		updateOrganizationChatModelOverride(
			queryClient,
			organization.id,
			"compaction",
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
		compactionMutation,
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
				modelsQuery.data
					? (modelsQuery.data.models.find((model) => model.is_default)?.id ??
						"")
					: undefined
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
			isLoading={modelsQuery.isLoading}
			isOverridesLoading={overridesQuery.isLoading}
			loadError={loadError}
			refetchError={refetchError}
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
		/>
	);
};
