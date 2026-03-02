import { API } from "api/api";
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
import { workspaceById } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { Skeleton } from "components/Skeleton/Skeleton";
import { getVSCodeHref, SESSION_TOKEN_PLACEHOLDER } from "modules/apps/apps";
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
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";
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
import { AgentDetailTopBar } from "./AgentDetail/TopBar";
import { useMessageWindow } from "./AgentDetail/useMessageWindow";
import type { AgentsOutletContext } from "./AgentsPage";
import { DiffRightPanel } from "./DiffRightPanel";
import { FilesChangedPanel } from "./FilesChangedPanel";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";

const noopSetChatErrorReason: AgentsOutletContext["setChatErrorReason"] =
	() => {};
const noopClearChatErrorReason: AgentsOutletContext["clearChatErrorReason"] =
	() => {};
const noopRequestArchiveAgent: AgentsOutletContext["requestArchiveAgent"] =
	() => {};
const lastModelConfigIDStorageKey = "agents.last-model-config-id";
type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

const toOptimisticMessageParts = (
	inputParts: readonly TypesGen.ChatInputPart[],
): readonly TypesGen.ChatMessagePart[] =>
	inputParts.map((part) => ({
		type: "text",
		...(part.text !== undefined ? { text: part.text } : {}),
	}));

const getOrderedMessagesFromStore = (
	store: ChatStoreHandle,
): readonly TypesGen.ChatMessage[] => {
	const snapshot = store.getSnapshot();
	return snapshot.orderedMessageIDs
		.map((messageID) => snapshot.messagesByID.get(messageID))
		.filter(isChatMessage);
};

interface AgentDetailTimelineProps {
	store: ChatStoreHandle;
	chatID: string;
	persistedErrorReason: string | undefined;
	onEditUserMessage?: (messageId: number, text: string) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
}

const AgentDetailTimeline: FC<AgentDetailTimelineProps> = ({
	store,
	chatID,
	persistedErrorReason,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
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
		/>
	);
};

interface AgentDetailInputProps {
	store: ChatStoreHandle;
	compressionThreshold: number | undefined;
	onSend: (message: string) => void;
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
}

