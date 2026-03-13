import { API, watchWorkspace } from "api/api";
import {
	chat,
	chatDiffStatus,
	chatModelConfigs,
	chatModels,
	chats,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	interruptChat,
	promoteChatQueuedMessage,
} from "api/queries/chats";
import { deploymentSSHConfig } from "api/queries/deployment";
import { workspaceById, workspaceByIdKey } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { useProxy } from "contexts/ProxyContext";
import {
	getTerminalHref,
	getVSCodeHref,
	openAppInNewWindow,
} from "modules/apps/apps";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import { pageTitle } from "utils/page";
import { portForwardURL } from "utils/portForward";
import {
	AgentChatInput,
	type ChatMessageInputRef,
	type UploadState,
} from "./AgentChatInput";
import {
	selectChatStatus,
	selectHasStreamState,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	useChatStore,
} from "./AgentDetail/ChatContext";
import { ConversationTimeline } from "./AgentDetail/ConversationTimeline";
import {
	getLatestContextUsage,
	getParentChatID,
	getWorkspaceAgent,
} from "./AgentDetail/chatHelpers";
import {
	buildParsedMessageSections,
	buildSubagentTitles,
	parseMessagesWithMergedTools,
} from "./AgentDetail/messageParsing";
import { buildStreamTools } from "./AgentDetail/streamState";
import { useMessageWindow } from "./AgentDetail/useMessageWindow";
import { useWorkspaceCreationWatcher } from "./AgentDetail/useWorkspaceCreationWatcher";
import {
	AgentDetailLoadingView,
	AgentDetailNotFoundView,
	AgentDetailView,
} from "./AgentDetailView";
import type { AgentsOutletContext } from "./AgentsPage";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";
import { useFileAttachments } from "./useFileAttachments";
import { useGitWatcher } from "./useGitWatcher";

/** localStorage key controlling whether the right panel is visible. */
export const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";

const localHosts = new Set(["localhost", "127.0.0.1", "0.0.0.0"]);

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
/** @internal Exported for testing. */
export const draftInputStorageKeyPrefix = "agents.draft-input.";
type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

