import { useEffect, useId, useRef, useState } from "react";
import { useQuery } from "react-query";
import { preferenceSettings } from "#/api/queries/users";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { isMobileViewport, mobileViewportMediaQuery } from "#/utils/mobile";
import {
	getAgentChatSendShortcut,
	MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
} from "../../utils/agentChatSendShortcut";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";
import { composerFiles } from "./composerFiles";
import type { AgentComposerBindings, ComposerContextValue } from "./context";
import { useComposerEditor } from "./useComposerEditor";

/** Coordinates submission and exposes the public state/actions/meta contract. */
export function useComposerRuntime(
	bindings: AgentComposerBindings,
	needsSetup: boolean,
) {
	const {
		onSend,
		isDisabled,
		isReadOnly = false,
		isLoading = false,
		hasModelOptions,
		isStreaming = false,
		onInterrupt,
		isInterruptPending = false,
		isEditingHistoryMessage = false,
		onCancelHistoryEdit,
		queuedMessages = [],
		onPromoteQueuedMessage,
	} = bindings;

	const editorRef = useRef<ChatMessageInputRef>(null);

	const warningId = useId();
	const preferencesQuery = useQuery(preferenceSettings());
	const sendShortcut = getAgentChatSendShortcut(
		preferencesQuery.data?.agent_chat_send_shortcut,
		preferencesQuery.isLoading,
	);
	const isMobile = useMediaQuery(mobileViewportMediaQuery);
	const [composerElement, setComposerElement] = useState<HTMLDivElement | null>(
		null,
	);

	const workspaceUploads = bindings.files.workspaceUploads;
	const workspaceUploadEntries = workspaceUploads?.uploads ?? [];
	const hasAttachments = bindings.files.attachments.length > 0;
	const hasWorkspaceFiles = workspaceUploadEntries.length > 0;
	const hasDraftFiles = hasAttachments || hasWorkspaceFiles;

	const editor = useComposerEditor(bindings, editorRef, hasDraftFiles);
	const files = composerFiles(bindings, editor.resetPromptCycle);

	const hasSendableContent =
		editor.hasContent ||
		editor.hasFileReferences ||
		files.hasUploadedAttachments;

	const submissionBlocked =
		isDisabled || isReadOnly || isLoading || files.hasActiveUploads;

	let canSend = false;
	if (!submissionBlocked && hasModelOptions) {
		canSend = hasSendableContent;
	}

	let showSendButton = true;
	let showStopButton = false;

	// A live recording keeps the accept action available even while
	// editing a streaming chat.
	if (isStreaming && !editor.speech.isRecording) {
		if (isEditingHistoryMessage) {
			showSendButton = false;
		} else if (!hasSendableContent && !files.hasActiveUploads) {
			showSendButton = false;
		}

		showStopButton = !showSendButton && onInterrupt !== undefined;
	}

	// Mobile viewports advertise no send shortcut.
	let sendShortcutLabel: string | undefined;
	let sendButtonKeyShortcuts: string | undefined;

	if (!isMobile) {
		if (sendShortcut === MODIFIER_AGENT_CHAT_SEND_SHORTCUT) {
			sendShortcutLabel = "Cmd/Ctrl+Enter";
			sendButtonKeyShortcuts = "Control+Enter Meta+Enter";
		} else {
			sendShortcutLabel = "Enter";
			sendButtonKeyShortcuts = "Enter";
		}
	}

	// Re-focus the editor after a send completes (isLoading goes from
	// true to false) so the user can immediately type again.
	const prevIsLoadingRef = useRef(isLoading);
	useEffect(() => {
		const wasLoading = prevIsLoadingRef.current;
		prevIsLoadingRef.current = isLoading;

		if (wasLoading && !isLoading && !isMobileViewport()) {
			editorRef.current?.focus();
		}
	}, [isLoading]);

	const submit = () => {
		const text = editorRef.current?.getValue()?.trim() ?? "";
		const hasSubmissionContent =
			Boolean(text) || files.hasUploadedAttachments || editor.hasFileReferences;

		if (submissionBlocked) {
			return;
		}

		// An empty draft promotes the first queued message instead.
		if (!hasSubmissionContent) {
			const nextMessage = queuedMessages[0];

			if (nextMessage && onPromoteQueuedMessage) {
				void onPromoteQueuedMessage(nextMessage.id);
			}

			return;
		}

		if (!hasModelOptions) {
			return;
		}

		onSend(text);
		editor.resetPromptCycle();

		if (!isMobileViewport()) {
			editorRef.current?.focus();
		}
	};

	const inlineText = (file: File, nextContent?: string) => {
		const content = nextContent ?? bindings.files.textContents?.get(file);
		const input = editorRef.current;

		if (content === undefined || !input) {
			return;
		}

		editor.resetPromptCycle();
		input.insertText(content);
		bindings.files.onRemoveAttachment?.(file);
	};

	const context: ComposerContextValue = {
		state: {
			isDisabled,
			isReadOnly,
			isLoading,
			isStreaming,
			isInterruptPending,
			isEditingHistoryMessage,
			warning: bindings.warning,
			invisibleCharCount: editor.invisibleCharCount,
			canSend,
			showSendButton,
			showStopButton,
			canAttachFiles: !isReadOnly && bindings.files.onAttach !== undefined,
			files: {
				attachments: bindings.files.attachments,
				uploadStates: bindings.files.uploadStates,
				previewUrls: bindings.files.previewUrls,
				textContents: bindings.files.textContents,
				workspaceUploads: workspaceUploads && {
					uploads: workspaceUploads.uploads,
				},
			},
			speechSupported: editor.speech.isSupported,
			speechRecording: editor.speech.isRecording,
			speechError: editor.speech.error,
			needsSetup,
		},
		actions: {
			attachFiles: files.attachFiles,
			inlineText,
			removeAttachment: bindings.files.onRemoveAttachment,
			removeWorkspaceUpload: (id) => workspaceUploads?.onRemove(id),
			resetPromptCycle: editor.resetPromptCycle,
			submit,
			startRecording: editor.startRecording,
			acceptRecording: editor.acceptRecording,
			cancelRecording: editor.cancelRecording,
			interrupt: onInterrupt,
			cancelHistoryEdit: onCancelHistoryEdit,
			contentChange: editor.contentChange,
			editorKeyDown: editor.editorKeyDown,
		},
		meta: {
			editorRef,
			warningId,
			composerElement,
			setComposerElement,
			initialValue: bindings.initialValue,
			initialEditorState: bindings.initialEditorState,
			remountKey: bindings.remountKey,
			sendShortcut,
			sendShortcutLabel,
			sendButtonKeyShortcuts,
		},
	};

	return context;
}