const AgentDetailInput: FC<AgentDetailInputProps> = ({
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
	const latestContextUsage = useMemo(() => {
		const usage = getLatestContextUsage(messages);
		if (!usage) {
			return usage;
		}
		return { ...usage, compressionThreshold };
	}, [messages, compressionThreshold]);
	const isStreaming =
		hasStreamState || chatStatus === "running" || chatStatus === "pending";

	return (
		<AgentChatInput
			onSend={onSend}
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
			sticky
		/>
	);
};

interface AgentDetailConversationProps {
	store: ChatStoreHandle;
	chatID: string;
	persistedErrorReason: string | undefined;
	compressionThreshold: number | undefined;
	onDeleteQueuedMessage: (id: number) => Promise<void>;
	onPromoteQueuedMessage: (id: number) => Promise<void>;
	onSend: (message: string, editedMessageID?: number) => Promise<void>;
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
	savingMessageId?: number | null;
}

const AgentDetailConversation: FC<AgentDetailConversationProps> = ({
	store,
	chatID,
	persistedErrorReason,
	compressionThreshold,
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	onSend,
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
	savingMessageId,
}) => {
	const inputValueRef = useRef("");
	const chatInputRef = useRef<ChatMessageInputRef>(null);
	const [editorInitialValue, setEditorInitialValue] = useState("");

	// -- History editing state --
	const [editingMessageId, setEditingMessageId] = useState<number | null>(null);
	const [draftBeforeHistoryEdit, setDraftBeforeHistoryEdit] = useState<
		string | null
	>(null);

	const handleEditUserMessage = useCallback(
		(messageId: number, text: string) => {
			setDraftBeforeHistoryEdit((prev) =>
				editingMessageId !== null ? prev : inputValueRef.current,
			);
			setEditingMessageId(messageId);
			setEditorInitialValue(text);
			inputValueRef.current = text;
		},
		[editingMessageId],
	);

	const handleCancelHistoryEdit = useCallback(() => {
		setEditorInitialValue(draftBeforeHistoryEdit ?? "");
		inputValueRef.current = draftBeforeHistoryEdit ?? "";
		setEditingMessageId(null);
		setDraftBeforeHistoryEdit(null);
	}, [draftBeforeHistoryEdit]);

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
		[editingQueuedMessageID],
	);

	const handleCancelQueueEdit = useCallback(() => {
		setEditorInitialValue(draftBeforeQueueEdit ?? "");
		inputValueRef.current = draftBeforeQueueEdit ?? "";
		setEditingQueuedMessageID(null);
		setDraftBeforeQueueEdit(null);
	}, [draftBeforeQueueEdit]);

	// Wraps the parent onSend to clear local input/editing state
	// and handle queue-edit deletion.
	const handleSendFromInput = useCallback(
		(message: string) => {
			const editedMessageID =
				editingMessageId !== null ? editingMessageId : undefined;
			const queueEditID = editingQueuedMessageID;

			// Clear input and editing state optimistically.
			setEditorInitialValue("");
			inputValueRef.current = "";
			if (editingMessageId !== null) {
				setEditingMessageId(null);
				setDraftBeforeHistoryEdit(null);
			}
			if (queueEditID !== null) {
				setEditingQueuedMessageID(null);
				setDraftBeforeQueueEdit(null);
			}

			void onSend(message, editedMessageID)
				.then(() => {
					if (queueEditID !== null) {
						void onDeleteQueuedMessage(queueEditID);
					}
				})
				.catch(() => {
					// Restore input so the user can retry.
					setEditorInitialValue(message);
					inputValueRef.current = message;
				});
		},
		[editingMessageId, editingQueuedMessageID, onDeleteQueuedMessage, onSend],
	);

	return (
		<>
			<AgentDetailTimeline
				store={store}
				chatID={chatID}
				persistedErrorReason={persistedErrorReason}
				onEditUserMessage={handleEditUserMessage}
				editingMessageId={editingMessageId}
				savingMessageId={savingMessageId}
			/>
			<AgentDetailInput
				store={store}
				compressionThreshold={compressionThreshold}
				onSend={handleSendFromInput}
				onDeleteQueuedMessage={onDeleteQueuedMessage}
				onPromoteQueuedMessage={onPromoteQueuedMessage}
				onInterrupt={onInterrupt}
				isInputDisabled={isInputDisabled}
				isSendPending={isSendPending}
				isInterruptPending={isInterruptPending}
				hasModelOptions={hasModelOptions}
				selectedModel={selectedModel}
				onModelChange={onModelChange}
				modelOptions={modelOptions}
				modelSelectorPlaceholder={modelSelectorPlaceholder}
				inputStatusText={inputStatusText}
				modelCatalogStatusMessage={modelCatalogStatusMessage}
				inputRef={chatInputRef}
				initialValue={editorInitialValue}
				onContentChange={(content) => {
					inputValueRef.current = content;
				}}
				editingQueuedMessageID={editingQueuedMessageID}
				onStartQueueEdit={handleStartQueueEdit}
				onCancelQueueEdit={handleCancelQueueEdit}
				isEditingHistoryMessage={editingMessageId !== null}
				onCancelHistoryEdit={handleCancelHistoryEdit}
			/>
		</>
	);
};