interface AgentDetailTimelineProps {
	store: ChatStoreHandle;
	chatID: string;
	persistedErrorReason: string | undefined;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly { mediaType: string; data?: string }[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	urlTransform?: UrlTransform;
}

export const AgentDetailTimeline: FC<AgentDetailTimelineProps> = ({
	store,
	chatID,
	persistedErrorReason,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
	urlTransform,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const streamState = useChatSelector(store, selectStreamState);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const streamError = useChatSelector(store, selectStreamError);
	const subagentStatusOverrides = useChatSelector(
		store,
		selectSubagentStatusOverrides,
	);
	const retryState = useChatSelector(store, selectRetryState);

	const messages = useMemo(
		() =>
			orderedMessageIDs
				.map((messageID) => messagesByID.get(messageID))
				.filter(isChatMessage),
		[messagesByID, orderedMessageIDs],
	);
	const streamTools = useMemo(
		() => buildStreamTools(streamState),
		[streamState],
	);
	const { hasMoreMessages, windowedMessages, loadMoreSentinelRef } =
		useMessageWindow({
			messages,
			resetKey: chatID,
		});
	const parsedMessages = useMemo(
		() => parseMessagesWithMergedTools(windowedMessages),
		[windowedMessages],
	);
	const subagentTitles = useMemo(
		() => buildSubagentTitles(parsedMessages),
		[parsedMessages],
	);
	const parsedSections = useMemo(
		() => buildParsedMessageSections(parsedMessages),
		[parsedMessages],
	);
	const detailErrorMessage =
		(chatStatus === "error" ? persistedErrorReason : undefined) || streamError;
	const latestMessage = messages[messages.length - 1];
	const latestMessageNeedsAssistantResponse =
		!latestMessage || latestMessage.role !== "assistant";
	const isAwaitingFirstStreamChunk =
		!streamState &&
		(chatStatus === "running" || chatStatus === "pending") &&
		latestMessageNeedsAssistantResponse;
	const hasStreamOutput = Boolean(streamState) || isAwaitingFirstStreamChunk;

	return (
		<ConversationTimeline
			isEmpty={messages.length === 0}
			hasMoreMessages={hasMoreMessages}
			loadMoreSentinelRef={loadMoreSentinelRef}
			parsedSections={parsedSections}
			hasStreamOutput={hasStreamOutput}
			streamState={streamState}
			streamTools={streamTools}
			subagentTitles={subagentTitles}
			subagentStatusOverrides={subagentStatusOverrides}
			retryState={retryState}
			isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
			detailErrorMessage={detailErrorMessage}
			onEditUserMessage={onEditUserMessage}
			editingMessageId={editingMessageId}
			savingMessageId={savingMessageId}
			urlTransform={urlTransform}
		/>
	);
};

interface AgentDetailInputProps {
	store: ChatStoreHandle;
	compressionThreshold: number | undefined;
	onSend: (message: string, fileIds?: string[]) => void;
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
	inputStatusText: string | null;
	modelCatalogStatusMessage: string | null;
	// Controlled input value and editing state, owned by the
	// conversation component.
	inputRef?: React.Ref<ChatMessageInputRef>;
	initialValue?: string;
	onContentChange?: (content: string) => void;
	editingQueuedMessageID: number | null;
	onStartQueueEdit: (id: number, text: string) => void;
	onCancelQueueEdit: () => void;
	isEditingHistoryMessage: boolean;
	onCancelHistoryEdit: () => void;
	// File blocks from the message being edited, converted to
	// File objects and pre-populated into attachments.
	editingFileBlocks?: readonly {
		mediaType: string;
		data?: string;
		fileId?: string;
	}[];
}

export const AgentDetailInput: FC<AgentDetailInputProps> = ({
	store,
	compressionThreshold,
	onSend,
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
	inputStatusText,
	modelCatalogStatusMessage,
	inputRef,
	initialValue,
	onContentChange,
	editingQueuedMessageID,
	onStartQueueEdit,
	onCancelQueueEdit,
	isEditingHistoryMessage,
	onCancelHistoryEdit,
	editingFileBlocks,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const hasStreamState = useChatSelector(store, selectHasStreamState);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const queuedMessages = useChatSelector(store, selectQueuedMessages);

	const messages = useMemo(
		() =>
			orderedMessageIDs
				.map((messageID) => messagesByID.get(messageID))
				.filter(isChatMessage),
		[messagesByID, orderedMessageIDs],
	);
	const { organizations } = useDashboard();
	const organizationId = organizations[0]?.id;
	const latestContextUsage = useMemo(() => {
		const usage = getLatestContextUsage(messages);
		if (!usage) {
			return usage;
		}
		return { ...usage, compressionThreshold };
	}, [messages, compressionThreshold]);
	const {
		attachments,
		uploadStates,
		previewUrls,
		handleAttach,
		handleRemoveAttachment,
		resetAttachments,
		setAttachments,
		setPreviewUrls,
		setUploadStates,
	} = useFileAttachments(organizationId);
	// Pre-populate attachments from existing file blocks when
	// entering edit mode on a message with images.
	useEffect(() => {
		if (!editingFileBlocks || editingFileBlocks.length === 0) {
			// Clear attachments when exiting edit mode.
			setAttachments([]);
			setUploadStates(new Map());
			setPreviewUrls(new Map());
			return;
		}
		const files = editingFileBlocks.map((block, i) => {
			const ext = block.mediaType.split("/")[1] ?? "png";
			// Empty File used as a Map key only, its content is never
			// read because the existing fileId is reused at send time.
			return new File([], `attachment-${i}.${ext}`, {
				type: block.mediaType,
			});
		});
		setAttachments(files);
		setPreviewUrls(
			new Map(
				files.map((f, i) => [
					f,
					`/api/experimental/chats/files/${editingFileBlocks[i].fileId}`,
				]),
			),
		);
		const newUploadStates = new Map<File, UploadState>();
		for (const [i, file] of files.entries()) {
			const block = editingFileBlocks[i];
			if (block.fileId) {
				newUploadStates.set(file, {
					status: "uploaded",
					fileId: block.fileId,
				});
			}
		}
		setUploadStates(newUploadStates);
	}, [editingFileBlocks, setAttachments, setPreviewUrls, setUploadStates]);

	const isStreaming =
		hasStreamState || chatStatus === "running" || chatStatus === "pending";

	return (
		<AgentChatInput
			onSend={(message) => {
				void (async () => {
					try {
						// Collect file IDs from already-uploaded attachments.
						// Skip files in error state (e.g. too large).
						const fileIds: string[] = [];
						let skippedErrors = 0;
						for (const file of attachments) {
							const state = uploadStates.get(file);
							if (state?.status === "error") {
								skippedErrors++;
								continue;
							}
							if (state?.status === "uploaded" && state.fileId) {
								fileIds.push(state.fileId);
							}
						}
						if (skippedErrors > 0) {
							toast.warning(
								`${skippedErrors} attachment${skippedErrors > 1 ? "s" : ""} could not be sent (upload failed)`,
							);
						}
						await onSend(message, fileIds.length > 0 ? fileIds : undefined);
						resetAttachments();
					} catch {
						// Attachments preserved for retry on failure.
					}
				})();
			}}
			attachments={attachments}
			onAttach={handleAttach}
			onRemoveAttachment={handleRemoveAttachment}
			uploadStates={uploadStates}
			previewUrls={previewUrls}
			inputRef={inputRef}
			initialValue={initialValue}
			onContentChange={onContentChange}
			queuedMessages={queuedMessages}
			onDeleteQueuedMessage={onDeleteQueuedMessage}
			onPromoteQueuedMessage={onPromoteQueuedMessage}
			editingQueuedMessageID={editingQueuedMessageID}
			onStartQueueEdit={onStartQueueEdit}
			onCancelQueueEdit={onCancelQueueEdit}
			isEditingHistoryMessage={isEditingHistoryMessage}
			onCancelHistoryEdit={onCancelHistoryEdit}
			isDisabled={isInputDisabled}
			isLoading={isSendPending}
			isStreaming={isStreaming}
			onInterrupt={onInterrupt}
			isInterruptPending={isInterruptPending}
			contextUsage={latestContextUsage}
			hasModelOptions={hasModelOptions}
			selectedModel={selectedModel}
			onModelChange={onModelChange}
			modelOptions={modelOptions}
			modelSelectorPlaceholder={modelSelectorPlaceholder}
			inputStatusText={inputStatusText}
			modelCatalogStatusMessage={modelCatalogStatusMessage}
		/>
	);
};

/** @internal Exported for testing. */
export function useConversationEditingState(deps: {
	chatID: string | undefined;
	onSend: (
		message: string,
		fileIds?: string[],
		editedMessageID?: number,
	) => Promise<void>;
	onDeleteQueuedMessage: (id: number) => Promise<void>;
	chatInputRef: React.RefObject<ChatMessageInputRef | null>;
	inputValueRef: React.RefObject<string>;
}) {
	const { chatID, onSend, onDeleteQueuedMessage, chatInputRef, inputValueRef } =
		deps;
	const draftStorageKey = chatID
		? `${draftInputStorageKeyPrefix}${chatID}`
		: null;
	const [editorInitialValue, setEditorInitialValue] = useState(() => {
		if (typeof window === "undefined" || !draftStorageKey) {
			return "";
		}
		const saved = localStorage.getItem(draftStorageKey);
		if (saved) {
			inputValueRef.current = saved;
		}
		return saved ?? "";
	});

	// -- History editing state --
	const [editingMessageId, setEditingMessageId] = useState<number | null>(null);
	const [draftBeforeHistoryEdit, setDraftBeforeHistoryEdit] = useState<
		string | null
	>(null);
	const [editingFileBlocks, setEditingFileBlocks] = useState<
		readonly { mediaType: string; data?: string; fileId?: string }[]
	>([]);

	const handleEditUserMessage = useCallback(
		(
			messageId: number,
			text: string,
			fileBlocks?: readonly {
				mediaType: string;
				data?: string;
				fileId?: string;
			}[],
		) => {
			setDraftBeforeHistoryEdit((prev) =>
				editingMessageId !== null ? prev : inputValueRef.current,
			);
			setEditingMessageId(messageId);
			setEditorInitialValue(text);
			inputValueRef.current = text;
			setEditingFileBlocks(fileBlocks ?? []);
		},
		[editingMessageId, inputValueRef],
	);

	const handleCancelHistoryEdit = useCallback(() => {
		setEditorInitialValue(draftBeforeHistoryEdit ?? "");
		inputValueRef.current = draftBeforeHistoryEdit ?? "";
		setEditingMessageId(null);
		setDraftBeforeHistoryEdit(null);
		setEditingFileBlocks([]);
	}, [draftBeforeHistoryEdit, inputValueRef]);

	// -- Queue editing state --
	const [editingQueuedMessageID, setEditingQueuedMessageID] = useState<
		number | null
	>(null);
	const [draftBeforeQueueEdit, setDraftBeforeQueueEdit] = useState<
		string | null
	>(null);

	const handleStartQueueEdit = useCallback(
		(id: number, text: string) => {
			setDraftBeforeQueueEdit((prev) =>
				editingQueuedMessageID === null ? inputValueRef.current : prev,
			);
			setEditingQueuedMessageID(id);
			setEditorInitialValue(text);
			inputValueRef.current = text;
		},
		[editingQueuedMessageID, inputValueRef],
	);

	const handleCancelQueueEdit = useCallback(() => {
		setEditorInitialValue(draftBeforeQueueEdit ?? "");
		inputValueRef.current = draftBeforeQueueEdit ?? "";
		setEditingQueuedMessageID(null);
		setDraftBeforeQueueEdit(null);
	}, [draftBeforeQueueEdit, inputValueRef]);

	// Wraps the parent onSend to clear local input/editing state
	// and handle queue-edit deletion.
	const handleSendFromInput = useCallback(
		async (message: string, fileIds?: string[]) => {
			const editedMessageID =
				editingMessageId !== null ? editingMessageId : undefined;
			const queueEditID = editingQueuedMessageID;

			await onSend(message, fileIds, editedMessageID);
			// Clear input and editing state on success.
			chatInputRef.current?.clear();
			chatInputRef.current?.focus();
			inputValueRef.current = "";
			if (typeof window !== "undefined" && draftStorageKey) {
				localStorage.removeItem(draftStorageKey);
			}
			if (editingMessageId !== null) {
				setEditingMessageId(null);
				setDraftBeforeHistoryEdit(null);
				setEditingFileBlocks([]);
			}
			if (queueEditID !== null) {
				setEditingQueuedMessageID(null);
				setDraftBeforeQueueEdit(null);
				void onDeleteQueuedMessage(queueEditID);
			}
		},
		[
			chatInputRef,
			editingMessageId,
			editingQueuedMessageID,
			onDeleteQueuedMessage,
			onSend,
			draftStorageKey,
			inputValueRef,
		],
	);

	const handleContentChange = useCallback(
		(content: string) => {
			inputValueRef.current = content;
			if (typeof window !== "undefined" && draftStorageKey) {
				if (content) {
					localStorage.setItem(draftStorageKey, content);
				} else {
					localStorage.removeItem(draftStorageKey);
				}
			}
		},
		[draftStorageKey, inputValueRef],
	);

	return {
		inputValueRef,
		chatInputRef,
		editorInitialValue,
		editingMessageId,
		editingFileBlocks,
		handleEditUserMessage,
		handleCancelHistoryEdit,
		editingQueuedMessageID,
		handleStartQueueEdit,
		handleCancelQueueEdit,
		handleSendFromInput,
		handleContentChange,
	};
}

const AgentDetail: FC = () => {
	const navigate = useNavigate();
	const { agentId } = useParams<{ agentId: string }>();
	const outletContext = useOutletContext<AgentsOutletContext>();
	const queryClient = useQueryClient();
	const [selectedModel, setSelectedModel] = useState("");
	const [pendingEditMessageId, setPendingEditMessageId] = useState<
		number | null
	>(null);
	const {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestArchiveAndDeleteWorkspace,
		requestUnarchiveAgent,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
	} = outletContext;
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);
	const chatInputRef = useRef<ChatMessageInputRef | null>(null);
	const inputValueRef = useRef("");

	// Right panel open/closed state is owned here so the loading
	// skeleton and the loaded view share the same layout, preventing
	// a horizontal shift when data arrives.
	const [showSidebarPanel, setShowSidebarPanel] = useState(() => {
		if (typeof window === "undefined") return false;
		return localStorage.getItem(RIGHT_PANEL_OPEN_KEY) === "true";
	});
	const handleSetShowSidebarPanel = useCallback(
		(next: boolean | ((prev: boolean) => boolean)) => {
			setShowSidebarPanel((prev) => {
				const value = typeof next === "function" ? next(prev) : next;
				if (typeof window !== "undefined") {
					localStorage.setItem(RIGHT_PANEL_OPEN_KEY, String(value));
				}
				return value;
			});
		},
		[],
	);

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatsQuery = useQuery(chats());
	const workspaceId = chatQuery.data?.chat?.workspace_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});

