import type * as TypesGen from "api/typesGenerated";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, Profiler, useEffect } from "react";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import type { ModelSelectorOption } from "#/components/ai-elements";
import { useFileAttachments } from "../hooks/useFileAttachments";
import type { ChatDetailError } from "../utils/usageLimitMessage";
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
	type useChatStore,
} from "./AgentDetail/ChatContext";
import { ConversationTimeline } from "./AgentDetail/ConversationTimeline";
import { getLatestContextUsage } from "./AgentDetail/chatHelpers";
import {
	buildSubagentTitles,
	parseMessagesWithMergedTools,
} from "./AgentDetail/messageParsing";
import { buildStreamTools } from "./AgentDetail/streamState";
import type { ParsedMessageEntry } from "./AgentDetail/types";
import { useOnRenderProfiler } from "./AgentDetail/useOnRenderProfiler";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

interface AgentDetailTimelineProps {
	store: ChatStoreHandle;
	persistedErrorReason: ChatDetailError | undefined;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

// Reads only message-related store state (stable during streaming).
// Computes parsedMessages once and passes them to ConversationTimeline
// via a memo boundary so that streaming ticks don't re-parse history.
const MessageListProvider: FC<AgentDetailTimelineProps> = ({
	store,
	persistedErrorReason,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
	urlTransform,
	mcpServers,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const streamError = useChatSelector(store, selectStreamError);
	const subagentStatusOverrides = useChatSelector(
		store,
		selectSubagentStatusOverrides,
	);
	const retryState = useChatSelector(store, selectRetryState);

	const messages = orderedMessageIDs
		.map((messageID) => messagesByID.get(messageID))
		.filter(isChatMessage);
	const parsedMessages = parseMessagesWithMergedTools(messages);
	const subagentTitles = buildSubagentTitles(parsedMessages);
	const detailError: ChatDetailError | undefined =
		(persistedErrorReason?.kind === "usage-limit" || chatStatus === "error"
			? persistedErrorReason
			: undefined) ??
		(streamError
			? { kind: "generic" as const, message: streamError }
			: undefined);
	const latestMessage = messages[messages.length - 1];
	const latestMessageNeedsAssistantResponse =
		!latestMessage || latestMessage.role !== "assistant";

	return (
		<StreamingBridge
			store={store}
			isEmpty={messages.length === 0}
			parsedMessages={parsedMessages}
			subagentTitles={subagentTitles}
			subagentStatusOverrides={subagentStatusOverrides}
			retryState={retryState}
			detailError={detailError}
			latestMessageNeedsAssistantResponse={latestMessageNeedsAssistantResponse}
			chatStatus={chatStatus}
			onEditUserMessage={onEditUserMessage}
			editingMessageId={editingMessageId}
			savingMessageId={savingMessageId}
			urlTransform={urlTransform}
			mcpServers={mcpServers}
		/>
	);
};

// Reads stream-specific store state (changes every token). Isolated
// so that streamState changes don't invalidate parsedMessages above.
const StreamingBridge: FC<{
	store: ChatStoreHandle;
	isEmpty: boolean;
	parsedMessages: ParsedMessageEntry[];
	subagentTitles: Map<string, string>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	retryState: { attempt: number; error: string } | null;
	detailError: ChatDetailError | undefined;
	latestMessageNeedsAssistantResponse: boolean;
	chatStatus: TypesGen.ChatStatus | null;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}> = ({
	store,
	isEmpty,
	parsedMessages,
	subagentTitles,
	subagentStatusOverrides,
	retryState,
	detailError,
	latestMessageNeedsAssistantResponse,
	chatStatus,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
	urlTransform,
	mcpServers,
}) => {
	const streamState = useChatSelector(store, selectStreamState);
	const streamTools = buildStreamTools(streamState);
	const onRenderProfiler = useOnRenderProfiler();
	const isAwaitingFirstStreamChunk =
		!streamState &&
		(chatStatus === "running" || chatStatus === "pending") &&
		latestMessageNeedsAssistantResponse;
	const hasStreamOutput = Boolean(streamState) || isAwaitingFirstStreamChunk;

	return (
		<Profiler id="AgentChat" onRender={onRenderProfiler}>
			<ConversationTimeline
				isEmpty={isEmpty}
				parsedMessages={parsedMessages}
				hasStreamOutput={hasStreamOutput}
				streamState={streamState}
				streamTools={streamTools}
				subagentTitles={subagentTitles}
				subagentStatusOverrides={subagentStatusOverrides}
				retryState={retryState}
				isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
				detailError={detailError}
				onEditUserMessage={onEditUserMessage}
				editingMessageId={editingMessageId}
				savingMessageId={savingMessageId}
				urlTransform={urlTransform}
				mcpServers={mcpServers}
			/>
		</Profiler>
	);
};

export const AgentDetailTimeline: FC<AgentDetailTimelineProps> = (props) => {
	return <MessageListProvider {...props} />;
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
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
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
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const hasStreamState = useChatSelector(store, selectHasStreamState);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const queuedMessages = useChatSelector(store, selectQueuedMessages);

	const messages = orderedMessageIDs
		.map((messageID) => messagesByID.get(messageID))
		.filter(isChatMessage);
	const { organizations } = useDashboard();
	const organizationId = organizations[0]?.id;
	const latestContextUsage = (() => {
		const usage = getLatestContextUsage(messages);
		if (!usage) {
			return usage;
		}
		return { ...usage, compressionThreshold };
	})();
	const {
		attachments,
		textContents,
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
		setAttachments(files);
		setPreviewUrls(
			new Map(
				files.map((f, i) => [
					f,
					`/api/experimental/chats/files/${fileBlocks[i].file_id}`,
				]),
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
		setUploadStates(newUploadStates);
	}, [editingFileBlocks, setAttachments, setPreviewUrls, setUploadStates]);

	const isStreaming =
		hasStreamState || chatStatus === "running" || chatStatus === "pending";

	return (
		<AgentChatInput
			onSend={(message) => {
				void (async () => {
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
					const fileArg = fileIds.length > 0 ? fileIds : undefined;
					try {
						await onSend(message, fileArg);
					} catch {
						// Attachments preserved for retry on failure.
						return;
					}
					resetAttachments();
				})();
			}}
			attachments={attachments}
			onAttach={handleAttach}
			onRemoveAttachment={handleRemoveAttachment}
			uploadStates={uploadStates}
			previewUrls={previewUrls}
			textContents={textContents}
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
			mcpServers={mcpServers}
			selectedMCPServerIds={selectedMCPServerIds}
			onMCPSelectionChange={onMCPSelectionChange}
			onMCPAuthComplete={onMCPAuthComplete}
		/>
	);
};
