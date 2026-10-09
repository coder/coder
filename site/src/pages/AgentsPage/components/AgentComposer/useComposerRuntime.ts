import type React from "react";
import { useId, useRef, useState } from "react";
import { useQuery } from "react-query";
import { preferenceSettings } from "#/api/queries/users";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { isMobileViewport, mobileViewportMediaQuery } from "#/utils/mobile";
import {
	getAgentChatSendShortcut,
	MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
} from "../../utils/agentChatSendShortcut";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";
import type { AgentComposerBindings, ComposerContextValue } from "./context";
import { useComposerEditor } from "./useComposerEditor";
import { useComposerFiles } from "./useComposerFiles";

/** Coordinates submission and exposes the public state/actions/meta contract. */
export function useComposerRuntime(bindings: AgentComposerBindings) {
	const {
		onSend,
		isDisabled,
		isReadOnly = false,
		isLoading,
		hasModelOptions,
		isStreaming = false,
		onInterrupt,
		isInterruptPending = false,
		isEditingHistoryMessage = false,
		onCancelHistoryEdit,
		queuedMessages = [],
		onPromoteQueuedMessage,
		attachments = [],
		workspaceUploads,
	} = bindings;

	const editorRef = useRef<ChatMessageInputRef>(null);
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [attachEditor] = useState(
		() => (editor: ChatMessageInputRef | null) => {
			editorRef.current = editor;
		},
	);
	const [attachFileInput] = useState(() => (input: HTMLInputElement | null) => {
		fileInputRef.current = input;
	});

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

	const editor = useComposerEditor(
		bindings,
		editorRef,
		attachments.length > 0 || (workspaceUploads?.uploads.length ?? 0) > 0,
	);
	const files = useComposerFiles(
		bindings,
		editorRef,
		fileInputRef,
		editor.resetPromptCycle,
	);

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

	if (isStreaming && !editor.speech.isRecording) {
		if (isEditingHistoryMessage) {
			showSendButton = false;
		} else if (!hasSendableContent && !files.hasActiveUploads) {
			showSendButton = false;
		}

		showStopButton = !showSendButton && onInterrupt !== undefined;
	}

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

	const submit = () => {
		const text = editorRef.current?.getValue()?.trim() ?? "";
		const hasSubmissionContent =
			Boolean(text) || files.hasUploadedAttachments || editor.hasFileReferences;

		if (
			!hasSubmissionContent &&
			!submissionBlocked &&
			queuedMessages.length > 0 &&
			onPromoteQueuedMessage
		) {
			void onPromoteQueuedMessage(queuedMessages[0].id);
			return;
		}

		if (!hasSubmissionContent || submissionBlocked || !hasModelOptions) {
			return;
		}

		const completion = onSend(text);
		editor.resetPromptCycle();

		const restoreFocus = () => {
			if (!isMobileViewport()) {
				editorRef.current?.focus();
			}
		};

		if (completion) {
			void completion.then(restoreFocus, restoreFocus);
		} else {
			restoreFocus();
		}
	};

	const composerKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Escape") {
			if (isEditingHistoryMessage) {
				e.preventDefault();
				onCancelHistoryEdit?.();
			} else if (isStreaming && onInterrupt && !isInterruptPending) {
				e.preventDefault();
				onInterrupt();
			}
		}
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
			isDragging: files.isDragging,
			invisibleCharCount: editor.invisibleCharCount,
			canSend,
			showSendButton,
			showStopButton,
			canAttachFiles: bindings.onAttach !== undefined,
			speechSupported: editor.speech.isSupported,
			speechRecording: editor.speech.isRecording,
			speechError: editor.speech.error,
		},
		actions: {
			...files.actions,
			resetPromptCycle: editor.resetPromptCycle,
			submit,
			startRecording: editor.startRecording,
			acceptRecording: editor.acceptRecording,
			cancelRecording: editor.cancelRecording,
			interrupt: onInterrupt,
			cancelHistoryEdit: onCancelHistoryEdit,
			contentChange: editor.contentChange,
			editorKeyDown: editor.editorKeyDown,
			composerKeyDown,
		},
		meta: {
			attachEditor,
			attachFileInput,
			warningId,
			composerElement,
			setComposerElement,
			initialValue: bindings.initialValue,
			initialEditorState: bindings.initialEditorState,
			remountKey: bindings.remountKey,
			sendShortcut,
			sendShortcutLabel,
			sendButtonKeyShortcuts,
			attachments,
			onRemoveAttachment: bindings.onRemoveAttachment,
			uploadStates: bindings.uploadStates,
			previewUrls: bindings.previewUrls,
			textContents: bindings.textContents,
			workspaceUploads,
		},
	};

	return { context, previews: files.previews };
}
