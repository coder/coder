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

	/** Supply this callback when workspace files can be accepted; omit it to reject them. */
	onAttach?: (files: File[]) => void;
	onRemove: (id: string) => void;

	/** Message shown when a workspace file is rejected because uploads are unavailable. */
	unavailableMessage?: string;

	/** Set when files upload during submission and failed uploads retry on the next send. */
	deferred?: boolean;
};

/** Draft inputs and callbacks for AgentComposerProvider. */
export type AgentComposerBindings = {
	onSend: (message: string) => Promise<void> | void;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading: boolean;
	inputRef?: React.Ref<ChatMessageInputRef>;
	initialValue: string;
	initialEditorState?: string;
	/** Increment to replace the editor; do not reuse a previous generation. */
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
		attachEditor: React.RefCallback<ChatMessageInputRef>;
		attachFileInput: React.RefCallback<HTMLInputElement>;
		warningId: string;
		composerElement: HTMLDivElement | null;
		setComposerElement: React.Dispatch<
			React.SetStateAction<HTMLDivElement | null>
		>;
		initialValue: string;
		initialEditorState?: string;
		remountKey?: number;
		sendShortcut: ReturnType<typeof getAgentChatSendShortcut>;
		sendShortcutLabel?: string;
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
