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
	/**
	 * Present only when the chat has a bound workspace with a connected
	 * agent; its absence hides the whole affordance.
	 */
	onAttach?: (files: File[]) => void;
	onRemove: (id: string) => void;
	/**
	 * Toast shown when a workspace-routed file arrives while onAttach is
	 * unavailable. Overridden on the new-chat page, where the fix is
	 * selecting a workspace rather than attaching one to the chat.
	 */
	unavailableMessage?: string;
	/**
	 * Deferred mode (new-chat page): entries upload during submit and
	 * every entry re-uploads on the next send after a failure, so error
	 * chips still count as sendable content.
	 */
	deferred?: boolean;
};

/** File data and operations supplied by the draft's upload owner. */
export type ComposerFileBindings = {
	attachments: readonly File[];
	onAttach?: (files: File[]) => void;
	onRemoveAttachment?: (attachment: number | File) => void;
	uploadStates?: Map<File, UploadState>;
	previewUrls?: Map<File, string>;
	textContents?: Map<File, string>;
	workspaceUploads?: WorkspaceUploadsProps;
};

/** Initial document and change notifications for the uncontrolled editor. */
export type ComposerEditorBindings = {
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
};

/** Submission capabilities shared by new and existing chat drafts. */
export type ComposerDraftBindings = ComposerEditorBindings & {
	onSend: (message: string) => void;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading?: boolean;
	/** Sending is refused until at least one model is available. */
	hasModelOptions: boolean;
	warning?: string;
};

/** Operations available only inside an existing chat. */
export type ComposerChatBindings = {
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	isEditingHistoryMessage?: boolean;
	onCancelHistoryEdit?: () => void;
	/** Newest-first list of non-empty user prompts for local history cycling. */
	userPromptHistory?: readonly string[];
};

/** Draft inputs for the default runtime. */
export type AgentComposerBindings = ComposerDraftBindings &
	ComposerChatBindings & {
		files: ComposerFileBindings;
		queuedMessages?: readonly ChatQueuedMessage[];
		onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
	};

/** Public composer contract: state to render, actions to call, and element wiring. */
export type ComposerContextValue = {
	state: {
		isDisabled: boolean;
		isReadOnly: boolean;
		isLoading: boolean;
		isStreaming: boolean;
		isInterruptPending: boolean;
		isEditingHistoryMessage: boolean;
		warning?: string;
		invisibleCharCount: number;
		canSend: boolean;
		showSendButton: boolean;
		showStopButton: boolean;
		canAttachFiles: boolean;
		speechSupported: boolean;
		speechRecording: boolean;
		speechError: string | null;
		needsSetup: boolean;
		files: Pick<
			ComposerFileBindings,
			"attachments" | "uploadStates" | "previewUrls" | "textContents"
		> & {
			workspaceUploads?: { uploads: readonly WorkspaceFileUpload[] };
		};
	};
	actions: {
		resetPromptCycle: () => void;
		submit: () => void;
		startRecording: () => void;
		acceptRecording: () => void;
		cancelRecording: () => void;
		interrupt?: () => void;
		cancelHistoryEdit?: () => void;
		/** Returns whether any file was routed to an upload owner. */
		attachFiles: (files: File[]) => boolean;
		/** Absent when the upload owner does not support removing attachments. */
		removeAttachment?: (attachment: number | File) => void;
		removeWorkspaceUpload: (id: string) => void;
		inlineText: (file: File, nextContent?: string) => void;
		contentChange: ComposerEditorBindings["onContentChange"];
		editorKeyDown: (event: React.KeyboardEvent) => void;
	};
	meta: {
		editorRef: React.RefObject<ChatMessageInputRef | null>;
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
