import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { AutoArchiveSettings } from "./components/AutoArchiveSettings";
import { DebugRetentionSettings } from "./components/DebugRetentionSettings";
import { RetentionPeriodSettings } from "./components/RetentionPeriodSettings";
import { WorkspaceAutostopSettings } from "./components/WorkspaceAutostopSettings";

export interface LifecyclePageViewProps {
	workspaceTTLData: TypesGen.ChatWorkspaceTTLResponse | undefined;
	isWorkspaceTTLLoading: boolean;
	isWorkspaceTTLLoadError: boolean;
	onSaveWorkspaceTTL: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatWorkspaceTTLRequest,
		unknown
	>;
	isSavingWorkspaceTTL: boolean;
	isSaveWorkspaceTTLError: boolean;
	retentionDaysData: TypesGen.ChatRetentionDaysResponse | undefined;
	isRetentionDaysLoading: boolean;
	isRetentionDaysLoadError: boolean;
	onSaveRetentionDays: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatRetentionDaysRequest,
		unknown
	>;
	isSavingRetentionDays: boolean;
	isSaveRetentionDaysError: boolean;
	debugRetentionDaysData: TypesGen.ChatDebugRetentionDaysResponse | undefined;
	isDebugRetentionDaysLoading: boolean;
	isDebugRetentionDaysLoadError: boolean;
	onSaveDebugRetentionDays: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatDebugRetentionDaysRequest,
		unknown
	>;
	isSavingDebugRetentionDays: boolean;
	isSaveDebugRetentionDaysError: boolean;
	autoArchiveDaysData: TypesGen.ChatAutoArchiveDaysResponse | undefined;
	isAutoArchiveDaysLoading: boolean;
	isAutoArchiveDaysLoadError: boolean;
	onSaveAutoArchiveDays: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatAutoArchiveDaysRequest,
		unknown
	>;
	isSavingAutoArchiveDays: boolean;
	isSaveAutoArchiveDaysError: boolean;
}

export const LifecyclePageView: FC<LifecyclePageViewProps> = ({
	workspaceTTLData,
	isWorkspaceTTLLoading,
	isWorkspaceTTLLoadError,
	onSaveWorkspaceTTL,
	isSavingWorkspaceTTL,
	isSaveWorkspaceTTLError,
	retentionDaysData,
	isRetentionDaysLoading,
	isRetentionDaysLoadError,
	onSaveRetentionDays,
	isSavingRetentionDays,
	isSaveRetentionDaysError,
	debugRetentionDaysData,
	isDebugRetentionDaysLoading,
	isDebugRetentionDaysLoadError,
	onSaveDebugRetentionDays,
	isSavingDebugRetentionDays,
	isSaveDebugRetentionDaysError,
	autoArchiveDaysData,
	isAutoArchiveDaysLoading,
	isAutoArchiveDaysLoadError,
	onSaveAutoArchiveDays,
	isSavingAutoArchiveDays,
	isSaveAutoArchiveDaysError,
}) => {
	return (
		<div className="flex max-w-[1100px] flex-col gap-4">
			<SettingsHeader>
				<SettingsHeaderTitle>Lifecycle</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Control workspace lifecycle and conversation retention.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<div className="flex flex-col gap-8">
				<WorkspaceAutostopSettings
					workspaceTTLData={workspaceTTLData}
					isWorkspaceTTLLoading={isWorkspaceTTLLoading}
					isWorkspaceTTLLoadError={isWorkspaceTTLLoadError}
					onSaveWorkspaceTTL={onSaveWorkspaceTTL}
					isSavingWorkspaceTTL={isSavingWorkspaceTTL}
					isSaveWorkspaceTTLError={isSaveWorkspaceTTLError}
				/>
				<AutoArchiveSettings
					autoArchiveDaysData={autoArchiveDaysData}
					isAutoArchiveDaysLoading={isAutoArchiveDaysLoading}
					isAutoArchiveDaysLoadError={isAutoArchiveDaysLoadError}
					onSaveAutoArchiveDays={onSaveAutoArchiveDays}
					isSavingAutoArchiveDays={isSavingAutoArchiveDays}
					isSaveAutoArchiveDaysError={isSaveAutoArchiveDaysError}
				/>
				<RetentionPeriodSettings
					retentionDaysData={retentionDaysData}
					isRetentionDaysLoading={isRetentionDaysLoading}
					isRetentionDaysLoadError={isRetentionDaysLoadError}
					onSaveRetentionDays={onSaveRetentionDays}
					isSavingRetentionDays={isSavingRetentionDays}
					isSaveRetentionDaysError={isSaveRetentionDaysError}
				/>
				<DebugRetentionSettings
					debugRetentionDaysData={debugRetentionDaysData}
					isDebugRetentionDaysLoading={isDebugRetentionDaysLoading}
					isDebugRetentionDaysLoadError={isDebugRetentionDaysLoadError}
					onSaveDebugRetentionDays={onSaveDebugRetentionDays}
					isSavingDebugRetentionDays={isSavingDebugRetentionDays}
					isSaveDebugRetentionDaysError={isSaveDebugRetentionDaysError}
				/>
			</div>
		</div>
	);
};
