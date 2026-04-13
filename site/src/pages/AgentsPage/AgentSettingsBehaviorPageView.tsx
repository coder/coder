import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatFullWidthSettings } from "./components/ChatFullWidthSettings";
import { PersonalInstructionsSettings } from "./components/PersonalInstructionsSettings";
import { RetentionPeriodSettings } from "./components/RetentionPeriodSettings";
import { SectionHeader } from "./components/SectionHeader";
import { SystemInstructionsSettings } from "./components/SystemInstructionsSettings";
import { UserCompactionThresholdSettings } from "./components/UserCompactionThresholdSettings";
import { VirtualDesktopSettings } from "./components/VirtualDesktopSettings";
import { WorkspaceAutostopSettings } from "./components/WorkspaceAutostopSettings";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AgentSettingsBehaviorPageViewProps {
	canSetSystemPrompt: boolean;

	// Raw query data
	systemPromptData: TypesGen.ChatSystemPromptResponse | undefined;
	userPromptData: TypesGen.UserChatCustomPrompt | undefined;
	desktopEnabledData: TypesGen.ChatDesktopEnabledResponse | undefined;
	workspaceTTLData: TypesGen.ChatWorkspaceTTLResponse | undefined;
	isWorkspaceTTLLoading: boolean;
	isWorkspaceTTLLoadError: boolean;
	retentionDaysData: TypesGen.ChatRetentionDaysResponse | undefined;
	isRetentionDaysLoading: boolean;
	isRetentionDaysLoadError: boolean;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;

	// Thresholds (passed through to child component)
	thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
	isThresholdsLoading: boolean;
	thresholdsError: unknown;
	onSaveThreshold: (
		modelConfigId: string,
		thresholdPercent: number,
	) => Promise<unknown>;
	onResetThreshold: (modelConfigId: string) => Promise<unknown>;

	// Mutation handlers
	onSaveSystemPrompt: (
		req: TypesGen.UpdateChatSystemPromptRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingSystemPrompt: boolean;
	isSaveSystemPromptError: boolean;

	onSaveUserPrompt: (
		req: TypesGen.UserChatCustomPrompt,
		options?: MutationCallbacks,
	) => void;
	isSavingUserPrompt: boolean;
	isSaveUserPromptError: boolean;

	onSaveDesktopEnabled: (
		req: TypesGen.UpdateChatDesktopEnabledRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingDesktopEnabled: boolean;
	isSaveDesktopEnabledError: boolean;

	onSaveWorkspaceTTL: (
		req: TypesGen.UpdateChatWorkspaceTTLRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingWorkspaceTTL: boolean;
	isSaveWorkspaceTTLError: boolean;

	onSaveRetentionDays: (
		req: TypesGen.UpdateChatRetentionDaysRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingRetentionDays: boolean;
	isSaveRetentionDaysError: boolean;
}

export const AgentSettingsBehaviorPageView: FC<
	AgentSettingsBehaviorPageViewProps
> = ({
	canSetSystemPrompt,
	systemPromptData,
	userPromptData,
	desktopEnabledData,
	workspaceTTLData,
	isWorkspaceTTLLoading,
	isWorkspaceTTLLoadError,
	retentionDaysData,
	isRetentionDaysLoading,
	isRetentionDaysLoadError,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	thresholds,
	isThresholdsLoading,
	thresholdsError,
	onSaveThreshold,
	onResetThreshold,
	onSaveSystemPrompt,
	isSavingSystemPrompt,
	isSaveSystemPromptError,
	onSaveUserPrompt,
	isSavingUserPrompt,
	isSaveUserPromptError,
	onSaveDesktopEnabled,
	isSavingDesktopEnabled,
	isSaveDesktopEnabledError,
	onSaveWorkspaceTTL,
	isSavingWorkspaceTTL,
	isSaveWorkspaceTTLError,
	onSaveRetentionDays,
	isSavingRetentionDays,
	isSaveRetentionDaysError,
}) => {
	const isAnyPromptSaving = isSavingSystemPrompt || isSavingUserPrompt;

	return (
		<>
			<SectionHeader
				label="Behavior"
				description="Custom instructions that shape how the agent responds in your conversations."
			/>

			<PersonalInstructionsSettings
				userPromptData={userPromptData}
				onSaveUserPrompt={onSaveUserPrompt}
				isSavingUserPrompt={isSavingUserPrompt}
				isSaveUserPromptError={isSaveUserPromptError}
				isAnyPromptSaving={isAnyPromptSaving}
			/>

			<hr className="my-5 border-0 border-t border-solid border-border" />
			<ChatFullWidthSettings />

			<hr className="my-5 border-0 border-t border-solid border-border" />
			<UserCompactionThresholdSettings
				modelConfigs={modelConfigsData ?? []}
				modelConfigsError={modelConfigsError}
				isLoadingModelConfigs={isLoadingModelConfigs}
				thresholds={thresholds}
				isThresholdsLoading={isThresholdsLoading}
				thresholdsError={thresholdsError}
				onSaveThreshold={onSaveThreshold}
				onResetThreshold={onResetThreshold}
			/>

			{/* ── Admin-only settings ── */}
			{canSetSystemPrompt && (
				<>
					<hr className="my-5 border-0 border-t border-solid border-border" />
					<SystemInstructionsSettings
						systemPromptData={systemPromptData}
						onSaveSystemPrompt={onSaveSystemPrompt}
						isSaveSystemPromptError={isSaveSystemPromptError}
						isAnyPromptSaving={isAnyPromptSaving}
					/>
					<hr className="my-5 border-0 border-t border-solid border-border" />
					<VirtualDesktopSettings
						desktopEnabledData={desktopEnabledData}
						onSaveDesktopEnabled={onSaveDesktopEnabled}
						isSavingDesktopEnabled={isSavingDesktopEnabled}
						isSaveDesktopEnabledError={isSaveDesktopEnabledError}
					/>

					<hr className="my-5 border-0 border-t border-solid border-border" />
					<WorkspaceAutostopSettings
						workspaceTTLData={workspaceTTLData}
						isWorkspaceTTLLoading={isWorkspaceTTLLoading}
						isWorkspaceTTLLoadError={isWorkspaceTTLLoadError}
						onSaveWorkspaceTTL={onSaveWorkspaceTTL}
						isSavingWorkspaceTTL={isSavingWorkspaceTTL}
						isSaveWorkspaceTTLError={isSaveWorkspaceTTLError}
					/>

					<hr className="my-5 border-0 border-t border-solid border-border" />
					<RetentionPeriodSettings
						retentionDaysData={retentionDaysData}
						isRetentionDaysLoading={isRetentionDaysLoading}
						isRetentionDaysLoadError={isRetentionDaysLoadError}
						onSaveRetentionDays={onSaveRetentionDays}
						isSavingRetentionDays={isSavingRetentionDays}
						isSaveRetentionDaysError={isSaveRetentionDaysError}
					/>
				</>
			)}
		</>
	);
};