	// Subscribe to live workspace updates so that agent status changes
	// (e.g. connected/disconnected) are reflected without a page refresh.
	useEffect(() => {
		if (!workspaceId) {
			return;
		}
		const socket = watchWorkspace(workspaceId);
		socket.addEventListener("message", (event) => {
			if (event.parseError) {
				return;
			}
			if (event.parsedMessage.type === "data") {
				queryClient.setQueryData(
					workspaceByIdKey(workspaceId),
					event.parsedMessage.data as TypesGen.Workspace,
				);
			}
		});
		return () => socket.close();
	}, [workspaceId, queryClient]);
	const diffStatusQuery = useQuery({
		...chatDiffStatus(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const sshConfigQuery = useQuery(deploymentSSHConfig());
	const workspace = workspaceQuery.data;
	const workspaceAgent = getWorkspaceAgent(workspace, undefined);
	const { proxy } = useProxy();

	const urlTransform = useCallback<UrlTransform>(
		(url) => {
			const host = proxy.preferredWildcardHostname;
			if (!host || !workspaceAgent || !workspace) {
				return url;
			}
			try {
				const parsed = new URL(url);
				if (!localHosts.has(parsed.hostname)) {
					return url;
				}
				return portForwardURL(
					host,
					Number.parseInt(parsed.port, 10),
					workspaceAgent.name,
					workspace.name,
					workspace.owner_name,
					"http",
					parsed.pathname,
					parsed.search,
				);
			} catch {
				return url;
			}
		},
		[proxy.preferredWildcardHostname, workspaceAgent, workspace],
	);

	const chatData = chatQuery.data;
	const chatRecord = chatData?.chat;
	const isArchived = chatRecord?.archived ?? false;
	const chatMessages = chatData?.messages;
	const chatQueuedMessages = chatData?.queued_messages;
	const chatLastModelConfigID = chatRecord?.last_model_config_id;

	const modelOptions = useMemo(
		() =>
			getModelOptionsFromCatalog(
				chatModelsQuery.data,
				chatModelConfigsQuery.data,
			),
		[chatModelsQuery.data, chatModelConfigsQuery.data],
	);
	const modelConfigIDByModelID = useMemo(() => {
		const byModelID = new Map<string, string>();
		for (const config of chatModelConfigsQuery.data ?? []) {
			const provider = config.provider.trim().toLowerCase();
			const model = config.model.trim();
			if (!provider || !model) {
				continue;
			}
			const colonRef = `${provider}:${model}`;
			if (!byModelID.has(colonRef)) {
				byModelID.set(colonRef, config.id);
			}
			const slashRef = `${provider}/${model}`;
			if (!byModelID.has(slashRef)) {
				byModelID.set(slashRef, config.id);
			}
		}
		return byModelID;
	}, [chatModelConfigsQuery.data]);
	const modelIDByConfigID = useMemo(() => {
		const byConfigID = new Map<string, string>();
		for (const [modelID, configID] of modelConfigIDByModelID.entries()) {
			if (!byConfigID.has(configID)) {
				byConfigID.set(configID, modelID);
			}
		}
		return byConfigID;
	}, [modelConfigIDByModelID]);

	const sendMutation = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const editMutation = useMutation(editChatMessage(queryClient, agentId ?? ""));
	const interruptMutation = useMutation(
		interruptChat(queryClient, agentId ?? ""),
	);
	const deleteQueuedMutation = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const promoteQueuedMutation = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
	);

	const { store, clearStreamError } = useChatStore({
		chatID: agentId,
		chatMessages,
		chatRecord,
		chatData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	});

	// Git watcher: runs regardless of sidebar visibility, but only
	// connects when the workspace agent is in the "connected" state
	// to avoid an infinite reconnect loop against a missing agent.
	const gitWatcher = useGitWatcher({
		chatId: agentId,
		agentStatus: workspaceAgent?.status,
	});

	// Detect workspace creation so the sidebar can resolve the
	// workspace and display agent/git info.
	useWorkspaceCreationWatcher({
		store,
		chatID: agentId,
	});

	const handleCommit = useCallback((repoRoot: string) => {
		const commitPrompt = `Commit and push the working changes in ${repoRoot}. If there are unstaged files, commit them too.`;
		const current = inputValueRef.current;
		if (current.includes(commitPrompt)) {
			return;
		}
		const prefix = current.trim() ? "\n\n" : "";
		chatInputRef.current?.insertText(prefix + commitPrompt);
		chatInputRef.current?.focus();
	}, []);

	// Extract PR number from diff status URL.
	const prMatch = diffStatusQuery.data?.url?.match(/\/pull\/(\d+)/)?.[1];
	const prNumber = prMatch ? Number(prMatch) : undefined;
	// Compute an effective selected model by validating the user's
	// explicit choice against the current model options, falling
	// back to the chat's last model or the first available option.
	const effectiveSelectedModel = useMemo(() => {
		if (
			selectedModel &&
			modelOptions.some((model) => model.id === selectedModel)
		) {
			return selectedModel;
		}
		if (chatLastModelConfigID) {
			const fromChat = modelIDByConfigID.get(chatLastModelConfigID);
			if (fromChat && modelOptions.some((model) => model.id === fromChat)) {
				return fromChat;
			}
		}
		return modelOptions[0]?.id ?? "";
	}, [selectedModel, chatLastModelConfigID, modelIDByConfigID, modelOptions]);

	const compressionThreshold = useMemo(() => {
		if (!chatLastModelConfigID) {
			return undefined;
		}
		const config = chatModelConfigsQuery.data?.find(
			(c) => c.id === chatLastModelConfigID,
		);
		return config?.compression_threshold;
	}, [chatLastModelConfigID, chatModelConfigsQuery.data]);
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(
		chatModelsQuery.data,
	);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		chatModelsQuery.isLoading,
		hasConfiguredModels,
	);
	const modelCatalogStatusMessage = getModelCatalogStatusMessage(
		chatModelsQuery.data,
		modelOptions,
		chatModelsQuery.isLoading,
		Boolean(chatModelsQuery.error),
	);
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";
	const isSubmissionPending =
		sendMutation.isPending ||
		editMutation.isPending ||
		interruptMutation.isPending;
	const isInputDisabled = !hasModelOptions || isArchived;

	const handleSend = async (
		message: string,
		fileIds?: string[],
		editedMessageID?: number,
	) => {
		const chatInputHandle = (
			editing.chatInputRef as React.RefObject<ChatMessageInputRef | null>
		)?.current;

		// Walk the Lexical tree in document order so file-reference
		// parts appear at the correct position relative to the
		// surrounding text the user typed.
		const editorParts = chatInputHandle?.getContentParts() ?? [];
		const hasFileReferences = editorParts.some(
			(p) => p.type === "file-reference",
		);
		const hasContent =
			message.trim() || (fileIds && fileIds.length > 0) || hasFileReferences;
		if (!hasContent || isSubmissionPending || !agentId || !hasModelOptions) {
			return;
		}

		const content: TypesGen.ChatInputPart[] = [];

		// Emit parts in document order — text segments and
		// file-reference chips are interleaved as they appear in
		// the editor.
		for (const part of editorParts) {
			if (part.type === "text") {
				const trimmed = part.text.trim();
				if (trimmed) {
					content.push({ type: "text", text: part.text });
				}
			} else {
				const r = part.reference;
				content.push({
					type: "file-reference",
					file_name: r.fileName,
					start_line: r.startLine,
					end_line: r.endLine,
					content: r.content,
				});
			}
		}

		// Add pre-uploaded file references.
		if (fileIds && fileIds.length > 0) {
			for (const fileId of fileIds) {
				content.push({ type: "file", file_id: fileId });
			}
		}
		if (editedMessageID !== undefined) {
			const request: TypesGen.EditChatMessageRequest = { content };
			clearChatErrorReason(agentId);
			clearStreamError();
			setPendingEditMessageId(editedMessageID);
			if (scrollContainerRef.current) {
				scrollContainerRef.current.scrollTop = 0;
			}
			store.clearStreamState();
			try {
				await editMutation.mutateAsync({
					messageId: editedMessageID,
					req: request,
				});
			} finally {
				setPendingEditMessageId(null);
			}
			return;
		}
		const selectedModelConfigID =
			(effectiveSelectedModel &&
				modelConfigIDByModelID.get(effectiveSelectedModel)) ||
			undefined;
		const request: TypesGen.CreateChatMessageRequest = {
			content,
			model_config_id: selectedModelConfigID,
		};
		clearChatErrorReason(agentId);
		clearStreamError();
		if (scrollContainerRef.current) {
			scrollContainerRef.current.scrollTop = 0;
		}

		// No optimistic rendering — the message will appear in the
		// timeline when the server confirms via the POST response or
		// via the SSE stream.
		store.clearStreamState();
		const response = await sendMutation.mutateAsync(request);
		// When the server accepts the message immediately (not
		// queued), insert it into the store so it appears in the
		// timeline without waiting for the SSE stream.
		if (!response.queued && response.message) {
			store.upsertDurableMessage(response.message);
		}
		if (typeof window !== "undefined") {
			if (selectedModelConfigID) {
				localStorage.setItem(
					lastModelConfigIDStorageKey,
					selectedModelConfigID,
				);
			} else {
				localStorage.removeItem(lastModelConfigIDStorageKey);
			}
		}
	};

	const handleInterrupt = () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		void interruptMutation.mutateAsync();
	};

