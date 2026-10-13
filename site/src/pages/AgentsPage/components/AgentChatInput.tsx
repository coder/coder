import { InfoIcon } from "lucide-react";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import type { ChatSlashCommand } from "../utils/slashCommands";
import { AgentComposer } from "./AgentComposer";
import type { ComposerFileBindings } from "./AgentComposer/context";
import type { AttachedWorkspaceInfo } from "./AgentComposerBadges";
import {
	AgentComposerContainer,
	AgentComposerContextIndicator,
	AgentComposerSetupNotice,
	needsAgentSetup,
} from "./AgentComposerLayout";
import { AgentComposerOptions } from "./AgentComposerOptions";
import type { UploadState } from "./AttachmentPreview";
import type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import type { AgentContextUsage } from "./ContextUsageIndicator";
import { QueuedMessagesList } from "./QueuedMessagesList";

export type { AttachedWorkspaceInfo } from "./AgentComposerBadges";
export {
	ImageThumbnail,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
export type { AgentContextUsage } from "./ContextUsageIndicator";

type AgentChatInputProps = {
	onSend: (message: string) => void;
	placeholder?: string;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading: boolean;
	// Ref for the Lexical editor, exposed for imperative access.
	inputRef?: React.Ref<ChatMessageInputRef>;
	// Initial text to seed the editor on first mount only.
	initialValue: string;
	// Serialized Lexical editor state for restoring drafts with
	// file-reference chips. Takes precedence over initialValue.
	initialEditorState?: string;
	// Monotonic counter to force editor remount.
	remountKey?: number;
	// Called on every content change inside the editor.
	onContentChange: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
	// Model selector.
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	planModeEnabled: boolean;
	onPlanModeToggle: (enabled: boolean) => void;
	isModelCatalogLoading: boolean;
	// Streaming controls (optional, for the detail page).
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	// Workspace picker.
	workspaceOptions?: ReadonlyArray<{
		id: string;
		name: string;
		owner_name: string;
		organization_id: string;
	}>;
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (id: string | null) => void;
	// Organization ID of the current chat. When set, workspaces from
	// other organizations are shown as disabled in the picker.
	chatOrganizationId?: string;
	isWorkspaceLoading?: boolean;
	// Queued user messages rendered above the textarea.
	queuedMessages?: readonly ChatQueuedMessage[];
	onDeleteQueuedMessage?: (id: number) => Promise<void> | void;
	onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
	// Caution shown at the top of the composer, owned by the parent.
	warning?: string;
	// When set, also hides the editor and every control (view-only mode).
	readOnlyNotice?: string;
	// History editing state, owned by the parent.
	isEditingHistoryMessage?: boolean;
	onCancelHistoryEdit?: () => void;
	// Newest-first list of non-empty user prompts for local history cycling.
	userPromptHistory?: readonly string[];

	// Optional context-usage summary shown to the left of the send button.
	// Fields the provider has not reported render as fallback values.
	// Omit entirely to hide the indicator.
	contextUsage?: AgentContextUsage;
	// Re-pins the chat to the workspace's latest context snapshot,
	// surfaced by the context indicator when the pinned context has
	// drifted.
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
	attachments?: readonly File[];
	onAttach?: (files: File[]) => void;
	onRemoveAttachment?: (attachment: number | File) => void;
	uploadStates?: Map<File, UploadState>;
	previewUrls?: Map<File, string>;
	textContents?: Map<File, string>;
	workspaceUploads?: ComposerFileBindings["workspaceUploads"];
	onTextPreview?: (
		content: string,
		fileName: string,
		mediaType: string,
	) => void;
	// MCP Server picker.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	workspaceSkills?: readonly SkillMetadata[];
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	chatId?: string;
	sshCommand?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	folder?: string;
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	// AI Gateway is disabled deployment-wide, independent of provider/model
	// configuration. Forces the setup notice regardless of the counts above.
	aiGatewayDisabled?: boolean;
	// Built-in commands offered by the "/" trigger menu ahead of
	// personal skills.
	slashCommands?: readonly ChatSlashCommand[];
	fillWidth?: boolean;
};

export const AgentChatInput: React.FC<AgentChatInputProps> = ({
	onSend,
	placeholder = "Type a message...",
	isDisabled,
	isReadOnly = false,
	isLoading,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
	selectedModel,
	onModelChange,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	reasoningEffort,
	onReasoningEffortChange,
	planModeEnabled,
	onPlanModeToggle,
	isModelCatalogLoading,
	isStreaming = false,
	onInterrupt,
	isInterruptPending = false,
	workspaceOptions,
	selectedWorkspaceId,
	onWorkspaceChange,
	chatOrganizationId,
	isWorkspaceLoading,
	queuedMessages = [],
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	warning,
	readOnlyNotice,
	isEditingHistoryMessage = false,
	onCancelHistoryEdit,
	userPromptHistory = [],
	contextUsage,
	onRefreshContext,
	isRefreshingContext,
	attachments = [],
	onAttach,
	onRemoveAttachment,
	uploadStates,
	previewUrls,
	textContents,
	workspaceUploads,
	onTextPreview,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	workspaceSkills,
	workspace,
	workspaceAgent,
	chatId,
	sshCommand,
	attachedWorkspace,
	folder,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames = [],
	aiGatewayDisabled,
	slashCommands,
	fillWidth = false,
}) => {
	const setup = {
		canConfigureAgentSetup,
		providerCount,
		modelCount,
		unsupportedProviderNames,
		aiGatewayDisabled,
	};
	const showAgentSetupNotice = needsAgentSetup(setup);
	const isReadOnlyView = readOnlyNotice !== undefined;

	// Only a chat-bound workspace counts: an unbound selection (new chat
	// form, or a picked workspace before the first send) has no pinned
	// context to resolve, so treating it as a workspace would leave the
	// menu in the loading state forever.
	const hasSkillsWorkspace = Boolean(attachedWorkspace?.id ?? workspace?.id);

	// With a context indicator beside the send button, the planning badge
	// joins the measured badge row so it can overflow.
	const shouldOverflowPlanningBadge = contextUsage !== undefined;

	// MCP selection changes are offered only for a controlled selection.
	const onMCPServerSelectionChange = selectedMCPServerIds
		? onMCPSelectionChange
		: undefined;

	return (
		<AgentComposer.RuntimeProvider
			needsSetup={showAgentSetupNotice}
			bindings={{
				onSend,
				isDisabled,
				isReadOnly,
				isLoading,
				hasModelOptions,
				warning,
				inputRef,
				initialValue,
				initialEditorState,
				remountKey,
				onContentChange,
				isStreaming,
				onInterrupt,
				isInterruptPending,
				isEditingHistoryMessage,
				onCancelHistoryEdit,
				userPromptHistory,
				files: {
					attachments,
					onAttach,
					onRemoveAttachment,
					uploadStates,
					previewUrls,
					textContents,
					workspaceUploads,
				},
				queuedMessages,
				onPromoteQueuedMessage,
			}}
		>
			<AgentComposerOptions.Provider
				organizationId={chatOrganizationId}
				isViewOnly={isReadOnlyView}
				planning={{ enabled: planModeEnabled, onChange: onPlanModeToggle }}
				mcp={{
					servers: mcpServers ?? [],
					selectedServerIds: selectedMCPServerIds ?? [],
					onSelectionChange: onMCPServerSelectionChange,
					onAuthComplete: onMCPAuthComplete,
				}}
				workspaceSelection={{
					options: workspaceOptions,
					selectedId: selectedWorkspaceId,
					onChange: onWorkspaceChange,
					isLoading: isWorkspaceLoading,
				}}
				linkedWorkspace={{
					workspace,
					agent: workspaceAgent,
					chatId,
					sshCommand,
					attachedWorkspace,
					folder,
				}}
			>
				<AgentComposerContainer fillWidth={fillWidth}>
					{queuedMessages.length > 0 && (
						<QueuedMessagesList
							messages={queuedMessages}
							onDelete={(id) => onDeleteQueuedMessage?.(id)}
							onPromote={(id) => onPromoteQueuedMessage?.(id)}
							className="mb-2"
						/>
					)}
					{showAgentSetupNotice && (
						<AgentComposerSetupNotice
							{...setup}
							organizationId={chatOrganizationId}
						/>
					)}
					<AgentComposer.Frame
						className={
							isReadOnlyView ? "border border-solid border-border" : undefined
						}
					>
						<AgentComposer.Warning />
						{isEditingHistoryMessage && <AgentComposer.EditBanner />}
						<AgentComposer.Attachments onTextPreview={onTextPreview} />
						{isReadOnlyView ? (
							<div
								role="status"
								className="flex items-center gap-2 px-3 py-2 text-[13px] text-content-primary"
							>
								<InfoIcon className="size-4 shrink-0 text-content-link" />
								{readOnlyNotice}
							</div>
						) : (
							<AgentComposer.Editor
								placeholder={placeholder}
								hasWorkspace={hasSkillsWorkspace}
								workspaceSkills={workspaceSkills}
								slashCommands={slashCommands}
							/>
						)}
						<AgentComposer.InvisibleCharacterWarning />
						<AgentComposer.Toolbar>
							<AgentComposerOptions.Frame>
								<AgentComposerOptions.Menu />
								<AgentComposerOptions.Model
									selectedModel={selectedModel}
									onModelChange={onModelChange}
									modelOptions={modelOptions}
									modelSelectorPlaceholder={modelSelectorPlaceholder}
									reasoningEffort={reasoningEffort}
									onReasoningEffortChange={onReasoningEffortChange}
									isModelCatalogLoading={isModelCatalogLoading}
								/>
								{!shouldOverflowPlanningBadge && (
									<AgentComposerOptions.PlanningBadge />
								)}
								<AgentComposerOptions.Badges
									includePlanning={shouldOverflowPlanningBadge}
								/>
							</AgentComposerOptions.Frame>
							<div className="flex shrink-0 items-center gap-2">
								{!isReadOnlyView && <AgentComposer.VoiceInput />}
								{contextUsage !== undefined && (
									<AgentComposerContextIndicator
										usage={contextUsage}
										onRefreshContext={onRefreshContext}
										isRefreshingContext={isRefreshingContext}
									/>
								)}
								{!isReadOnlyView && <AgentComposer.Submit />}
								{!isReadOnlyView && <AgentComposer.Stop />}
								<AgentComposer.InterruptStatus />
							</div>
						</AgentComposer.Toolbar>
					</AgentComposer.Frame>
				</AgentComposerContainer>
			</AgentComposerOptions.Provider>
		</AgentComposer.RuntimeProvider>
	);
};
