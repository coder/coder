import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { RetentionPeriodSettings } from "./components/RetentionPeriodSettings";
import { SectionHeader } from "./components/SectionHeader";
import { WorkspaceAutostopSettings } from "./components/WorkspaceAutostopSettings";

export interface AgentSettingsLifecyclePageViewProps {
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
}

export const AgentSettingsLifecyclePageView: FC<
	AgentSettingsLifecyclePageViewProps
> = ({
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
}) => {
	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Lifecycle"
				description="Control workspace lifecycle and conversation retention."
			/>
			<WorkspaceAutostopSettings
				workspaceTTLData={workspaceTTLData}
				isWorkspaceTTLLoading={isWorkspaceTTLLoading}
				isWorkspaceTTLLoadError={isWorkspaceTTLLoadError}
				onSaveWorkspaceTTL={onSaveWorkspaceTTL}
				isSavingWorkspaceTTL={isSavingWorkspaceTTL}
				isSaveWorkspaceTTLError={isSaveWorkspaceTTLError}
			/>
			<RetentionPeriodSettings
				retentionDaysData={retentionDaysData}
				isRetentionDaysLoading={isRetentionDaysLoading}
				isRetentionDaysLoadError={isRetentionDaysLoadError}
				onSaveRetentionDays={onSaveRetentionDays}
				isSavingRetentionDays={isSavingRetentionDays}
				isSaveRetentionDaysError={isSaveRetentionDaysError}
			/>
		</div>
	);
};
