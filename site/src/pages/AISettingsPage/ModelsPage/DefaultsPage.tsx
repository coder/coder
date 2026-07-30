import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatAIProviderCatalog,
	chatModelConfigsByOrganization,
	chatModelOverride,
	updateChatModelOverride,
} from "#/api/queries/chats";
import { providerConfigFromCatalogEntry } from "#/modules/aiModels/providerStates";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { providerInfoByIDFromConfigs } from "#/pages/AgentsPage/utils/modelOptions";
import { pageTitle } from "#/utils/page";
import DefaultsPageView from "./DefaultsPageView";
import { useOrganizationModels } from "./organizationModels";

const DefaultsPage: FC = () => {
	const { organization } = useOrganizationModels();
	const { experiments } = useDashboard();
	const showAdvisorModelOverride = experiments.includes("chat-advisor");
	const queryClient = useQueryClient();

	const providerCatalogQuery = useQuery(chatAIProviderCatalog());
	const modelConfigsQuery = useQuery(
		chatModelConfigsByOrganization(organization.id),
	);
	const generalModelOverrideQuery = useQuery(
		chatModelOverride(organization.id, "general"),
	);
	const titleGenerationModelOverrideQuery = useQuery(
		chatModelOverride(organization.id, "title_generation"),
	);
	const compactionModelOverrideQuery = useQuery(
		chatModelOverride(organization.id, "compaction"),
	);
	const exploreModelOverrideQuery = useQuery(
		chatModelOverride(organization.id, "explore"),
	);
	const advisorModelOverrideQuery = useQuery({
		...chatModelOverride(organization.id, "advisor"),
		enabled: showAdvisorModelOverride,
	});

	const saveGeneralModelOverrideMutation = useMutation(
		updateChatModelOverride(queryClient, organization.id, "general"),
	);
	const saveTitleGenerationModelMutation = useMutation(
		updateChatModelOverride(queryClient, organization.id, "title_generation"),
	);
	const saveCompactionModelMutation = useMutation(
		updateChatModelOverride(queryClient, organization.id, "compaction"),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatModelOverride(queryClient, organization.id, "explore"),
	);
	const saveAdvisorModelOverrideMutation = useMutation(
		updateChatModelOverride(queryClient, organization.id, "advisor"),
	);

	const providerInfoByID = providerInfoByIDFromConfigs(
		providerCatalogQuery.data?.map(providerConfigFromCatalogEntry),
	);

	return (
		<>
			<title>{pageTitle("Defaults & overrides", "AI Settings")}</title>
			<DefaultsPageView
				generalModelOverrideData={generalModelOverrideQuery.data}
				titleGenerationModelOverrideData={
					titleGenerationModelOverrideQuery.data
				}
				compactionModelOverrideData={compactionModelOverrideQuery.data}
				exploreModelOverrideData={exploreModelOverrideQuery.data}
				advisorModelOverrideData={advisorModelOverrideQuery.data}
				modelConfigsData={modelConfigsQuery.data}
				providerInfoByID={providerInfoByID}
				modelConfigsError={
					modelConfigsQuery.error ?? providerCatalogQuery.error
				}
				isLoadingModelConfigs={modelConfigsQuery.isLoading}
				onSaveGeneralModelOverride={saveGeneralModelOverrideMutation.mutate}
				isSavingGeneralModelOverride={
					saveGeneralModelOverrideMutation.isPending
				}
				isSaveGeneralModelOverrideError={
					saveGeneralModelOverrideMutation.isError
				}
				onSaveTitleGenerationModel={saveTitleGenerationModelMutation.mutate}
				isSavingTitleGenerationModel={
					saveTitleGenerationModelMutation.isPending
				}
				isSaveTitleGenerationModelError={
					saveTitleGenerationModelMutation.isError
				}
				onSaveCompactionModel={saveCompactionModelMutation.mutate}
				isSavingCompactionModel={saveCompactionModelMutation.isPending}
				isSaveCompactionModelError={saveCompactionModelMutation.isError}
				onSaveExploreModelOverride={saveExploreModelOverrideMutation.mutate}
				isSavingExploreModelOverride={
					saveExploreModelOverrideMutation.isPending
				}
				isSaveExploreModelOverrideError={
					saveExploreModelOverrideMutation.isError
				}
				showAdvisorModelOverride={showAdvisorModelOverride}
				onSaveAdvisorModelOverride={saveAdvisorModelOverrideMutation.mutate}
				isSavingAdvisorModelOverride={
					saveAdvisorModelOverrideMutation.isPending
				}
				isSaveAdvisorModelOverrideError={
					saveAdvisorModelOverrideMutation.isError
				}
			/>
		</>
	);
};

export default DefaultsPage;
