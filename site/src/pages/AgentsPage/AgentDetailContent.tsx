import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useEffect, useMemo } from "react";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
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
import type { ChatDetailError } from "./usageLimitMessage";
import { useFileAttachments } from "./useFileAttachments";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

interface AgentDetailTimelineProps {
	store: ChatStoreHandle;
	persistedErrorReason: ChatDetailError | undefined;
	onOpenAnalytics?: () => void;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	urlTransform?: UrlTransform;
}

export const AgentDetailTimeline: FC<AgentDetailTimelineProps> = ({
	store,
	persistedErrorReason,
	onOpenAnalytics,
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
	const parsedMessages = useMemo(
		() => parseMessagesWithMergedTools(messages),
		[messages],
	);
	const subagentTitles = useMemo(
		() => buildSubagentTitles(parsedMessages),
		[parsedMessages],
	);
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
	const isAwaitingFirstStreamChunk =
		!streamState &&
		(chatStatus === "running" || chatStatus === "pending") &&
		latestMessageNeedsAssistantResponse;
	const hasStreamOutput = Boolean(streamState) || isAwaitingFirstStreamChunk;

	return (
		<ConversationTimeline
			isEmpty={messages.length === 0}
			parsedMessages={parsedMessages}
			hasStreamOutput={hasStreamOutput}
			streamState={streamState}
			streamTools={streamTools}
			subagentTitles={subagentTitles}
			subagentStatusOverrides={subagentStatusOverrides}
			retryState={retryState}
			isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
			detailError={detailError}
			onOpenAnalytics={onOpenAnalytics}
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
		const fileBlocks = editingFileBlocks.filter(
			(b): b is TypesGen.ChatFilePart => b.type === "file",
		);
		const files = fileBlocks.map((block, i) => {
			const mt = block.media_type ?? "application/octet-stream";
			const ext = mt.split("/")[1] ?? "png";
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
