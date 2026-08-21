import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModels,
	organizationChatModelOverrides,
	updateOrganizationChatModelOverride,
} from "#/api/queries/chats";
import type { ChatModelOverrideContext } from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	filterModelsWithEnabledProvider,
	providerInfoByIDFromDescriptors,
} from "#/pages/AgentsPage/utils/modelOptions";
import { pageTitle } from "#/utils/page";
import {
	splitModelQueryErrors,
	useOrganizationModels,
} from "../organizationModels";
import DefaultsPageView, { type SaveModelOverride } from "./DefaultsPageView";

const contexts: readonly ChatModelOverrideContext[] = [
	"general",
	"explore",
	"title_generation",
	"compaction",
	"advisor",
];

const DefaultsPage: FC = () => {
	const { organization } = useOrganizationModels();
	// Remount on organization change so mutation state (pending saves,
	// errors) from one organization never renders on another's form.
	return <DefaultsPageContent key={organization.id} />;
};

const DefaultsPageContent: FC = () => {
	const queryClient = useQueryClient();
	const { experiments } = useDashboard();
	const { organization, permissions } = useOrganizationModels();
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
	const providerInfoByID = providerInfoByIDFromDescriptors(
		modelsQuery.data?.providers,
	);
	const enabledModels = filterModelsWithEnabledProvider(
		(modelsQuery.data?.models ?? []).filter((model) => model.enabled),
		providerInfoByID,
	);
	const { loadError, refetchError } = splitModelQueryErrors(
		modelsQuery,
		overridesQuery,
	);
	const saveByContext = new Map<ChatModelOverrideContext, SaveModelOverride>();
	for (const [index, context] of contexts.entries()) {
		const mutation = mutations[index];
		if (mutation) {
			saveByContext.set(context, mutation.mutate);
		}
	}

	return (
		<>
			<title>{pageTitle("Defaults & overrides", "AI Settings")}</title>
			<DefaultsPageView
				overrides={overridesQuery.data?.overrides}
				enabledModels={enabledModels}
				providerInfoByID={providerInfoByID}
				isLoading={modelsQuery.isLoading || overridesQuery.isLoading}
				loadError={loadError}
				refetchError={refetchError}
				canEdit={permissions?.editChatModelConfigs ?? false}
				showAdvisor={experiments.includes("chat-advisor")}
				saveByContext={saveByContext}
				savingContexts={
					new Set(contexts.filter((_, index) => mutations[index]?.isPending))
				}
				errorContexts={
					new Set(contexts.filter((_, index) => mutations[index]?.isError))
				}
			/>
		</>
	);
};

export default DefaultsPage;