	const handleDeleteQueuedMessage = useCallback(
		async (id: number) => {
			const previousQueuedMessages = store.getSnapshot().queuedMessages;
			store.setQueuedMessages(
				previousQueuedMessages.filter((message) => message.id !== id),
			);
			try {
				await deleteQueuedMutation.mutateAsync(id);
			} catch (error) {
				store.setQueuedMessages(previousQueuedMessages);
				throw error;
			}
		},
		[deleteQueuedMutation, store],
	);

	const handlePromoteQueuedMessage = useCallback(
		async (id: number) => {
			const previousSnapshot = store.getSnapshot();
			const previousQueuedMessages = previousSnapshot.queuedMessages;
			const previousChatStatus = previousSnapshot.chatStatus;
			store.setQueuedMessages(
				previousQueuedMessages.filter((message) => message.id !== id),
			);
			store.clearStreamState();
			store.clearStreamError();
			store.setChatStatus("pending");
			try {
				await promoteQueuedMutation.mutateAsync(id);
			} catch (error) {
				store.setQueuedMessages(previousQueuedMessages);
				store.setChatStatus(previousChatStatus);
				throw error;
			}
		},
		[promoteQueuedMutation, store],
	);

	const editing = useConversationEditingState({
		chatID: agentId,
		onSend: handleSend,
		onDeleteQueuedMessage: handleDeleteQueuedMessage,
		chatInputRef,
		inputValueRef,
	});

