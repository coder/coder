import { type FC, Profiler, type ReactNode, useEffect } from "react";
import { toast } from "sonner";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFileAttachments } from "../hooks/useFileAttachments";
import type { ChatDetailError } from "../utils/usageLimitMessage";
import {
	AgentChatInput,
	type AttachedWorkspaceInfo,
	type ChatMessageInputRef,
	type UploadState,
} from "./AgentChatInput";
import { ConversationTimeline } from "./ChatConversation/ConversationTimeline";
import { getLatestContextUsage } from "./ChatConversation/chatHelpers";
import {
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
	modelSelectorHelp?: ReactNode;
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
	attachedWorkspace?: AttachedWorkspaceInfo;
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
	modelSelectorHelp,
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
	attachedWorkspace,
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

	const inputElement = (
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
			attachedWorkspace={attachedWorkspace}
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
