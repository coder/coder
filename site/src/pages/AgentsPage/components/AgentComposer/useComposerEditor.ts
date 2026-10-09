import type React from "react";
import { useEffect, useImperativeHandle, useRef, useState } from "react";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { isMobileViewport } from "#/utils/mobile";
import { useSpeechRecognition } from "../../hooks/useSpeechRecognition";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";
import type { AgentComposerBindings } from "./context";

type PromptCycle = {
	history: readonly string[];
	index: number;
	savedDraft: string;
};

/** Owns document interactions without controlling Lexical's draft value. */
export function useComposerEditor(
	bindings: AgentComposerBindings,
	editorRef: React.RefObject<ChatMessageInputRef | null>,
	hasAttachments: boolean,
) {
	const {
		inputRef,
		initialValue,
		remountKey,
		onContentChange,
		isLoading,
		isReadOnly = false,
		isEditingHistoryMessage = false,
		userPromptHistory = [],
	} = bindings;

	const [hasContent, setHasContent] = useState(() =>
		Boolean(initialValue.trim()),
	);
	const [hasFileReferences, setHasFileReferences] = useState(false);
	const [invisibleCharCount, setInvisibleCharCount] = useState(() =>
		countInvisibleCharacters(initialValue),
	);

	const [promptCycle, setPromptCycle] = useState<PromptCycle | null>(null);
	const currentCycleValueRef = useRef<string | null>(null);

	const prevIsLoadingRef = useRef(isLoading);

	const speech = useSpeechRecognition();
	const [preRecordingValue, setPreRecordingValue] = useState("");

	useEffect(() => {
		// Lexical remounts independently of the provider, ending the history session.
		setPromptCycle(null);
		currentCycleValueRef.current = null;
	}, [remountKey]);

	useEffect(() => {
		if (!speech.isRecording) {
			return;
		}

		const editor = editorRef.current;

		if (!editor) {
			return;
		}

		editor.clear();
		const combined = preRecordingValue
			? `${preRecordingValue} ${speech.transcript}`
			: speech.transcript;

		if (combined) {
			editor.insertText(combined);
		}
	}, [speech.transcript, speech.isRecording, preRecordingValue, editorRef]);

	// Delegate lazily so the forwarded handle survives Lexical remounts.
	useImperativeHandle(
		inputRef,
		() => ({
			setValue: (text) => editorRef.current?.setValue(text),
			insertText: (text) => editorRef.current?.insertText(text),
			clear: () => editorRef.current?.clear(),
			focus: () => editorRef.current?.focus(),
			getValue: () => editorRef.current?.getValue() ?? "",
			addFileReference: (ref) => editorRef.current?.addFileReference(ref),
			getContentParts: () => editorRef.current?.getContentParts() ?? [],
		}),
		[editorRef],
	);

	useEffect(() => {
		const wasLoading = prevIsLoadingRef.current;
		prevIsLoadingRef.current = isLoading;

		if (wasLoading && !isLoading && !isMobileViewport()) {
			editorRef.current?.focus();
		}
	}, [isLoading, editorRef]);

	const resetPromptCycle = () => {
		setPromptCycle(null);
		currentCycleValueRef.current = null;
	};

	const applyCycleValue = (text: string) => {
		const editor = editorRef.current;

		if (!editor) {
			return;
		}

		// Editor callbacks may run before React commits the next history index.
		currentCycleValueRef.current = text;
		editor.setValue(text);
		editor.focus();
	};

	const contentChange: AgentComposerBindings["onContentChange"] = (
		content,
		serializedEditorState,
		hasRefs,
	) => {
		if (promptCycle !== null && content !== currentCycleValueRef.current) {
			resetPromptCycle();
		}

		setHasContent(Boolean(content.trim()));
		setHasFileReferences(hasRefs);
		setInvisibleCharCount(countInvisibleCharacters(content));

		onContentChange(content, serializedEditorState, hasRefs);
	};

	const restoreCycleDraft = () => {
		const savedDraft = promptCycle?.savedDraft ?? "";

		resetPromptCycle();
		applyCycleValue(savedDraft);
	};

	const editorKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Escape" && promptCycle !== null) {
			e.preventDefault();
			e.stopPropagation();
			restoreCycleDraft();
			return;
		}

		// Streaming permits cycling; cycle-aware Escape must not interrupt it.
		if (isEditingHistoryMessage || isReadOnly || isLoading) {
			return;
		}

		if (e.key !== "ArrowUp" && e.key !== "ArrowDown") {
			return;
		}

		if (promptCycle === null) {
			if (
				e.key !== "ArrowUp" ||
				hasContent ||
				hasAttachments ||
				hasFileReferences
			) {
				return;
			}

			const cycleHistory = [...userPromptHistory];
			const latestPrompt = cycleHistory[0];

			if (latestPrompt === undefined) {
				return;
			}

			e.preventDefault();
			setPromptCycle({
				history: cycleHistory,
				index: 0,
				savedDraft: editorRef.current?.getValue() ?? "",
			});
			applyCycleValue(latestPrompt);
			return;
		}

		e.preventDefault();
		const nextIndex =
			e.key === "ArrowDown"
				? promptCycle.index - 1
				: Math.min(promptCycle.index + 1, promptCycle.history.length - 1);

		if (nextIndex === promptCycle.index) {
			return;
		}

		const nextPrompt = promptCycle.history[nextIndex];

		if (nextPrompt === undefined) {
			restoreCycleDraft();
			return;
		}

		setPromptCycle({ ...promptCycle, index: nextIndex });
		applyCycleValue(nextPrompt);
	};

	const startRecording = () => {
		resetPromptCycle();
		setPreRecordingValue(editorRef.current?.getValue()?.trim() ?? "");
		speech.start();
	};

	const acceptRecording = () => {
		speech.stop();

		if (!isMobileViewport()) {
			editorRef.current?.focus();
		}
	};

	const cancelRecording = () => {
		const original = preRecordingValue;
		speech.cancel();

		const editor = editorRef.current;

		if (editor) {
			editor.clear();

			if (original) {
				editor.insertText(original);
			}

			if (!isMobileViewport()) {
				editor.focus();
			}
		}

		setPreRecordingValue("");
	};

	return {
		hasContent,
		hasFileReferences,
		invisibleCharCount,
		speech,
		resetPromptCycle,
		contentChange,
		editorKeyDown,
		startRecording,
		acceptRecording,
		cancelRecording,
	};
}
