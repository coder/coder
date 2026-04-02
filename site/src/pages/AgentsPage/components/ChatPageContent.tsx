import { type FC, Profiler, useEffect, useRef } from "react";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFileAttachments } from "../hooks/useFileAttachments";
import type { ChatDetailError } from "../utils/usageLimitMessage";
import {
	AgentChatInput,
	type ChatMessageInputRef,
	type UploadState,
} from "./AgentChatInput";
import { ConversationTimeline } from "./ChatConversation/ConversationTimeline";
import { getLatestContextUsage } from "./ChatConversation/chatHelpers";
import {
	isActiveChatStatus,
	selectChatStatus,
	selectHasStreamState,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	useChatSelector,
	type useChatStore,
} from "./ChatConversation/chatStore";
import { LiveStreamTail } from "./ChatConversation/LiveStreamTail";
import {
	buildComputerUseSubagentIds,
	buildSubagentTitles,
	getEditableUserMessagePayload,
	parseMessagesWithMergedTools,
} from "./ChatConversation/messageParsing";
import {
	type PreparedUserSubmission,
	prepareUserSubmission,
} from "./ChatConversation/prepareUserSubmission";
import { useOnRenderProfiler } from "./ChatConversation/useOnRenderProfiler";
import type { ModelSelectorOption } from "./ChatElements";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

const isChatMessage = (
	message: TypesGen.ChatMessage | undefined,
): message is TypesGen.ChatMessage => Boolean(message);