const AgentDetail: FC = () => {
	const navigate = useNavigate();
	const { agentId } = useParams<{ agentId: string }>();
	const outletContext = useOutletContext<AgentsOutletContext | undefined>();
	const queryClient = useQueryClient();
	const [selectedModel, setSelectedModel] = useState("");
	const [showDiffPanel, setShowDiffPanel] = useState(false);
	const [pendingEditMessageId, setPendingEditMessageId] = useState<
		number | null
	>(null);
	const chatErrorReasons = outletContext?.chatErrorReasons ?? {};
	const setChatErrorReason =
		outletContext?.setChatErrorReason ?? noopSetChatErrorReason;
	const clearChatErrorReason =
		outletContext?.clearChatErrorReason ?? noopClearChatErrorReason;
	const requestArchiveAgent =
		outletContext?.requestArchiveAgent ?? noopRequestArchiveAgent;
	const isSidebarCollapsed = outletContext?.isSidebarCollapsed ?? false;
	const onToggleSidebarCollapsed =
		outletContext?.onToggleSidebarCollapsed ?? (() => {});
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

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
	const diffStatusQuery = useQuery({
		...chatDiffStatus(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const hasDiffStatus = Boolean(diffStatusQuery.data?.url);
	const workspace = workspaceQuery.data;
	const workspaceAgent = getWorkspaceAgent(workspace, undefined);
	const chatData = chatQuery.data;
	const chatRecord = chatData?.chat;
	const chatMessages = chatData?.messages;
	const chatQueuedMessages = chatData?.queued_messages;
	const chatLastModelConfigID = chatRecord?.last_model_config_id;

	// Auto-open the diff panel when diff status first appears.
	// See: https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
	const [prevHasDiffStatus, setPrevHasDiffStatus] = useState(false);
	if (hasDiffStatus !== prevHasDiffStatus) {
		setPrevHasDiffStatus(hasDiffStatus);
		if (hasDiffStatus) {
			setShowDiffPanel(true);
		}
	}

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

	useEffect(() => {
		setSelectedModel((current) => {
			if (current && modelOptions.some((model) => model.id === current)) {
				return current;
			}
			if (chatLastModelConfigID) {
				const fromChat = modelIDByConfigID.get(chatLastModelConfigID);
				if (fromChat && modelOptions.some((model) => model.id === fromChat)) {
					return fromChat;
				}
			}
			return modelOptions[0]?.id ?? "";
		});
	}, [chatLastModelConfigID, modelIDByConfigID, modelOptions]);

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
	const isInputDisabled = !hasModelOptions;

	const handleSend = async (message: string, editedMessageID?: number) => {
		if (
			!message.trim() ||
			isSubmissionPending ||
			!agentId ||
			!hasModelOptions
		) {
			return;
		}
		const content: TypesGen.ChatInputPart[] = [{ type: "text", text: message }];
		if (editedMessageID !== undefined) {
			const request: TypesGen.EditChatMessageRequest = { content };
			clearChatErrorReason(agentId);
			clearStreamError();
			setPendingEditMessageId(editedMessageID);
			if (scrollContainerRef.current) {
				scrollContainerRef.current.scrollTop = 0;
			}
			const previousChatStatus = store.getSnapshot().chatStatus;
			const previousMessages = getOrderedMessagesFromStore(store);
			const messageIndex = previousMessages.findIndex(
				(msg) => msg.id === editedMessageID,
			);
			if (messageIndex !== -1) {
				const optimisticEditedMessage: TypesGen.ChatMessage = {
					...previousMessages[messageIndex],
					content: toOptimisticMessageParts(request.content),
				};
				store.replaceMessages([
					...previousMessages.slice(0, messageIndex),
					optimisticEditedMessage,
				]);
			}
			store.clearStreamState();
			store.setChatStatus("pending");
			try {
				await editMutation.mutateAsync({
					messageId: editedMessageID,
					req: request,
				});
			} catch (error) {
				store.replaceMessages(previousMessages);
				store.setChatStatus(previousChatStatus);
				throw error;
			} finally {
				setPendingEditMessageId(null);
			}
			return;
		}
		const selectedModelConfigID =
			(selectedModel && modelConfigIDByModelID.get(selectedModel)) || undefined;
		const request: TypesGen.CreateChatMessageRequest = {
			content,
			model_config_id: selectedModelConfigID,
		};
		clearChatErrorReason(agentId);
		clearStreamError();
		if (scrollContainerRef.current) {
			scrollContainerRef.current.scrollTop = 0;
		}

		// Inject an optimistic user message so the bubble appears in
		// the timeline immediately, without waiting for the server.
		const previousMessages = getOrderedMessagesFromStore(store);
		const previousChatStatus = store.getSnapshot().chatStatus;
		const optimisticMessage: TypesGen.ChatMessage = {
			id: -Date.now(),
			chat_id: agentId,
			created_at: new Date().toISOString(),
			role: "user",
			content: toOptimisticMessageParts(content),
		};
		store.upsertDurableMessage(optimisticMessage);
		store.clearStreamState();
		store.setChatStatus("pending");

		try {
			await sendMutation.mutateAsync(request);
		} catch (error) {
			// Roll back the optimistic message so the timeline
			// returns to its previous state.
			store.replaceMessages(previousMessages);
			store.setChatStatus(previousChatStatus);
			throw error;
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

	const chatTitle = chatQuery.data?.chat?.title;

	// Update the browser tab title when navigating to / between agents.
	useEffect(() => {
		document.title = chatTitle
			? pageTitle(chatTitle, "Agents")
			: pageTitle("Agents");
		return () => {
			document.title = pageTitle("Agents");
		};
	}, [chatTitle]);

	const parentChatID = getParentChatID(chatQuery.data?.chat);
	const parentChat = parentChatID
		? chatsQuery.data?.find((chat) => chat.id === parentChatID)
		: undefined;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const shouldShowDiffPanel = hasDiffStatus && showDiffPanel;

	const handleOpenInEditor = async (editor: "cursor" | "vscode") => {
		if (!workspace || !workspaceAgent) {
			return;
		}

		try {
			const { key } = await API.getApiKey();
			const vscodeHref = getVSCodeHref("vscode", {
				owner: workspace.owner_name,
				workspace: workspace.name,
				token: key,
				agent: workspaceAgent.name,
				folder: workspaceAgent.expanded_directory,
			});

			if (editor === "cursor") {
				const cursorApp = workspaceAgent.apps.find((app) => {
					const name = (app.display_name ?? app.slug).toLowerCase();
					return app.slug.toLowerCase() === "cursor" || name === "cursor";
				});
				if (cursorApp?.external && cursorApp.url) {
					const href = cursorApp.url.includes(SESSION_TOKEN_PLACEHOLDER)
						? cursorApp.url.replaceAll(SESSION_TOKEN_PLACEHOLDER, key)
						: cursorApp.url;
					window.location.assign(href);
					return;
				}
				window.location.assign(vscodeHref.replace(/^vscode:/, "cursor:"));
				return;
			}

			window.location.assign(vscodeHref);
		} catch {
			toast.error(
				editor === "cursor"
					? "Failed to open in Cursor."
					: "Failed to open in VS Code.",
			);
		}
	};

	const handleViewWorkspace = () => {
		if (!workspaceRoute) {
			return;
		}
		navigate(workspaceRoute);
	};

	const handleArchiveAgentAction = () => {
		if (!agentId) {
			return;
		}
		requestArchiveAgent(agentId);
	};

	if (chatQuery.isLoading) {
		return (
			<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
				<AgentDetailTopBar
					diff={{
						hasDiffStatus: false,
						diffStatus: undefined,
						showDiffPanel: false,
						onToggleFilesChanged: () => {},
					}}
					workspace={{
						canOpenEditors: false,
						canOpenWorkspace: false,
						onOpenInEditor: () => {},
						onViewWorkspace: () => {},
					}}
					onOpenParentChat={() => {}}
					onArchiveAgent={() => {}}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				/>
				<div className="flex h-full flex-col-reverse overflow-hidden">
					<div className="px-4">
						<div className="mx-auto w-full max-w-3xl py-6">
							<div className="flex flex-col gap-3">
								{/* User message bubble (right-aligned) */}
								<div className="flex w-full justify-end">
									<Skeleton className="h-10 w-2/3 rounded-lg" />
								</div>
								{/* Assistant response lines (left-aligned) */}
								<div className="space-y-3">
									<Skeleton className="h-4 w-full" />
									<Skeleton className="h-4 w-5/6" />
									<Skeleton className="h-4 w-4/6" />
								</div>
								{/* Second user message bubble */}
								<div className="mt-3 flex w-full justify-end">
									<Skeleton className="h-10 w-1/2 rounded-lg" />
								</div>
								{/* Second assistant response */}
								<div className="space-y-3">
									<Skeleton className="h-4 w-full" />
									<Skeleton className="h-4 w-5/6" />
									<Skeleton className="h-4 w-4/6" />
									<Skeleton className="h-4 w-full" />
									<Skeleton className="h-4 w-3/5" />
								</div>
							</div>
						</div>
						<AgentChatInput
							onSend={() => {}}
							initialValue=""
							isDisabled={isInputDisabled}
							isLoading={false}
							selectedModel={selectedModel}
							onModelChange={setSelectedModel}
							modelOptions={modelOptions}
							modelSelectorPlaceholder={modelSelectorPlaceholder}
							hasModelOptions={hasModelOptions}
							inputStatusText={inputStatusText}
							modelCatalogStatusMessage={modelCatalogStatusMessage}
							sticky
						/>
					</div>
				</div>
			</div>
		);
	}

	if (!chatQuery.data || !agentId) {
		return (
			<div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
				<AgentDetailTopBar
					diff={{
						hasDiffStatus: false,
						diffStatus: undefined,
						showDiffPanel: false,
						onToggleFilesChanged: () => {},
					}}
					workspace={{
						canOpenEditors: false,
						canOpenWorkspace: false,
						onOpenInEditor: () => {},
						onViewWorkspace: () => {},
					}}
					onOpenParentChat={() => {}}
					onArchiveAgent={() => {}}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				/>
				<div className="flex flex-1 items-center justify-center text-content-secondary">
					Chat not found
				</div>
			</div>
		);
	}

	return (
		<div
			className={cn(
				"flex min-h-0 min-w-0 flex-1",
				shouldShowDiffPanel && "flex-col xl:flex-row",
			)}
		>
			<div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
				<AgentDetailTopBar
					chatTitle={chatTitle}
					parentChat={parentChat}
					onOpenParentChat={(chatId) => navigate(`/agents/${chatId}`)}
					diff={{
						hasDiffStatus,
						diffStatus: diffStatusQuery.data,
						showDiffPanel,
						onToggleFilesChanged: () => setShowDiffPanel((prev) => !prev),
					}}
					workspace={{
						canOpenEditors,
						canOpenWorkspace,
						onOpenInEditor: (editor) => {
							void handleOpenInEditor(editor);
						},
						onViewWorkspace: handleViewWorkspace,
					}}
					onArchiveAgent={handleArchiveAgentAction}
					isSidebarCollapsed={isSidebarCollapsed}
					onToggleSidebarCollapsed={onToggleSidebarCollapsed}
				/>
				<div
					aria-hidden
					className="pointer-events-none absolute inset-x-0 top-0 z-10 h-6 bg-surface-primary"
					style={{
						maskImage:
							"linear-gradient(to bottom, black 0%, rgba(0,0,0,0.6) 40%, rgba(0,0,0,0.2) 70%, transparent 100%)",
						WebkitMaskImage:
							"linear-gradient(to bottom, black 0%, rgba(0,0,0,0.6) 40%, rgba(0,0,0,0.2) 70%, transparent 100%)",
					}}
				/>
				<div
					ref={scrollContainerRef}
					className="flex h-full flex-col-reverse overflow-y-auto [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
				>
					<div className="px-4">
						<AgentDetailConversation
							store={store}
							chatID={agentId}
							persistedErrorReason={
								chatErrorReasons[agentId] || chatRecord?.last_error || undefined
							}
							compressionThreshold={compressionThreshold}
							onDeleteQueuedMessage={handleDeleteQueuedMessage}
							onPromoteQueuedMessage={handlePromoteQueuedMessage}
							onSend={handleSend}
							onInterrupt={handleInterrupt}
							isInputDisabled={isInputDisabled}
							isSendPending={isSubmissionPending}
							isInterruptPending={interruptMutation.isPending}
							hasModelOptions={hasModelOptions}
							selectedModel={selectedModel}
							onModelChange={setSelectedModel}
							modelOptions={modelOptions}
							modelSelectorPlaceholder={modelSelectorPlaceholder}
							inputStatusText={inputStatusText}
							modelCatalogStatusMessage={modelCatalogStatusMessage}
							savingMessageId={pendingEditMessageId}
						/>
					</div>
				</div>
			</div>
			<DiffRightPanel isOpen={shouldShowDiffPanel}>
				<FilesChangedPanel chatId={agentId} />
			</DiffRightPanel>
		</div>
	);
};

export default AgentDetail;
