import { cn } from "cn";
import {
	type FC,
	Profiler,
	type ReactNode,
	useEffect,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import {
	chatPromptsQuery,
	refreshChatContext,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { getWorkspaceAgents } from "#/utils/workspace";
import { useChatDraftAttachments } from "../hooks/useChatDraftAttachments";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import { useFileAttachments } from "../hooks/useFileAttachments";
import {
	useWorkspaceFileUploads,
	type WorkspaceFileUpload,
} from "../hooks/useWorkspaceFileUploads";
import {
	getChatFileURL,
	isWorkspaceFileReferencePart,
} from "../utils/chatAttachments";
import {
	getProviderForModelOption,
	resolveCompactionThreshold,
} from "../utils/modelOptions";
import { CHAT_SLASH_COMMANDS } from "../utils/slashCommands";
import {
	AgentChatInput,
	type AttachedWorkspaceInfo,
	type ChatMessageInputRef,
	isUploadInProgress,
	type UploadState,
} from "./AgentChatInput";
import { ConversationTimeline } from "./ChatConversation/ConversationTimeline";
import type { ChatDetailError } from "./ChatConversation/chatError";
import { getLatestContextUsage } from "./ChatConversation/chatHelpers";
import {
	isActiveChatStatus,
	selectChatStatus,
	selectHasStreamState,
	selectIsAwaitingFirstStreamChunk,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectReconnectState,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	type useChatStore,
} from "./ChatConversation/chatStore";
import {
	LiveStreamTailContent,
	TerminalStatusRow,
} from "./ChatConversation/LiveStreamTail";
import { deriveLiveStatus } from "./ChatConversation/liveStatusModel";
import {
	buildSubagentMaps,
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./ChatConversation/messageParsing";
import { buildStreamTools } from "./ChatConversation/streamState";
import { useOnRenderProfiler } from "./ChatConversation/useOnRenderProfiler";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import { ChatMessageScroller } from "./ChatMessageScroller";
import { getWorkspaceOptionsWithLinkedWorkspace } from "./workspaceOptions";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

// A resolved chat with no context (unpinned) or no resources authoritatively
// has no workspace skills; only an unresolved chat leaves them unknown.
// Duplicate names keep the first resource to match read_skill resolution,
// which also collapses duplicates first-wins in resource order.
export const workspaceSkillsFromChat = (
	chat: TypesGen.Chat | undefined,
): SkillMetadata[] | undefined => {
	if (!chat) {
		return undefined;
	}
	const skills = new Map<string, SkillMetadata>();
	for (const resource of chat.context?.resources ?? []) {
		if (
			resource.kind !== "skill" ||
			resource.status !== "ok" ||
			skills.has(resource.skill_name ?? "")
		) {
			continue;
		}
		skills.set(resource.skill_name ?? "", {
			name: resource.skill_name ?? "",
			description: resource.skill_description ?? "",
		});
	}
	return [...skills.values()];
};

type ChatPageTimelineProps = {
	organizationId: string | undefined;
	store: ChatStoreHandle;
	chatFiles?: readonly TypesGen.ChatFileMetadata[];
	persistedError: ChatDetailError | undefined;
	initialActiveTurnMaxMessageId?: number;
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	isHydratingMessages: boolean;
	hasFetchMoreError: boolean;
	onFetchMoreMessages: () => Promise<unknown>;
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
	footer?: ReactNode;
};

export const ChatPageTimeline: FC<ChatPageTimelineProps> = ({
	organizationId,
	store,
	chatFiles,
	persistedError,
	initialActiveTurnMaxMessageId,
	hasMoreMessages,
	isFetchingMoreMessages,
	isHydratingMessages,
	hasFetchMoreError,
	onFetchMoreMessages,
	onEditUserMessage,
	editingMessageId,
	onImplementPlan,
	onSendAskUserQuestionResponse,
	urlTransform,
	mcpServers,
	footer,
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
	const streamState = useChatSelector(store, selectStreamState);
	const streamError = useChatSelector(store, selectStreamError);
	const retryState = useChatSelector(store, selectRetryState);
	const reconnectState = useChatSelector(store, selectReconnectState);
	const subagentStatusOverrides = useChatSelector(
		store,
		selectSubagentStatusOverrides,
	);
	const isChatCompleted = !hasStream;

	const liveStatus = deriveLiveStatus({
		streamState,
		retryState,
		reconnectState,
		streamError,
		persistedError: persistedError ?? null,
		isAwaitingFirstStreamChunk,
		chatStatus,
	});
	const streamTools = buildStreamTools(
		streamState?.toolCalls,
		streamState?.toolResults,
	);

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
			<ChatMessageScroller
				hasMoreMessages={hasMoreMessages}
				isFetchingMoreMessages={isFetchingMoreMessages}
				isHydratingMessages={isHydratingMessages}
				hasFetchMoreError={hasFetchMoreError}
				hasTranscriptRows={parsedMessages.length > 0}
				onFetchMoreMessages={onFetchMoreMessages}
			>
				{/* VNC sessions for completed agents may already be
					   terminated, so inline desktop previews are disabled
					   via showDesktopPreviews={false} to avoid a perpetual
					   "disconnected" state. The MonitorIcon variant still
					   renders correctly. */}
				<ConversationTimeline
					organizationId={organizationId}
					parsedMessages={parsedMessages}
					chatFiles={chatFiles}
					initialActiveTurnMaxMessageId={initialActiveTurnMaxMessageId}
					streamState={streamState}
					streamTools={streamTools}
					liveStatus={liveStatus}
					subagentStatusOverrides={subagentStatusOverrides}
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
				<TerminalStatusRow liveStatus={liveStatus} />
			</ChatMessageScroller>
			{/* The empty state sits outside the scroller content, which holds
			    transcript rows only. */}
			<div className={cn("mx-auto w-full px-4", chatWidthClass(chatFullWidth))}>
				<LiveStreamTailContent
					isTranscriptEmpty={parsedMessages.length === 0}
					liveStatus={liveStatus}
				/>
				{footer}
			</div>
		</Profiler>
	);
};

export type PendingAttachment = {
	fileId: string;
	mediaType: string;
};

export type PendingWorkspaceUpload = {
	path: string;
	name: string;
	size: number;
	mediaType: string;
	// The workspace whose filesystem holds the bytes, echoed back to
	// the server which rejects references from a stale binding.
	workspaceId: string;
};

export type SendChatMessageOptions = {
	message: string;
	attachments?: readonly PendingAttachment[];
	workspaceUploads?: readonly PendingWorkspaceUpload[];
};

type ChatPageInputProps = {
	chat: TypesGen.Chat;
	store: ChatStoreHandle;
	models: readonly TypesGen.ChatModel[] | undefined;
	onSend: (options: SendChatMessageOptions) => Promise<void> | void;
	onDeleteQueuedMessage: (id: number) => Promise<void>;
	onPromoteQueuedMessage: (id: number) => Promise<void>;
	onInterrupt: () => void;
	isInputDisabled: boolean;
	isReadOnly?: boolean;
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
	onCancelHistoryEdit: () => void;
	// File parts from the message being edited, converted to
	// File objects and pre-populated into attachments.
	editingFileBlocks?: readonly TypesGen.ChatMessagePart[];
	// MCP server picker state.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	onWorkspaceChange?: (workspaceId: string | null) => void;
	isWorkspaceLoading?: boolean;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	sshCommand?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	folder?: string;
};

export const ChatPageInput: FC<ChatPageInputProps> = ({
	chat,
	store,
	models,
	onSend,
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	onInterrupt,
	isInputDisabled,
	isReadOnly = false,
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
	onPlanModeToggle,
	isModelCatalogLoading = false,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
	isEditing,
	onCancelHistoryEdit,
	editingFileBlocks,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	onWorkspaceChange,
	isWorkspaceLoading = false,
	workspace,
	workspaceAgent,
	sshCommand,
	attachedWorkspace,
	folder,
}) => {
	const { user: currentUser } = useAuthenticated();
	const organizationId = chat.organization_id;
	const chatId = chat.id;
	const chatContext = chat.context;
	const planModeEnabled = chat.plan_mode === "plan";
	const selectedWorkspaceId = chat.workspace_id ?? null;
	const workspaceSkills = workspaceSkillsFromChat(chat);
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const workspaceOptions = getWorkspaceOptionsWithLinkedWorkspace(
		workspacesQuery.data?.workspaces ?? [],
		workspace,
		currentUser.id,
	);
	const thresholdsQuery = useQuery(userCompactionThresholds());
	const compressionThreshold = resolveCompactionThreshold(
		chat.last_model_config_id,
		thresholdsQuery.data?.thresholds,
		models,
	);
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

	const rawUsage = getLatestContextUsage(
		messages,
		modelOptions.find((option) => option.id === selectedModel)?.contextLimit,
	);
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
	// Scope on the chat's bound workspace ID (known with the chat
	// record) rather than the async-loaded workspace object, so a
	// rebind resets immediately and query resolution never does.
	const composeWorkspaceUploads = useWorkspaceFileUploads(
		chatId,
		selectedWorkspaceId ?? undefined,
	);
	const editWorkspaceUploads = useWorkspaceFileUploads(
		chatId,
		selectedWorkspaceId ?? undefined,
	);
	// Workspace file references preserved from the message being
	// edited. They are already uploaded; editing only re-references
	// them (or drops them when the chip is removed).
	const [preservedWorkspaceUploads, setPreservedWorkspaceUploads] = useState<
		readonly WorkspaceFileUpload[]
	>([]);
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

	const { reset: resetEditWorkspaceUploads } = editWorkspaceUploads;

	useEffect(() => {
		const previous = editScopeRef.current;
		const scopeChanged =
			previous.organizationId !== organizationId || previous.chatId !== chatId;
		editScopeRef.current = { organizationId, chatId };
		if (scopeChanged) {
			resetEditAttachments();
			setPreservedWorkspaceUploads([]);
		}
	}, [organizationId, chatId, resetEditAttachments]);

	// Preserved references point at files inside the bound workspace's
	// filesystem. Rebinding or clearing the workspace mid-edit makes
	// them unreadable for the agent, so drop them (regular attachments
	// are chat files and survive workspace changes). Scope tracks the
	// chat's bound workspace ID, not the async-loaded workspace
	// object, which resolves after mount without a rebind.
	const editWorkspaceScopeRef = useRef(selectedWorkspaceId);
	useEffect(() => {
		if (editWorkspaceScopeRef.current === selectedWorkspaceId) {
			return;
		}
		editWorkspaceScopeRef.current = selectedWorkspaceId;
		setPreservedWorkspaceUploads([]);
	}, [selectedWorkspaceId]);

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
			const mt = block.media_type;
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

	// Workspace file references from the edited message become
	// preserved (already-uploaded) entries so they survive the
	// edit unless explicitly removed. Only references uploaded to
	// the currently bound workspace qualify: after a rebind the
	// old paths are unreadable for the agent and the server
	// rejects them. Compare against the chat's bound workspace ID
	// (available with the chat record) rather than the
	// async-loaded workspace object, whose late resolution would
	// otherwise drop references hydrated before it arrived.
	// Kept separate from the attachment hydration above and keyed on
	// the blocks reference plus the workspace binding: a rebind mid-edit
	// only recomputes the references (ordinary attachments added during
	// the edit survive), a failed edit submission restores the same
	// array on rollback (same key, no re-run, so uploads added mid-edit
	// survive), and rebinding the workspace away and back re-runs
	// preservation so references valid for the restored binding
	// reappear. Editing a different message passes a fresh array.
	const hydratedEditBlocksRef = useRef<{
		blocks: readonly TypesGen.ChatMessagePart[] | null;
		workspaceId: string | null;
	} | null>(null);
	useEffect(() => {
		if (!isEditing) {
			return;
		}
		if (
			hydratedEditBlocksRef.current !== null &&
			hydratedEditBlocksRef.current.blocks === (editingFileBlocks ?? null) &&
			hydratedEditBlocksRef.current.workspaceId === selectedWorkspaceId
		) {
			return;
		}
		hydratedEditBlocksRef.current = {
			blocks: editingFileBlocks ?? null,
			workspaceId: selectedWorkspaceId,
		};
		resetEditWorkspaceUploads();
		setPreservedWorkspaceUploads(
			(editingFileBlocks ?? [])
				.filter(isWorkspaceFileReferencePart)
				.filter(
					(part) =>
						selectedWorkspaceId !== null &&
						part.workspace_file_workspace_id === selectedWorkspaceId,
				)
				.map(
					(part, i): WorkspaceFileUpload => ({
						id: `preserved-${i}-${part.workspace_file_path}`,
						file: new File([], part.workspace_file_name, {
							type:
								part.workspace_file_media_type || "application/octet-stream",
						}),
						status: "uploaded",
						response: {
							path: part.workspace_file_path,
							name: part.workspace_file_name,
							size: part.workspace_file_size,
							media_type:
								part.workspace_file_media_type || "application/octet-stream",
							workspace_id: part.workspace_file_workspace_id,
						},
					}),
				),
		);
	}, [
		isEditing,
		editingFileBlocks,
		selectedWorkspaceId,
		resetEditWorkspaceUploads,
	]);

	// Exiting edit mode should only clear the edit bucket. Compose draft
	// attachments must survive canceling or completing an edit.
	useEffect(() => {
		if (isEditing) {
			wasEditingRef.current = true;
			return;
		}
		if (!wasEditingRef.current) {
			return;
		}
		// History edits clear isEditing before the edit mutation
		// settles and restore it on failure. Defer cleanup until the
		// submission resolves so a failed edit keeps its uploads and
		// attachments for retry.
		if (isSendPending) {
			return;
		}
		wasEditingRef.current = false;
		hydratedEditBlocksRef.current = null;
		resetEditAttachments();
		resetEditWorkspaceUploads();
		setPreservedWorkspaceUploads([]);
	}, [
		isEditing,
		isSendPending,
		resetEditAttachments,
		resetEditWorkspaceUploads,
	]);

	const isStreaming = hasStreamState || isActiveChatStatus(chatStatus);

	// The workspace upload affordance requires an existing chat bound
	// to a workspace whose agent is connected; the agent writes the
	// bytes into its home directory. A freshly attached or rebound
	// workspace has no bound agent until the next generation, and the
	// upload handler then selects one itself, so any connected root
	// agent qualifies in that case (mirrors the new-chat page).
	const uploadAgentConnected = workspaceAgent
		? workspaceAgent.status === "connected"
		: workspace !== undefined &&
			getWorkspaceAgents(workspace).some(
				(agent) => !agent.parent_id && agent.status === "connected",
			);
	const canUploadWorkspaceFiles = Boolean(
		chatId && workspace && uploadAgentConnected,
	);
	const modeWorkspaceUploads = isEditing
		? editWorkspaceUploads
		: composeWorkspaceUploads;
	const visibleWorkspaceUploads = isEditing
		? [...preservedWorkspaceUploads, ...editWorkspaceUploads.uploads]
		: composeWorkspaceUploads.uploads;
	const handleRemoveWorkspaceUpload = (id: string) => {
		if (
			isEditing &&
			preservedWorkspaceUploads.some((upload) => upload.id === id)
		) {
			setPreservedWorkspaceUploads((current) =>
				current.filter((upload) => upload.id !== id),
			);
			return;
		}
		modeWorkspaceUploads.remove(id);
	};

	const inputElement = (
		<AgentChatInput
			onSend={(message) => {
				void (async () => {
					const hasActiveUploads =
						attachments.some((file) =>
							isUploadInProgress(uploadStates.get(file)),
						) ||
						visibleWorkspaceUploads.some(
							(upload) => upload.status === "uploading",
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
					const pendingWorkspaceUploads: PendingWorkspaceUpload[] = [];
					let skippedWorkspaceErrors = 0;
					for (const upload of visibleWorkspaceUploads) {
						if (upload.status === "error") {
							skippedWorkspaceErrors++;
							continue;
						}
						if (upload.status === "uploaded" && upload.response) {
							pendingWorkspaceUploads.push({
								path: upload.response.path,
								name: upload.response.name,
								size: upload.response.size,
								mediaType: upload.response.media_type,
								workspaceId: upload.response.workspace_id,
							});
						}
					}
					if (skippedErrors > 0) {
						toast.warning(
							`${skippedErrors} attachment${skippedErrors > 1 ? "s" : ""} could not be sent (upload failed)`,
						);
					}
					if (skippedWorkspaceErrors > 0) {
						toast.warning(
							`${skippedWorkspaceErrors} workspace file${skippedWorkspaceErrors > 1 ? "s" : ""} could not be sent (upload failed)`,
						);
					}
					const attachmentsArg =
						pendingAttachments.length > 0 ? pendingAttachments : undefined;
					const workspaceUploadsArg =
						pendingWorkspaceUploads.length > 0
							? pendingWorkspaceUploads
							: undefined;
					try {
						await onSend({
							message,
							attachments: attachmentsArg,
							workspaceUploads: workspaceUploadsArg,
						});
					} catch {
						// Attachments preserved for retry on failure.
						return;
					}
					if (isEditing) {
						editAttachments.resetAttachments();
						resetEditWorkspaceUploads();
						setPreservedWorkspaceUploads([]);
					} else {
						composeAttachments.resetAttachments();
						composeWorkspaceUploads.reset();
					}
				})();
			}}
			attachments={attachments}
			onAttach={handleAttach}
			onRemoveAttachment={handleRemoveAttachment}
			uploadStates={uploadStates}
			previewUrls={previewUrls}
			textContents={textContents}
			workspaceUploads={{
				uploads: visibleWorkspaceUploads,
				onAttach: canUploadWorkspaceFiles
					? modeWorkspaceUploads.attach
					: undefined,
				onRemove: handleRemoveWorkspaceUpload,
			}}
			inputRef={inputRef}
			initialValue={initialValue}
			initialEditorState={initialEditorState}
			remountKey={remountKey}
			onContentChange={onContentChange}
			queuedMessages={queuedMessages}
			onDeleteQueuedMessage={onDeleteQueuedMessage}
			onPromoteQueuedMessage={onPromoteQueuedMessage}
			isEditingHistoryMessage={isEditing}
			onCancelHistoryEdit={onCancelHistoryEdit}
			userPromptHistory={userPromptHistory}
			isDisabled={isInputDisabled}
			isReadOnly={isReadOnly}
			isLoading={isSendPending}
			isStreaming={isStreaming}
			onInterrupt={onInterrupt}
			isInterruptPending={isInterruptPending || chatStatus === "interrupting"}
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
			chatOrganizationId={organizationId}
			selectedWorkspaceId={selectedWorkspaceId}
			onWorkspaceChange={onWorkspaceChange}
			isWorkspaceLoading={workspacesQuery.isLoading || isWorkspaceLoading}
			mcpServers={mcpServers}
			selectedMCPServerIds={selectedMCPServerIds}
			onMCPSelectionChange={onMCPSelectionChange}
			onMCPAuthComplete={onMCPAuthComplete}
			workspaceSkills={workspaceSkills}
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
			// Commands act on the whole chat, so they only make sense
			// for new sends: hide them while editing a history message.
			slashCommands={isEditing ? undefined : CHAT_SLASH_COMMANDS}
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
