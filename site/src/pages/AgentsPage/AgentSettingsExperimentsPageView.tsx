import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { AdminChatDebugLoggingSettings } from "./components/AdminChatDebugLoggingSettings";
import { SectionHeader } from "./components/SectionHeader";
import { VirtualDesktopSettings } from "./components/VirtualDesktopSettings";

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
			<AdminChatDebugLoggingSettings
				adminSettings={debugLoggingData}
				onSaveAdminSetting={onSaveDebugLogging}
				isSavingAdminSetting={isSavingDebugLogging}
				isSaveAdminSettingError={isSaveDebugLoggingError}
			/>
		</div>
	);
};
