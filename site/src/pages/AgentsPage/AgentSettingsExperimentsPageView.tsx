import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { AdminChatDebugLoggingSettings } from "./components/AdminChatDebugLoggingSettings";
import { AdvisorSettings } from "./components/AdvisorSettings";
import { SectionHeader } from "./components/SectionHeader";
import { VirtualDesktopSettings } from "./components/VirtualDesktopSettings";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

export interface AgentSettingsExperimentsPageViewProps {
	desktopEnabledData: TypesGen.ChatDesktopEnabledResponse | undefined;
	isLoadingDesktopEnabled: boolean;
	onSaveDesktopEnabled: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatDesktopEnabledRequest,
		unknown
	>;
	isSavingDesktopEnabled: boolean;
	isSaveDesktopEnabledError: boolean;
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
	debugLoggingData: TypesGen.ChatDebugLoggingAdminSettings | undefined;
	isLoadingDebugLogging: boolean;
	onSaveDebugLogging: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatDebugLoggingAllowUsersRequest,
		unknown
	>;
	isSavingDebugLogging: boolean;
	isSaveDebugLoggingError: boolean;
	advisorConfigData: TypesGen.AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
	modelConfigsData: readonly TypesGen.ChatModelConfig[];
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	isFetchingModelConfigs: boolean;
	onSaveAdvisorConfig: (
		req: TypesGen.UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
	saveAdvisorConfigError: unknown;
}

export const AgentSettingsExperimentsPageView: FC<
	AgentSettingsExperimentsPageViewProps
> = ({
	desktopEnabledData,
	isLoadingDesktopEnabled,
	onSaveDesktopEnabled,
	isSavingDesktopEnabled,
	isSaveDesktopEnabledError,
	computerUseProviderData,
	isLoadingComputerUseProvider,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
	debugLoggingData,
	isLoadingDebugLogging,
	onSaveDebugLogging,
	isSavingDebugLogging,
	isSaveDebugLoggingError,
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	isFetchingModelConfigs,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
}) => {
	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Experiments"
				description="Opt in to experimental features."
			/>
			<VirtualDesktopSettings
				desktopEnabledData={desktopEnabledData}
				isLoadingDesktopEnabled={isLoadingDesktopEnabled}
				onSaveDesktopEnabled={onSaveDesktopEnabled}
				isSavingDesktopEnabled={isSavingDesktopEnabled}
				isSaveDesktopEnabledError={isSaveDesktopEnabledError}
				computerUseProviderData={computerUseProviderData}
				isLoadingComputerUseProvider={isLoadingComputerUseProvider}
				onSaveComputerUseProvider={onSaveComputerUseProvider}
				isSavingComputerUseProvider={isSavingComputerUseProvider}
				computerUseProviderSaveError={computerUseProviderSaveError}
			/>
			<AdvisorSettings
				advisorConfigData={advisorConfigData}
				isAdvisorConfigLoading={isAdvisorConfigLoading}
				isAdvisorConfigFetching={isAdvisorConfigFetching}
				isAdvisorConfigLoadError={isAdvisorConfigLoadError}
				modelConfigs={modelConfigsData}
				modelConfigsError={modelConfigsError}
				isLoadingModelConfigs={isLoadingModelConfigs}
				isFetchingModelConfigs={isFetchingModelConfigs}
				onSaveAdvisorConfig={onSaveAdvisorConfig}
				isSavingAdvisorConfig={isSavingAdvisorConfig}
				isSaveAdvisorConfigError={isSaveAdvisorConfigError}
				saveAdvisorConfigError={saveAdvisorConfigError}
			/>
			<AdminChatDebugLoggingSettings
				adminSettings={debugLoggingData}
				isLoadingAdminSetting={isLoadingDebugLogging}
				onSaveAdminSetting={onSaveDebugLogging}
				isSavingAdminSetting={isSavingDebugLogging}
				isSaveAdminSettingError={isSaveDebugLoggingError}
			/>
		</div>
	);
};
