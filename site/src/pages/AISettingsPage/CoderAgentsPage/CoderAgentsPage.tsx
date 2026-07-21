import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatAdvisorConfig,
	chatComputerUseProvider,
	chatModelConfigs,
	chatModelOverride,
	chatPersonalModelOverridesAdminSettings,
	chatProviderConfigs,
	updateChatAdvisorConfig,
	updateChatComputerUseProvider,
	updateChatModelOverride,
	updateChatPersonalModelOverridesAdminSettings,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { AIResourceOrganizationSelector } from "#/components/AIResourceOrganizationSelector/AIResourceOrganizationSelector";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
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
	const { organization } = useAIResourceOrganization();
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
		...chatModelOverride(organization.name, generalOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const exploreModelOverrideQuery = useQuery({
		...chatModelOverride(organization.name, exploreOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const titleGenerationModelQuery = useQuery({
		...chatModelOverride(organization.name, titleGenerationOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const compactionModelQuery = useQuery({
		...chatModelOverride(organization.name, compactionOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs(organization.name));
	const advisorConfigQuery = useQuery({
		...chatAdvisorConfig(organization.name),
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
		updateChatModelOverride(queryClient),
	);
	const saveTitleGenerationModelMutation = useMutation(
		updateChatModelOverride(queryClient),
	);
	const saveCompactionModelMutation = useMutation(
		updateChatModelOverride(queryClient),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatModelOverride(queryClient),
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
			<AIResourceOrganizationSelector />
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
				onSaveGeneralModelOverride={(req, options) =>
					saveGeneralModelOverrideMutation.mutate(
						{
							organization: organization.name,
							context: generalOverrideContext,
							req,
						},
						options,
					)
				}
				isSavingGeneralModelOverride={
					saveGeneralModelOverrideMutation.isPending
				}
				isSaveGeneralModelOverrideError={
					saveGeneralModelOverrideMutation.isError
				}
				onSaveTitleGenerationModel={(req, options) =>
					saveTitleGenerationModelMutation.mutate(
						{
							organization: organization.name,
							context: titleGenerationOverrideContext,
							req,
						},
						options,
					)
				}
				isSavingTitleGenerationModel={
					saveTitleGenerationModelMutation.isPending
				}
				isSaveTitleGenerationModelError={
					saveTitleGenerationModelMutation.isError
				}
				onSaveCompactionModel={(req, options) =>
					saveCompactionModelMutation.mutate(
						{
							organization: organization.name,
							context: compactionOverrideContext,
							req,
						},
						options,
					)
				}
				isSavingCompactionModel={saveCompactionModelMutation.isPending}
				isSaveCompactionModelError={saveCompactionModelMutation.isError}
				onSaveExploreModelOverride={(req, options) =>
					saveExploreModelOverrideMutation.mutate(
						{
							organization: organization.name,
							context: exploreOverrideContext,
							req,
						},
						options,
					)
				}
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
				onSaveAdvisorConfig={(req, options) =>
					saveAdvisorConfigMutation.mutate(
						{ organization: organization.name, req },
						options,
					)
				}
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
