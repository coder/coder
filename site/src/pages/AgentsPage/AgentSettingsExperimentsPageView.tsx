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
	onSaveDesktopEnabled: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatDesktopEnabledRequest,
		unknown
	>;
	isSavingDesktopEnabled: boolean;
	isSaveDesktopEnabledError: boolean;
	debugLoggingData: TypesGen.ChatDebugLoggingAdminSettings | undefined;
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
	onSaveDesktopEnabled,
	isSavingDesktopEnabled,
	isSaveDesktopEnabledError,
	debugLoggingData,
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
				onSaveDesktopEnabled={onSaveDesktopEnabled}
				isSavingDesktopEnabled={isSavingDesktopEnabled}
				isSaveDesktopEnabledError={isSaveDesktopEnabledError}
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
				onSaveAdminSetting={onSaveDebugLogging}
				isSavingAdminSetting={isSavingDebugLogging}
				isSaveAdminSettingError={isSaveDebugLoggingError}
			/>
		</div>
	);
};