	const chatTitle = chatQuery.data?.chat?.title;

	const titleElement = (
		<title>
			{chatTitle ? pageTitle(chatTitle, "Agents") : pageTitle("Agents")}
		</title>
	);

	const parentChatID = getParentChatID(chatQuery.data?.chat);
	const parentChat = parentChatID
		? chatsQuery.data?.find((chat) => chat.id === parentChatID)
		: undefined;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const terminalHref =
		workspace && workspaceAgent
			? getTerminalHref({
					username: workspace.owner_name,
					workspace: workspace.name,
					agent: workspaceAgent.name,
				})
			: null;
	const sshCommand =
		workspace && workspaceAgent && sshConfigQuery.data?.hostname_suffix
			? `ssh ${workspaceAgent.name}.${workspace.name}.${workspace.owner_name}.${sshConfigQuery.data.hostname_suffix}`
			: undefined;

	const generateKeyMutation = useMutation({
		mutationFn: () => API.getApiKey(),
	});

	const handleOpenInEditor = (editor: "cursor" | "vscode") => {
		if (!workspace || !workspaceAgent) {
			return;
		}

		generateKeyMutation.mutate(undefined, {
			onSuccess: ({ key }) => {
				location.href = getVSCodeHref(editor, {
					owner: workspace.owner_name,
					workspace: workspace.name,
					token: key,
					agent: workspaceAgent.name,
					folder: workspaceAgent.expanded_directory,
				});
			},
			onError: () => {
				toast.error(
					editor === "cursor"
						? "Failed to open in Cursor."
						: "Failed to open in VS Code.",
				);
			},
		});
	};

