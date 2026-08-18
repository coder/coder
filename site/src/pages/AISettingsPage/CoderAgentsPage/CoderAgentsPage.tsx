import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { chatProviderConfigs } from "#/api/queries/aiProviders";
import {
	chatAdvisorConfig,
	chatComputerUseProvider,
	chatModelConfigs,
	chatModelOverride,
	chatPersonalModelOverridesAdminSettings,
	updateChatAdvisorConfig,
	updateChatComputerUseProvider,
	updateChatModelOverride,
	updateChatPersonalModelOverridesAdminSettings,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { providerInfoByIDFromConfigs } from "#/pages/AgentsPage/utils/modelOptions";
import { pageTitle } from "#/utils/page";
import { CoderAgentsPageView } from "./CoderAgentsPageView";

const generalOverrideContext: TypesGen.ChatModelOverrideContext = "general";
const exploreOverrideContext: TypesGen.ChatModelOverrideContext = "explore";
const titleGenerationOverrideContext: TypesGen.ChatModelOverrideContext =
	"title_generation";
const compactionOverrideContext: TypesGen.ChatModelOverrideContext =
	"compaction";

const CoderAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
	const queryClient = useQueryClient();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;
	const showAdvisorSettings = experiments.includes("chat-advisor");
	const showVirtualDesktopSettings = experiments.includes(
		"chat-virtual-desktop",
	);

	const personalModelOverridesAdminSettingsQuery = useQuery({
		...chatPersonalModelOverridesAdminSettings(),
		enabled: canEditDeploymentConfig,
	});
	const generalModelOverrideQuery = useQuery({
		...chatModelOverride(generalOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const exploreModelOverrideQuery = useQuery({
		...chatModelOverride(exploreOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const titleGenerationModelQuery = useQuery({
		...chatModelOverride(titleGenerationOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const compactionModelQuery = useQuery({
		...chatModelOverride(compactionOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const advisorConfigQuery = useQuery({
		...chatAdvisorConfig(),
		enabled: canEditDeploymentConfig && showAdvisorSettings,
	});
	const computerUseProviderQuery = useQuery({
		...chatComputerUseProvider(),
		enabled: canEditDeploymentConfig && showVirtualDesktopSettings,
	});
	const providerConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: canEditDeploymentConfig,
	});
	const savePersonalModelOverridesAdminSettingsMutation = useMutation(
		updateChatPersonalModelOverridesAdminSettings(queryClient),
	);
	const saveGeneralModelOverrideMutation = useMutation(
		updateChatModelOverride(queryClient, generalOverrideContext),
	);
	const saveTitleGenerationModelMutation = useMutation(
		updateChatModelOverride(queryClient, titleGenerationOverrideContext),
	);
	const saveCompactionModelMutation = useMutation(
		updateChatModelOverride(queryClient, compactionOverrideContext),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatModelOverride(queryClient, exploreOverrideContext),
	);
	const saveAdvisorConfigMutation = useMutation(
		updateChatAdvisorConfig(queryClient),
	);
	const saveComputerUseProviderMutation = useMutation(
		updateChatComputerUseProvider(queryClient),
	);

	const providerInfoByID = providerInfoByIDFromConfigs(
		providerConfigsQuery.data,
	);

	return (
		<RequirePermission isFeatureVisible={canEditDeploymentConfig}>
			<title>{pageTitle("Coder Agents", "AI Settings")}</title>
			<CoderAgentsPageView
				adminOverridesData={personalModelOverridesAdminSettingsQuery.data}
				adminOverridesError={personalModelOverridesAdminSettingsQuery.error}
				onRetryAdminOverrides={() => {
					void personalModelOverridesAdminSettingsQuery.refetch();
				}}
				isRetryingAdminOverrides={
					personalModelOverridesAdminSettingsQuery.isFetching
				}
				onSaveAdminOverrides={
					savePersonalModelOverridesAdminSettingsMutation.mutate
				}
				isSavingAdminOverrides={
					savePersonalModelOverridesAdminSettingsMutation.isPending
				}
				isSaveAdminOverridesError={
					savePersonalModelOverridesAdminSettingsMutation.isError
				}
				generalModelOverrideData={generalModelOverrideQuery.data}
				titleGenerationModelOverrideData={titleGenerationModelQuery.data}
				compactionModelOverrideData={compactionModelQuery.data}
				exploreModelOverrideData={exploreModelOverrideQuery.data}
				modelConfigsData={modelConfigsQuery.data}
				providerInfoByID={providerInfoByID}
				modelConfigsError={
					modelConfigsQuery.error ?? providerConfigsQuery.error
				}
				isLoadingModelConfigs={
					modelConfigsQuery.isLoading || providerConfigsQuery.isLoading
				}
				isFetchingModelConfigs={
					modelConfigsQuery.isFetching || providerConfigsQuery.isFetching
				}
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
				showAdvisorSettings={showAdvisorSettings}
				advisorConfigData={advisorConfigQuery.data}
				isAdvisorConfigLoading={advisorConfigQuery.isLoading}
				isAdvisorConfigFetching={advisorConfigQuery.isFetching}
				isAdvisorConfigLoadError={advisorConfigQuery.isError}
				onSaveAdvisorConfig={saveAdvisorConfigMutation.mutate}
				isSavingAdvisorConfig={saveAdvisorConfigMutation.isPending}
				isSaveAdvisorConfigError={saveAdvisorConfigMutation.isError}
				saveAdvisorConfigError={saveAdvisorConfigMutation.error}
				showVirtualDesktopSettings={showVirtualDesktopSettings}
				computerUseProviderData={computerUseProviderQuery.data}
				isLoadingComputerUseProvider={computerUseProviderQuery.isLoading}
				onSaveComputerUseProvider={saveComputerUseProviderMutation.mutate}
				isSavingComputerUseProvider={saveComputerUseProviderMutation.isPending}
				computerUseProviderSaveError={saveComputerUseProviderMutation.error}
			/>
		</RequirePermission>
	);
};

export default CoderAgentsPage;
