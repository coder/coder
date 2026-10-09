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

/** File data and operations supplied by the draft's upload owner. */
export type ComposerFileBindings = {
	attachments: readonly File[];
	onAttach?: (files: File[]) => void;
	onRemoveAttachment: (attachment: number | File) => void;
	uploadStates: Map<File, UploadState>;
	previewUrls: Map<File, string>;
	textContents: Map<File, string>;
	workspaceUploads: WorkspaceUploadsProps;
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
	onSend?: (message: string) => Promise<void> | void;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading?: boolean;
	warning?: string;
};

/** Operations available only inside an existing chat. */
export type ComposerChatBindings = {
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	isEditingHistoryMessage?: boolean;
	onCancelHistoryEdit?: () => void;
	userPromptHistory?: readonly string[];
};

/** Draft inputs for the default runtime; assemblies select their own capabilities. */
export type AgentComposerBindings = ComposerDraftBindings &
	ComposerChatBindings & {
		files?: ComposerFileBindings;
		queuedMessages?: readonly ChatQueuedMessage[];
		onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
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
		invisibleCharCount: number;
		canSend: boolean;
		showSendButton: boolean;
		showStopButton: boolean;
		canAttachFiles: boolean;
		speechSupported: boolean;
		speechRecording: boolean;
		speechError: string | null;
		needsSetup: boolean;
		files?: Pick<
			ComposerFileBindings,
			"attachments" | "uploadStates" | "previewUrls" | "textContents"
		> & {
			workspaceUploads: { uploads: readonly WorkspaceFileUpload[] };
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
		attachFiles: (files: File[]) => boolean;
		removeAttachment: ComposerFileBindings["onRemoveAttachment"];
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
