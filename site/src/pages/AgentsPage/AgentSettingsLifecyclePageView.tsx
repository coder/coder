import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { AdminBadge } from "./components/AdminBadge";
import { RetentionPeriodSettings } from "./components/RetentionPeriodSettings";
import { SectionHeader } from "./components/SectionHeader";
import { WorkspaceAutostopSettings } from "./components/WorkspaceAutostopSettings";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

export interface AgentSettingsLifecyclePageViewProps {
	workspaceTTLData: TypesGen.ChatWorkspaceTTLResponse | undefined;
	isWorkspaceTTLLoading: boolean;
	isWorkspaceTTLLoadError: boolean;
	onSaveWorkspaceTTL: (
		req: TypesGen.UpdateChatWorkspaceTTLRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingWorkspaceTTL: boolean;
	isSaveWorkspaceTTLError: boolean;
	retentionDaysData: TypesGen.ChatRetentionDaysResponse | undefined;
	isRetentionDaysLoading: boolean;
	isRetentionDaysLoadError: boolean;
	onSaveRetentionDays: (
		req: TypesGen.UpdateChatRetentionDaysRequest,
		options?: MutationCallbacks,
	) => void;
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
				badge={<AdminBadge />}
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
