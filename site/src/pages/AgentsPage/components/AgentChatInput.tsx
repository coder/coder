import type React from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import type { AgentComposerBindings } from "./AgentComposer";
import type { AgentComposerSetup } from "./AgentComposerLayout";
import type { AgentComposerOptions } from "./AgentComposerOptions";
import { ChatComposer } from "./AgentComposers";
import type { ChatAutomationNames } from "./ChatConversation/AutomationLabel";
import type { ChatMessageInput } from "./ChatMessageInput/ChatMessageInput";
import type { AgentContextUsage } from "./ContextUsageIndicator";

export type { AttachedWorkspaceInfo } from "./AgentComposerOptions";
export {
	ImageThumbnail,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
export type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";
export type { AgentContextUsage } from "./ContextUsageIndicator";

type AgentChatInputProps = AgentComposerBindings &
	AgentComposerSetup &
	Omit<
		React.ComponentProps<typeof AgentComposerOptions>,
		"onAttachClick" | "showAgentSetupNotice" | "hasContextUsage"
	> & {
		placeholder?: string;
		queuedMessages?: readonly ChatQueuedMessage[];
		automationNames?: ChatAutomationNames;
		onDeleteQueuedMessage?: (id: number) => Promise<void> | void;
		contextUsage?: AgentContextUsage;
		onRefreshContext?: () => void;
		isRefreshingContext?: boolean;
		workspaceSkills?: React.ComponentProps<
			typeof ChatMessageInput
		>["workspaceSkills"];
		slashCommands?: React.ComponentProps<
			typeof ChatMessageInput
		>["slashCommands"];
		fillWidth?: boolean;
	};

const NO_AUTOMATION_NAMES: ChatAutomationNames = {
	names: new Map(),
	status: "settled",
};

/** Compatibility assembly for existing composer stories and integrations. */
export const AgentChatInput = ({
	onSend,
	isDisabled,
	isReadOnly,
	isLoading,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
	hasModelOptions,
	isStreaming,
	onInterrupt,
	isInterruptPending,
	isEditingHistoryMessage,
	onCancelHistoryEdit,
	userPromptHistory,
	queuedMessages = [],
	onPromoteQueuedMessage,
	attachments,
	onAttach,
	onRemoveAttachment,
	uploadStates,
	previewUrls,
	textContents,
	workspaceUploads,
	onTextPreview,
	warning,
	selectedModel,
	onModelChange,
	modelOptions,
	modelSelectorPlaceholder,
	reasoningEffort,
	onReasoningEffortChange,
	planModeEnabled,
	onPlanModeToggle,
	manageAutomationsEnabled,
	onManageAutomationsToggle,
	isModelCatalogLoading,
	workspaceOptions,
	selectedWorkspaceId,
	onWorkspaceChange,
	chatOrganizationId,
	isWorkspaceLoading,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	workspace,
	workspaceAgent,
	chatId,
	sshCommand,
	attachedWorkspace,
	folder,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames,
	aiGatewayDisabled,
	automationNames = NO_AUTOMATION_NAMES,
	onDeleteQueuedMessage,
	contextUsage,
	onRefreshContext,
	isRefreshingContext,
	placeholder,
	workspaceSkills,
	slashCommands,
	fillWidth,
}: AgentChatInputProps) => (
	<ChatComposer
		bindings={{
			onSend,
			isDisabled,
			isReadOnly,
			isLoading,
			inputRef,
			initialValue,
			initialEditorState,
			remountKey,
			onContentChange,
			hasModelOptions,
			isStreaming,
			onInterrupt,
			isInterruptPending,
			isEditingHistoryMessage,
			onCancelHistoryEdit,
			userPromptHistory,
			queuedMessages,
			onPromoteQueuedMessage,
			attachments,
			onAttach,
			onRemoveAttachment,
			uploadStates,
			previewUrls,
			textContents,
			workspaceUploads,
			onTextPreview,
			warning,
		}}
		options={{
			isDisabled,
			selectedModel,
			onModelChange,
			modelOptions,
			modelSelectorPlaceholder,
			reasoningEffort,
			onReasoningEffortChange,
			planModeEnabled,
			onPlanModeToggle,
			manageAutomationsEnabled,
			onManageAutomationsToggle,
			isModelCatalogLoading,
			workspaceOptions,
			selectedWorkspaceId,
			onWorkspaceChange,
			chatOrganizationId,
			isWorkspaceLoading,
			mcpServers,
			selectedMCPServerIds,
			onMCPSelectionChange,
			onMCPAuthComplete,
			workspace,
			workspaceAgent,
			chatId,
			sshCommand,
			attachedWorkspace,
			folder,
		}}
		setup={{
			canConfigureAgentSetup,
			providerCount,
			modelCount,
			unsupportedProviderNames,
			aiGatewayDisabled,
		}}
		queue={{
			messages: queuedMessages,
			automationNames,
			onDelete: (id) => onDeleteQueuedMessage?.(id),
			onPromote: (id) => onPromoteQueuedMessage?.(id),
		}}
		context={
			contextUsage === undefined
				? undefined
				: {
						usage: contextUsage,
						onRefreshContext,
						isRefreshingContext,
					}
		}
		editor={{
			placeholder,
			workspaceSkills,
			slashCommands,
			hasWorkspace: Boolean(attachedWorkspace?.id ?? workspace?.id),
		}}
		fillWidth={fillWidth}
	/>
);
