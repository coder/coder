import { type FC, Profiler, type ReactNode, useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import { chatPromptsQuery, refreshChatContext } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { AgentChatSendShortcut } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { useChatDraftAttachments } from "../hooks/useChatDraftAttachments";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import { useFileAttachments } from "../hooks/useFileAttachments";
import { getChatFileURL } from "../utils/chatAttachments";
import { getProviderForModelOption } from "../utils/modelOptions";
import type { ChatDetailError } from "../utils/usageLimitMessage";
import {
	AgentChatInput,
	type AttachedWorkspaceInfo,
	type ChatMessageInputRef,
	isUploadInProgress,
	type UploadState,
} from "./AgentChatInput";
import { ConversationTimeline } from "./ChatConversation/ConversationTimeline";
import { getLatestContextUsage } from "./ChatConversation/chatHelpers";
import {
	selectChatStatus,
	selectHasStreamState,
	selectIsAwaitingFirstStreamChunk,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	useChatSelector,
	type useChatStore,
} from "./ChatConversation/chatStore";
import { LiveStreamTail } from "./ChatConversation/LiveStreamTail";
import {
	buildSubagentMaps,
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./ChatConversation/messageParsing";
import { useOnRenderProfiler } from "./ChatConversation/useOnRenderProfiler";
import type { ModelSelectorOption } from "./ChatElements";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

interface ChatPageTimelineProps {
	store: ChatStoreHandle;
	persistedError: ChatDetailError | undefined;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const ChatPageTimeline: FC<ChatPageTimelineProps> = ({
	store,
	persistedError,
	onEditUserMessage,
	editingMessageId,
	onImplementPlan,
	onSendAskUserQuestionResponse,
	urlTransform,
	mcpServers,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const hasStream = useChatSelector(store, selectHasStreamState);
	const isAwaitingFirstStreamChunk = useChatSelector(
		store,
		selectIsAwaitingFirstStreamChunk,
	);
	const isChatCompleted = !hasStream;

	const messages = orderedMessageIDs
		.map((messageID) => {
			const message = messagesByID.get(messageID);
			if (!message && process.env.NODE_ENV !== "production") {
				console.warn(
					`[ChatPageContent] orderedMessageIDs contains ID ${messageID} ` +
						"not found in messagesByID. This may indicate a store/cache " +
						"desync bug.",
				);
			}
			return message;
		})
		.filter(isChatMessage);
	const pendingToolCallIDs = getPendingToolCallIDs(messages, chatStatus);
	const parsedMessages = parseMessagesWithMergedTools(messages, {
		pendingToolCallIDs,
	});
	const { titles: subagentTitles, variants: subagentVariants } =
		buildSubagentMaps(parsedMessages);
	const onRenderProfiler = useOnRenderProfiler();

	return (
		<Profiler id="AgentChat" onRender={onRenderProfiler}>
			<div
				data-testid="chat-timeline-wrapper"
				className={cn(
					"mx-auto flex w-full flex-col py-6",
					chatWidthClass(chatFullWidth),
				)}
			>
				{/* VNC sessions for completed agents may already be
					   terminated, so inline desktop previews are disabled
					   via showDesktopPreviews={false} to avoid a perpetual
					   "disconnected" state. The MonitorIcon variant still
					   renders correctly. */}
				<ConversationTimeline
					parsedMessages={parsedMessages}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					onEditUserMessage={onEditUserMessage}
					editingMessageId={editingMessageId}
					onImplementPlan={onImplementPlan}
					onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
					isChatCompleted={isChatCompleted}
					hasActiveStream={hasStream}
					isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
					showDesktopPreviews={false}
				/>
				<LiveStreamTail
					store={store}
					persistedError={persistedError}
					isTranscriptEmpty={parsedMessages.length === 0}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
				/>
			</div>
		</Profiler>
	);
};

export type PendingAttachment = {
	fileId: string;
	mediaType: string;
};

interface ChatPageInputProps {
	// Organization that owns this chat. Used to scope file uploads.
	organizationId: string | undefined;
	store: ChatStoreHandle;
	compressionThreshold: number | undefined;
	onSend: (
		message: string,
		attachments?: readonly PendingAttachment[],
	) => Promise<void> | void;
	sendShortcut: AgentChatSendShortcut;
	onDeleteQueuedMessage: (id: number) => Promise<void>;
	onPromoteQueuedMessage: (id: number) => Promise<void>;
	onInterrupt: () => void;
	isInputDisabled: boolean;
	isSendPending: boolean;
	isInterruptPending: boolean;
	hasModelOptions: boolean;
	selectedModel: string;
	onModelChange: (modelID: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	modelSelectorHelp?: ReactNode;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	aiGatewayDisabled?: boolean;
	planModeEnabled?: boolean;
	onPlanModeToggle?: (enabled: boolean) => void;
	isModelCatalogLoading?: boolean;
	// Imperative editor handle plus the one-time initial draft,
	// owned by the conversation component.
	inputRef?: React.Ref<ChatMessageInputRef>;
	initialValue?: string;
	initialEditorState?: string;
	remountKey?: number;
	onContentChange?: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
	isEditing: boolean;
	editingQueuedMessageID: number | null;
	onStartQueueEdit: (
		id: number,
		text: string,
		fileBlocks: readonly TypesGen.ChatMessagePart[],
	) => void;
	onCancelQueueEdit: () => void;
	isEditingHistoryMessage: boolean;
	onCancelHistoryEdit: () => void;
	// File parts from the message being edited, converted to
	// File objects and pre-populated into attachments.
	editingFileBlocks?: readonly TypesGen.ChatMessagePart[];
	// MCP server picker state.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	// Pinned workspace-context state for the chat, surfaced by the
	// context indicator (dirty marker and pinned resources).
	chatContext?: TypesGen.ChatContext;
	workspaceOptions: readonly TypesGen.Workspace[];
	chatOrganizationId?: string;
	selectedWorkspaceId: string | null;
	onWorkspaceChange?: (workspaceId: string | null) => void;
	isWorkspaceLoading: boolean;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	chatId?: string;
	sshCommand?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	folder?: string;
}

export const ChatPageInput: FC<ChatPageInputProps> = ({
	organizationId,
	store,
	compressionThreshold,
	onSend,
	sendShortcut,
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	onInterrupt,
	isInputDisabled,
	isSendPending,
	isInterruptPending,
	hasModelOptions,
	selectedModel,
	onModelChange,
	modelOptions,
	modelSelectorPlaceholder,
	modelSelectorHelp,
	reasoningEffort,
	onReasoningEffortChange,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames,
	aiGatewayDisabled,
	planModeEnabled,
	onPlanModeToggle,
	isModelCatalogLoading = false,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
	isEditing,
	editingQueuedMessageID,
	onStartQueueEdit,
	onCancelQueueEdit,
	isEditingHistoryMessage,
	onCancelHistoryEdit,
	editingFileBlocks,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	chatContext,
	workspaceOptions,
	chatOrganizationId,
	selectedWorkspaceId,
	onWorkspaceChange,
	isWorkspaceLoading,
	workspace,
	workspaceAgent,
	chatId,
	sshCommand,
	attachedWorkspace,
	folder,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const hasStreamState = useChatSelector(store, selectHasStreamState);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const queuedMessages = useChatSelector(store, selectQueuedMessages);

	const messages = orderedMessageIDs
		.map((messageID) => {
			const message = messagesByID.get(messageID);
			if (!message && process.env.NODE_ENV !== "production") {
				console.warn(
					`[ChatPageContent] orderedMessageIDs contains ID ${messageID} ` +
						"not found in messagesByID. This may indicate a store/cache " +
						"desync bug.",
				);
			}
			return message;
		})
		.filter(isChatMessage);
	// Source the composer's prompt-history cycle from the dedicated /prompts endpoint.
	const { data: promptsData } = useQuery(chatPromptsQuery(chatId ?? ""));
	const userPromptHistory: readonly string[] =
		promptsData?.prompts.map((prompt) => prompt.text) ?? [];

	const rawUsage = getLatestContextUsage(messages);
	const latestContextUsage =
		rawUsage || chatContext
			? {
					...(rawUsage ?? {}),
					compressionThreshold,
					context: chatContext,
				}
			: rawUsage;
	const queryClient = useQueryClient();
	const refreshContextMutation = useMutation(
		refreshChatContext(queryClient, chatId ?? ""),
	);
	const handleRefreshContext = chatId
		? () =>
				refreshContextMutation.mutate(undefined, {
					onSuccess: () => toast.success("Context refreshed."),
					onError: () => toast.error("Failed to refresh context."),
				})
		: undefined;
	const composeAttachments = useChatDraftAttachments(organizationId, chatId, {
		provider: getProviderForModelOption(modelOptions, selectedModel),
	});
	const editAttachments = useFileAttachments(organizationId, {
		provider: getProviderForModelOption(modelOptions, selectedModel),
	});
	const {
		setAttachments: setEditAttachments,
		setPreviewUrls: setEditPreviewUrls,
		setUploadStates: setEditUploadStates,
		resetAttachments: resetEditAttachments,
	} = editAttachments;
	const wasEditingRef = useRef(isEditing);
	const modeAttachments = isEditing ? editAttachments : composeAttachments;
	const {
		attachments,
		textContents,
		uploadStates,
		previewUrls,
		handleAttach,
		handleRemoveAttachment,
	} = modeAttachments;

	// Edit attachments are scoped to the chat being edited, not the compose
	// draft. Clear them when navigation changes the chat scope.
	const editScopeRef = useRef({ organizationId, chatId });

	useEffect(() => {
		const previous = editScopeRef.current;
		const scopeChanged =
			previous.organizationId !== organizationId || previous.chatId !== chatId;
		editScopeRef.current = { organizationId, chatId };
		if (scopeChanged) {
			resetEditAttachments();
		}
	}, [organizationId, chatId, resetEditAttachments]);

	// Pre-populate the edit bucket from existing file blocks only
	// while explicitly editing a message.
	useEffect(() => {
		if (!isEditing) {
			return;
		}
		if (!editingFileBlocks || editingFileBlocks.length === 0) {
			setEditAttachments([]);
			setEditUploadStates(new Map());
			setEditPreviewUrls(new Map());
			return;
		}
		const fileBlocks = editingFileBlocks.filter(
			(b): b is TypesGen.ChatFilePart => b.type === "file",
		);
		const files = fileBlocks.map((block, i) => {
			const mt = block.media_type ?? "application/octet-stream";
			const ext = mt === "text/plain" ? "txt" : (mt.split("/")[1] ?? "png");
			// Empty File used as a Map key only, its content is never
			// read because the existing file_id is reused at send time.
			return new File([], `attachment-${i}.${ext}`, { type: mt });
		});
		setEditAttachments(files);
		setEditPreviewUrls(
			new Map(
				files.map((f, i) => [f, getChatFileURL(fileBlocks[i].file_id ?? "")]),
			),
		);
		const newUploadStates = new Map<File, UploadState>();
		for (const [i, file] of files.entries()) {
			const block = fileBlocks[i];
			if (block.file_id) {
				newUploadStates.set(file, {
					status: "uploaded",
					fileId: block.file_id,
				});
			}
		}
		setEditUploadStates(newUploadStates);
	}, [
		isEditing,
		editingFileBlocks,
		setEditAttachments,
		setEditPreviewUrls,
		setEditUploadStates,
	]);

	// Exiting edit mode should only clear the edit bucket. Compose draft
	// attachments must survive canceling or completing an edit.
	useEffect(() => {
		if (wasEditingRef.current && !isEditing) {
			resetEditAttachments();
		}
		wasEditingRef.current = isEditing;
	}, [isEditing, resetEditAttachments]);

	const isStreaming = hasStreamState || chatStatus === "running";

	const inputElement = (
		<AgentChatInput
			onSend={(message) => {
				void (async () => {
					const hasActiveUploads = attachments.some((file) =>
						isUploadInProgress(uploadStates.get(file)),
					);
					if (hasActiveUploads) {
						toast.warning("Wait for file uploads to finish before sending.");
						return;
					}
					// Collect uploaded attachment metadata for the optimistic
					// transcript builder while keeping the server payload
					// shape unchanged downstream.
					const pendingAttachments: PendingAttachment[] = [];
					let skippedErrors = 0;
					for (const file of attachments) {
						const state = uploadStates.get(file);
						if (state?.status === "error") {
							skippedErrors++;
							continue;
						}
						if (state?.status === "uploaded" && state.fileId) {
							pendingAttachments.push({
								fileId: state.fileId,
								mediaType: file.type || "application/octet-stream",
							});
						}
					}
					if (skippedErrors > 0) {
						toast.warning(
							`${skippedErrors} attachment${skippedErrors > 1 ? "s" : ""} could not be sent (upload failed)`,
						);
					}
					const attachmentArg =
						pendingAttachments.length > 0 ? pendingAttachments : undefined;
					try {
						await onSend(message, attachmentArg);
					} catch {
						// Attachments preserved for retry on failure.
						return;
					}
					if (isEditing) {
						editAttachments.resetAttachments();
					} else {
						composeAttachments.resetAttachments();
					}
				})();
			}}
			sendShortcut={sendShortcut}
			attachments={attachments}
			onAttach={handleAttach}
			onRemoveAttachment={handleRemoveAttachment}
			uploadStates={uploadStates}
			previewUrls={previewUrls}
			textContents={textContents}
			inputRef={inputRef}
			initialValue={initialValue}
			initialEditorState={initialEditorState}
			remountKey={remountKey}
			onContentChange={onContentChange}
			queuedMessages={queuedMessages}
			onDeleteQueuedMessage={onDeleteQueuedMessage}
			onPromoteQueuedMessage={onPromoteQueuedMessage}
			editingQueuedMessageID={editingQueuedMessageID}
			onStartQueueEdit={onStartQueueEdit}
			onCancelQueueEdit={onCancelQueueEdit}
			isEditingHistoryMessage={isEditingHistoryMessage}
			onCancelHistoryEdit={onCancelHistoryEdit}
			userPromptHistory={userPromptHistory}
			isDisabled={isInputDisabled}
			isLoading={isSendPending}
			isStreaming={isStreaming}
			onInterrupt={onInterrupt}
			isInterruptPending={isInterruptPending}
			contextUsage={latestContextUsage}
			onRefreshContext={handleRefreshContext}
			isRefreshingContext={refreshContextMutation.isPending}
			hasModelOptions={hasModelOptions}
			selectedModel={selectedModel}
			onModelChange={onModelChange}
			modelOptions={modelOptions}
			modelSelectorPlaceholder={modelSelectorPlaceholder}
			reasoningEffort={reasoningEffort}
			onReasoningEffortChange={onReasoningEffortChange}
			planModeEnabled={planModeEnabled}
			onPlanModeToggle={onPlanModeToggle}
			isModelCatalogLoading={isModelCatalogLoading}
			workspaceOptions={workspaceOptions}
			chatOrganizationId={chatOrganizationId}
			selectedWorkspaceId={selectedWorkspaceId}
			onWorkspaceChange={onWorkspaceChange}
			isWorkspaceLoading={isWorkspaceLoading}
			mcpServers={mcpServers}
			selectedMCPServerIds={selectedMCPServerIds}
			onMCPSelectionChange={onMCPSelectionChange}
			onMCPAuthComplete={onMCPAuthComplete}
			workspace={workspace}
			workspaceAgent={workspaceAgent}
			chatId={chatId}
			sshCommand={sshCommand}
			attachedWorkspace={attachedWorkspace}
			folder={folder}
			canConfigureAgentSetup={canConfigureAgentSetup}
			providerCount={providerCount}
			modelCount={modelCount}
			unsupportedProviderNames={unsupportedProviderNames}
			aiGatewayDisabled={aiGatewayDisabled}
		/>
	);

	if (!modelSelectorHelp) {
		return inputElement;
	}

	return (
		<div>
			{inputElement}
			<div className="px-3 pt-1 text-2xs text-content-secondary">
				{modelSelectorHelp}
			</div>
		</div>
	);
};