interface ChatPageTimelineProps {
	chatID?: string;
	store: ChatStoreHandle;
	persistedError: ChatDetailError | undefined;
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

export const ChatPageTimeline: FC<ChatPageTimelineProps> = ({
	chatID,
	store,
	persistedError,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
	urlTransform,
	mcpServers,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const hasStreamState = useChatSelector(store, selectHasStreamState);
	const isTurnActive = isActiveChatStatus(chatStatus) || hasStreamState;

	const messages = orderedMessageIDs
		.map((messageID) => messagesByID.get(messageID))
		.filter(isChatMessage);
	const parsedMessages = parseMessagesWithMergedTools(messages);
	const subagentTitles = buildSubagentTitles(parsedMessages);
	const computerUseSubagentIds = buildComputerUseSubagentIds(parsedMessages);
	const onRenderProfiler = useOnRenderProfiler();

	return (
		<Profiler id="AgentChat" onRender={onRenderProfiler}>
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-3 py-6">
				{/* VNC sessions for completed agents may already be
					   terminated, so inline desktop previews are disabled
					   via showDesktopPreviews={false} to avoid a perpetual
					   "disconnected" state. The MonitorIcon variant still
					   renders correctly. */}
				<ConversationTimeline
					parsedMessages={parsedMessages}
					subagentTitles={subagentTitles}
					onEditUserMessage={onEditUserMessage}
					editingMessageId={editingMessageId}
					savingMessageId={savingMessageId}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
					computerUseSubagentIds={computerUseSubagentIds}
					showDesktopPreviews={false}
					isTurnActive={isTurnActive}
				/>
				<LiveStreamTail
					store={store}
					persistedError={persistedError}
					startingResetKey={chatID}
					isTranscriptEmpty={parsedMessages.length === 0}
					subagentTitles={subagentTitles}
					computerUseSubagentIds={computerUseSubagentIds}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
				/>
			</div>
		</Profiler>
	);
};

interface ChatPageInputProps {
	store: ChatStoreHandle;
	compressionThreshold: number | undefined;
	onSend: (submission: PreparedUserSubmission) => Promise<void>;
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
	isModelCatalogLoading?: boolean;
	// Imperative editor handle plus the one-time initial draft,
	// owned by the conversation component.
	inputRef?: React.RefObject<ChatMessageInputRef | null>;
	initialValue?: string;
	initialEditorState?: string;
	remountKey?: number;
	onContentChange?: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
	editingQueuedMessageID: number | null;
	onStartQueueEdit: (
		id: number,
		text: string,
		fileBlocks: readonly TypesGen.ChatMessagePart[],
	) => void;
	onCancelQueueEdit: () => void;
	isEditingHistoryMessage: boolean;
	onCancelHistoryEdit: () => void;
	onEditUserMessage: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	// File parts from the message being edited, converted to
	// File objects and pre-populated into attachments.
	editingFileBlocks?: readonly TypesGen.ChatMessagePart[];
	// MCP server picker state.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	lastInjectedContext?: readonly TypesGen.ChatMessagePart[];
}

export const ChatPageInput: FC<ChatPageInputProps> = ({
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
	isModelCatalogLoading = false,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
	editingQueuedMessageID,
	onStartQueueEdit,
	onCancelQueueEdit,
	isEditingHistoryMessage,
	onCancelHistoryEdit,
	onEditUserMessage,
	editingFileBlocks,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	lastInjectedContext,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const hasStreamState = useChatSelector(store, selectHasStreamState);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const queuedMessages = useChatSelector(store, selectQueuedMessages);

	const messages = orderedMessageIDs
		.map((messageID) => messagesByID.get(messageID))
		.filter(isChatMessage);
	let lastEditableUserMessage: TypesGen.ChatMessage | undefined;
	for (let index = orderedMessageIDs.length - 1; index >= 0; index--) {
		const message = messagesByID.get(orderedMessageIDs[index]);
		if (message?.role === "user") {
			lastEditableUserMessage = message;
			break;
		}
	}

	const handleEditLastUserMessage = lastEditableUserMessage
		? () => {
				const { text, fileBlocks } = getEditableUserMessagePayload(
					lastEditableUserMessage,
				);
				onEditUserMessage(lastEditableUserMessage.id, text, fileBlocks);
			}
		: undefined;

	const rawUsage = getLatestContextUsage(messages);
	const latestContextUsage = rawUsage
		? { ...rawUsage, compressionThreshold, lastInjectedContext }
		: rawUsage;
	const { organizations } = useDashboard();
	const organizationId = organizations[0]?.id;
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
		setTextContents,
		setUploadStates,
	} = useFileAttachments(organizationId);
	type AttachmentSnapshot = {
		attachments: File[];
		previewUrls: Map<File, string>;
		textContents: Map<File, string>;
		uploadStates: Map<File, UploadState>;
	};

	const skipEditingFileBlockSyncRef = useRef(false);

	const snapshotAttachments = (): AttachmentSnapshot => ({
		attachments: [...attachments],
		previewUrls: new Map(previewUrls),
		textContents: new Map(textContents),
		uploadStates: new Map(uploadStates),
	});

	const restoreAttachments = (snapshot: AttachmentSnapshot) => {
		skipEditingFileBlockSyncRef.current = true;
		setAttachments([...snapshot.attachments]);
		// resetAttachments() revokes blob previews on the optimistic path,
		// so recreate them here when restoring the failed edit draft.
		const restoredPreviewUrls = new Map<File, string>();
		for (const [file, url] of snapshot.previewUrls) {
			restoredPreviewUrls.set(
				file,
				url.startsWith("blob:") ? URL.createObjectURL(file) : url,
			);
		}
		setPreviewUrls(restoredPreviewUrls);
		setTextContents(new Map(snapshot.textContents));
		setUploadStates(new Map(snapshot.uploadStates));
	};

	// Pre-populate attachments from existing file blocks when
	// entering edit mode so retries preserve prior attachments.
	useEffect(() => {
		if (skipEditingFileBlockSyncRef.current) {
			skipEditingFileBlockSyncRef.current = false;
			return;
		}
		if (!editingFileBlocks || editingFileBlocks.length === 0) {
			// Clear attachments when exiting edit mode.
			setAttachments([]);
			setUploadStates(new Map());
			setPreviewUrls(new Map());
			setTextContents(new Map());
			return;
		}
		const fileBlocks = editingFileBlocks.filter(
			(b): b is TypesGen.ChatFilePart => b.type === "file",
		);
		const files = fileBlocks.map((block, i) => {
			const mt = block.media_type ?? "application/octet-stream";
			const ext = mt === "text/plain" ? "txt" : (mt.split("/")[1] ?? "png");
			// Empty File used as a Map key only. The synthetic name
			// preserves the original block index so retries stay aligned
			// even if the user removes another attachment first.
			return new File([], `attachment-${i}.${ext}`, { type: mt });
		});
		setAttachments(files);
		const nextPreviewUrls = new Map<File, string>();
		const nextTextContents = new Map<File, string>();
		const newUploadStates = new Map<File, UploadState>();
		for (const [i, file] of files.entries()) {
			const block = fileBlocks[i];
			if (block.file_id) {
				nextPreviewUrls.set(
					file,
					`/api/experimental/chats/files/${block.file_id}`,
				);
				newUploadStates.set(file, {
					status: "uploaded",
					fileId: block.file_id,
				});
				continue;
			}
			if (block.media_type.startsWith("image/") && block.data) {
				nextPreviewUrls.set(
					file,
					`data:${block.media_type};base64,${block.data}`,
				);
			}
			if (block.media_type === "text/plain" && block.data) {
				nextTextContents.set(file, block.data);
			}
		}
		setPreviewUrls(nextPreviewUrls);
		setTextContents(nextTextContents);
		setUploadStates(newUploadStates);
	}, [
		editingFileBlocks,
		setAttachments,
		setPreviewUrls,
		setTextContents,
		setUploadStates,
	]);

	const isStreaming =
		hasStreamState || chatStatus === "running" || chatStatus === "pending";

	return (
		<AgentChatInput
			onSend={(_message) => {
				void (async () => {
					const submission = prepareUserSubmission({
						editorParts: inputRef?.current?.getContentParts() ?? [],
						attachments,
						uploadStates,
						editingFileBlocks,
					});
					if (submission.skippedAttachmentErrors > 0) {
						toast.warning(
							`${submission.skippedAttachmentErrors} attachment${submission.skippedAttachmentErrors > 1 ? "s" : ""} could not be sent (upload failed)`,
						);
					}
					if (submission.requestContent.length === 0) {
						return;
					}
					const attachmentSnapshot = isEditingHistoryMessage
						? snapshotAttachments()
						: null;
					if (isEditingHistoryMessage) {
						resetAttachments();
					}
					try {
						await onSend(submission);
					} catch {
						if (attachmentSnapshot) {
							restoreAttachments(attachmentSnapshot);
						}
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
			onEditLastUserMessage={handleEditLastUserMessage}
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
			isModelCatalogLoading={isModelCatalogLoading}
			mcpServers={mcpServers}
			selectedMCPServerIds={selectedMCPServerIds}
			onMCPSelectionChange={onMCPSelectionChange}
			onMCPAuthComplete={onMCPAuthComplete}
		/>
	);
};
