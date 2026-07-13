import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { AdvisorSettings } from "#/pages/AgentsPage/components/AdvisorSettings";
import { VirtualDesktopSettings } from "#/pages/AgentsPage/components/VirtualDesktopSettings";
import type { ProviderInfo } from "#/pages/AgentsPage/utils/modelOptions";
import {
	AdminPersonalModelOverridesSettings,
	type SavePersonalModelOverridesAdminSetting,
} from "./components/AdminPersonalModelOverridesSettings";
import {
	type MutationCallbacks,
	SubagentModelOverrideSettings,
} from "./components/SubagentModelOverrideSettings";

type SaveModelOverride = (
	req: TypesGen.UpdateChatModelOverrideRequest,
	options?: MutationCallbacks,
) => void;

export interface CoderAgentsPageViewProps {
	adminOverridesData?: TypesGen.ChatPersonalModelOverridesAdminSettings;
	adminOverridesError?: unknown;
	onRetryAdminOverrides?: () => void;
	isRetryingAdminOverrides?: boolean;
	onSaveAdminOverrides: SavePersonalModelOverridesAdminSetting;
	isSavingAdminOverrides: boolean;
	isSaveAdminOverridesError: boolean;
	generalModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	titleGenerationModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	exploreModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	isFetchingModelConfigs: boolean;
	onSaveGeneralModelOverride?: SaveModelOverride;
	isSavingGeneralModelOverride?: boolean;
	isSaveGeneralModelOverrideError?: boolean;
	onSaveTitleGenerationModel: SaveModelOverride;
	isSavingTitleGenerationModel: boolean;
	isSaveTitleGenerationModelError: boolean;
	onSaveExploreModelOverride: SaveModelOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
	showAdvisorSettings: boolean;
	advisorConfigData: TypesGen.AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
	onSaveAdvisorConfig: (
		req: TypesGen.UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
	saveAdvisorConfigError: unknown;
	showVirtualDesktopSettings: boolean;
	computerUseProviderData: TypesGen.ChatComputerUseProviderResponse | undefined;
	isLoadingComputerUseProvider: boolean;
	onSaveComputerUseProvider: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatComputerUseProviderRequest,
		unknown
	>;
	isSavingComputerUseProvider: boolean;
	computerUseProviderSaveError: Error | null;
}

export const CoderAgentsPageView: FC<CoderAgentsPageViewProps> = ({
	adminOverridesData,
	adminOverridesError,
	onRetryAdminOverrides,
	isRetryingAdminOverrides,
	onSaveAdminOverrides,
	isSavingAdminOverrides,
	isSaveAdminOverridesError,
	generalModelOverrideData,
	titleGenerationModelOverrideData,
	exploreModelOverrideData,
	modelConfigsData,
	providerInfoByID,
	modelConfigsError,
	isLoadingModelConfigs,
	isFetchingModelConfigs,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride = false,
	isSaveGeneralModelOverrideError = false,
	onSaveTitleGenerationModel,
	isSavingTitleGenerationModel,
	isSaveTitleGenerationModelError,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
	showAdvisorSettings,
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
	showVirtualDesktopSettings,
	computerUseProviderData,
	isLoadingComputerUseProvider,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
}) => {
	const enabledModelConfigs = (modelConfigsData ?? []).filter(
		(modelConfig) => modelConfig.enabled,
	);
	const showGeneralModelSection =
		onSaveGeneralModelOverride !== undefined ||
		generalModelOverrideData !== undefined ||
		isSavingGeneralModelOverride ||
		isSaveGeneralModelOverrideError;

	return (
		<div className="flex max-w-4xl flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Coder Agents</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure deployment-wide defaults for Coder Agents and agent-specific
					capabilities.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<div className="flex flex-col gap-6 rounded-lg border border-solid border-border px-6 py-7">
				<AdminPersonalModelOverridesSettings
					adminSettings={adminOverridesData}
					adminSettingsError={adminOverridesError}
					onRetryAdminSettings={onRetryAdminOverrides}
					isRetryingAdminSettings={isRetryingAdminOverrides}
					onSaveAdminSetting={onSaveAdminOverrides}
					isSavingAdminSetting={isSavingAdminOverrides}
					isSaveAdminSettingError={isSaveAdminOverridesError}
				/>
				{showGeneralModelSection && onSaveGeneralModelOverride && (
					<SubagentModelOverrideSettings
						title="General model"
						description="Used by delegated agents that can edit files or run commands."
						modelOverrideData={generalModelOverrideData}
						enabledModelConfigs={enabledModelConfigs}
						providerInfoByID={providerInfoByID}
						modelConfigsError={modelConfigsError}
						isLoading={isLoadingModelConfigs}
						onSaveModelOverride={onSaveGeneralModelOverride}
						isSaving={isSavingGeneralModelOverride}
						isSaveError={isSaveGeneralModelOverrideError}
						saveErrorMessage="Failed to save general model override."
					/>
				)}
				<SubagentModelOverrideSettings
					title="Title generation model"
					description="Leave unset to use Coder's title default, which prefers fast models from configured providers."
					modelOverrideData={titleGenerationModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					providerInfoByID={providerInfoByID}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveTitleGenerationModel}
					isSaving={isSavingTitleGenerationModel}
					isSaveError={isSaveTitleGenerationModelError}
					saveErrorMessage="Failed to save title generation model."
					unsetPlaceholder="Use title default"
					unavailableModelWarning="The selected model is currently unavailable. Title generation will be skipped until you choose another model or clear this setting."
				/>
				<SubagentModelOverrideSettings
					title="Explore subagent model"
					description="Used for read-only codebase exploration before work returns to the main agent."
					modelOverrideData={exploreModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					providerInfoByID={providerInfoByID}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveExploreModelOverride}
					isSaving={isSavingExploreModelOverride}
					isSaveError={isSaveExploreModelOverrideError}
					saveErrorMessage="Failed to save Explore model override."
				/>
				{showVirtualDesktopSettings && (
					<VirtualDesktopSettings
						computerUseProviderData={computerUseProviderData}
						isLoadingComputerUseProvider={isLoadingComputerUseProvider}
						onSaveComputerUseProvider={onSaveComputerUseProvider}
						isSavingComputerUseProvider={isSavingComputerUseProvider}
						computerUseProviderSaveError={computerUseProviderSaveError}
					/>
				)}
				{showAdvisorSettings && (
					<AdvisorSettings
						advisorConfigData={advisorConfigData}
						isAdvisorConfigLoading={isAdvisorConfigLoading}
						isAdvisorConfigFetching={isAdvisorConfigFetching}
						isAdvisorConfigLoadError={isAdvisorConfigLoadError}
						modelConfigs={modelConfigsData ?? []}
						modelConfigsError={modelConfigsError}
						isLoadingModelConfigs={isLoadingModelConfigs}
						isFetchingModelConfigs={isFetchingModelConfigs}
						onSaveAdvisorConfig={onSaveAdvisorConfig}
						isSavingAdvisorConfig={isSavingAdvisorConfig}
						isSaveAdvisorConfigError={isSaveAdvisorConfigError}
						saveAdvisorConfigError={saveAdvisorConfigError}
					/>
				)}
			</div>
		</div>
	);
};