	const handleViewWorkspace = () => {
		if (!workspaceRoute) {
			return;
		}
		window.open(workspaceRoute, "_blank");
	};

	const handleOpenTerminal = () => {
		if (!terminalHref) {
			return;
		}
		openAppInNewWindow(terminalHref);
	};

	const handleArchiveAgentAction = () => {
		if (!agentId || isArchived) {
			return;
		}
		requestArchiveAgent(agentId);
	};

	const handleArchiveAndDeleteWorkspaceAction = () => {
		if (!agentId || isArchived || !workspaceId) {
			return;
		}
		requestArchiveAndDeleteWorkspace(agentId, workspaceId);
	};

	const handleUnarchiveAgentAction = () => {
		if (!agentId || !isArchived) {
			return;
		}
		requestUnarchiveAgent(agentId);
	};

	if (chatQuery.isLoading) {
		return (
			<AgentDetailLoadingView
				titleElement={titleElement}
				isInputDisabled={isInputDisabled}
				effectiveSelectedModel={effectiveSelectedModel}
				setSelectedModel={setSelectedModel}
				modelOptions={modelOptions}
				modelSelectorPlaceholder={modelSelectorPlaceholder}
				hasModelOptions={hasModelOptions}
				inputStatusText={inputStatusText}
				modelCatalogStatusMessage={modelCatalogStatusMessage}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				showRightPanel={showSidebarPanel}
			/>
		);
	}

