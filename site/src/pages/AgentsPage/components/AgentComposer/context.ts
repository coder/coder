import type React from "react";
import { createContext, use } from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import type { WorkspaceFileUpload } from "../../hooks/useWorkspaceFileUploads";
import type { getAgentChatSendShortcut } from "../../utils/agentChatSendShortcut";
import type { UploadState } from "../AttachmentPreview";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";

/** Workspace file uploads displayed and routed by the composer. */
type WorkspaceUploadsProps = {
	uploads: readonly WorkspaceFileUpload[];
	// Present only when the chat has a bound workspace with a
	// connected agent; its absence hides the whole affordance.
	onAttach?: (files: File[]) => void;
	onRemove: (id: string) => void;
	// Toast shown when a workspace-routed file arrives while onAttach
	// is unavailable. Overridden on the new-chat page, where the fix
	// is selecting a workspace rather than attaching one to the chat.
	unavailableMessage?: string;
	// Deferred mode (new-chat page): entries upload during submit and
	// every entry re-uploads on the next send after a failure, so
	// error chips still count as sendable content.
	deferred?: boolean;
};

/** External callbacks and draft inputs for the feature-local composer runtime. */
export type AgentComposerBindings = {
	onSend: (message: string) => void;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading: boolean;
	inputRef?: React.Ref<ChatMessageInputRef>;
	initialValue: string;
	initialEditorState?: string;
	remountKey?: number;
	onContentChange: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
	hasModelOptions: boolean;
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	warning?: string;
	isEditingHistoryMessage?: boolean;
	onCancelHistoryEdit?: () => void;
	userPromptHistory?: readonly string[];
	queuedMessages?: readonly ChatQueuedMessage[];
	onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
	attachments?: readonly File[];
	onAttach?: (files: File[]) => void;
	onRemoveAttachment?: (attachment: number | File) => void;
	uploadStates?: Map<File, UploadState>;
	previewUrls?: Map<File, string>;
	textContents?: Map<File, string>;
	workspaceUploads?: WorkspaceUploadsProps;
	onTextPreview?: (
		content: string,
		fileName: string,
		mediaType: string,
	) => void;
};

export type ComposerContextValue = {
	state: {
		isDisabled: boolean;
		isReadOnly: boolean;
		isLoading: boolean;
		isStreaming: boolean;
		isInterruptPending: boolean;
		isEditingHistoryMessage: boolean;
		warning?: string;
		isDragging: boolean;
		invisibleCharCount: number;
		canSend: boolean;
		showSendButton: boolean;
		showStopButton: boolean;
		canAttachFiles: boolean;
		speechSupported: boolean;
		speechRecording: boolean;
		speechError: string | null;
	};
	actions: {
		openFilePicker: () => void;
		resetPromptCycle: () => void;
		submit: () => void;
		startRecording: () => void;
		acceptRecording: () => void;
		cancelRecording: () => void;
		interrupt?: () => void;
		cancelHistoryEdit?: () => void;
		fileSelect: (event: React.ChangeEvent<HTMLInputElement>) => void;
		filePaste: (file: File) => boolean;
		inlineText: (file: File, nextContent?: string) => void;
		textPreview: (content: string, fileName: string, mediaType: string) => void;
		imagePreview: (src: string) => void;
		contentChange: AgentComposerBindings["onContentChange"];
		editorKeyDown: (event: React.KeyboardEvent) => void;
		composerKeyDown: (event: React.KeyboardEvent) => void;
		dragOver: (event: React.DragEvent) => void;
		dragLeave: (event: React.DragEvent) => void;
		drop: (event: React.DragEvent) => void;
	};
	meta: {
		warningId: string;
		composerElement: HTMLDivElement | null;
		setComposerElement: React.Dispatch<
			React.SetStateAction<HTMLDivElement | null>
		>;
		initialValue: string;
		initialEditorState?: string;
		remountKey?: number;
		sendShortcut: ReturnType<typeof getAgentChatSendShortcut>;
		sendButtonLabel: string;
		sendButtonTooltip: string;
		sendButtonKeyShortcuts?: string;
		attachments: readonly File[];
		onRemoveAttachment?: AgentComposerBindings["onRemoveAttachment"];
		uploadStates?: Map<File, UploadState>;
		previewUrls?: Map<File, string>;
		textContents?: Map<File, string>;
		workspaceUploads?: WorkspaceUploadsProps;
	};
};

export const ComposerContext = createContext<ComposerContextValue | null>(null);
export const ComposerRefsContext = createContext<{
	editorRef: React.RefObject<ChatMessageInputRef | null>;
	fileInputRef: React.RefObject<HTMLInputElement | null>;
} | null>(null);

/** Reads composer state and actions; must be called inside AgentComposer.Provider. */
export function useAgentComposer() {
	const context = use(ComposerContext);
	if (!context) {
		throw new Error(
			"useAgentComposer must be used inside AgentComposer.Provider",
		);
	}
	return context;
}