	if (!chatQuery.data || !agentId) {
		return (
			<AgentDetailNotFoundView
				titleElement={titleElement}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			/>
		);
	}
	return (
		<AgentDetailView
			agentId={agentId}
			chatTitle={chatTitle}
			parentChat={parentChat}
			chatErrorReasons={chatErrorReasons}
			chatRecord={chatRecord}
			isArchived={isArchived}
			hasWorkspace={Boolean(workspaceId)}
			store={store}
			editing={editing}
			pendingEditMessageId={pendingEditMessageId}
			effectiveSelectedModel={effectiveSelectedModel}
			setSelectedModel={setSelectedModel}
			modelOptions={modelOptions}
			modelSelectorPlaceholder={modelSelectorPlaceholder}
			hasModelOptions={hasModelOptions}
			inputStatusText={inputStatusText}
			modelCatalogStatusMessage={modelCatalogStatusMessage}
			compressionThreshold={compressionThreshold}
			isInputDisabled={isInputDisabled}
			isSubmissionPending={isSubmissionPending}
			isInterruptPending={interruptMutation.isPending}
			isSidebarCollapsed={isSidebarCollapsed}
			onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			showSidebarPanel={showSidebarPanel}
			onSetShowSidebarPanel={handleSetShowSidebarPanel}
			prNumber={prNumber}
			diffStatusData={diffStatusQuery.data}
			gitWatcher={gitWatcher}
			canOpenEditors={canOpenEditors}
			canOpenWorkspace={canOpenWorkspace}
			sshCommand={sshCommand}
			handleOpenInEditor={handleOpenInEditor}
			handleViewWorkspace={handleViewWorkspace}
			handleOpenTerminal={handleOpenTerminal}
			handleCommit={handleCommit}
			onNavigateToChat={(chatId) => navigate(`/agents/${chatId}`)}
			handleInterrupt={handleInterrupt}
			handleDeleteQueuedMessage={handleDeleteQueuedMessage}
			handlePromoteQueuedMessage={handlePromoteQueuedMessage}
			handleArchiveAgentAction={handleArchiveAgentAction}
			handleUnarchiveAgentAction={handleUnarchiveAgentAction}
			handleArchiveAndDeleteWorkspaceAction={
				handleArchiveAndDeleteWorkspaceAction
			}
			urlTransform={urlTransform}
			scrollContainerRef={scrollContainerRef}
		/>
	);
};

export default AgentDetail;
