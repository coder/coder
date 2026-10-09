import { cn } from "cn";
import {
	ArrowUpIcon,
	CheckIcon,
	MicIcon,
	PencilIcon,
	SquareIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import type React from "react";
import {
	createContext,
	use,
	useEffect,
	useId,
	useImperativeHandle,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { toast } from "sonner";
import { preferenceSettings } from "#/api/queries/users";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { isMobileViewport, mobileViewportMediaQuery } from "#/utils/mobile";
import { useSpeechRecognition } from "../hooks/useSpeechRecognition";
import {
	isWorkspaceUploadInProgress,
	type WorkspaceFileUpload,
} from "../hooks/useWorkspaceFileUploads";
import {
	getAgentChatSendShortcut,
	MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
} from "../utils/agentChatSendShortcut";
import {
	chatAttachmentAcceptAttribute,
	isChatAttachmentFile,
	shouldRouteFileToWorkspace,
} from "../utils/chatAttachments";
import type { ChatSlashCommand } from "../utils/slashCommands";
import {
	AttachmentPreview,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
import {
	ChatMessageInput,
	type ChatMessageInputRef,
} from "./ChatMessageInput/ChatMessageInput";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import { ImageLightbox } from "./ImageLightbox";
import { TextPreviewDialog } from "./TextPreviewDialog";
import { WorkspaceUploadPreview } from "./WorkspaceUploadPreview";

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

type ComposerContextValue = {
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

const ComposerContext = createContext<ComposerContextValue | null>(null);
const ComposerRefsContext = createContext<{
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

const workspaceRequiredAttachmentMessage =
	"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.";
const workspaceUploadPendingSendMessage =
	"Wait for the current message to finish sending, then add the file again.";

function Provider({
	bindings,
	children,
}: {
	bindings: AgentComposerBindings;
	children: React.ReactNode;
}) {
	const {
		onSend,
		isDisabled,
		isReadOnly = false,
		isLoading,
		inputRef,
		initialValue,
		initialEditorState,
		remountKey,
		onContentChange,
		hasModelOptions,
		isStreaming = false,
		onInterrupt,
		isInterruptPending = false,
		warning,
		isEditingHistoryMessage = false,
		onCancelHistoryEdit,
		userPromptHistory = [],
		queuedMessages = [],
		onPromoteQueuedMessage,
		attachments = [],
		onAttach,
		onRemoveAttachment,
		uploadStates,
		previewUrls,
		textContents,
		workspaceUploads,
		onTextPreview,
	} = bindings;
	const warningId = useId();
	const preferencesQuery = useQuery(preferenceSettings());
	const sendShortcut = getAgentChatSendShortcut(
		preferencesQuery.data?.agent_chat_send_shortcut,
		preferencesQuery.isLoading,
	);
	const isMobile = useMediaQuery(mobileViewportMediaQuery);
	const internalRef = useRef<ChatMessageInputRef>(null);
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [composerElement, setComposerElement] = useState<HTMLDivElement | null>(
		null,
	);
	const [previewImage, setPreviewImage] = useState<string | null>(null);
	const [previewText, setPreviewText] = useState<string | null>(null);
	const [previewTextFileName, setPreviewTextFileName] = useState<string | null>(
		null,
	);
	const [previewTextMediaType, setPreviewTextMediaType] = useState<
		string | null
	>(null);
	const [hasFileReferences, setHasFileReferences] = useState(false);
	const [cycleIndex, setCycleIndex] = useState<number | null>(null);
	const [cycleSavedDraft, setCycleSavedDraft] = useState<string | null>(null);
	const cycleHistorySnapshotRef = useRef<readonly string[] | null>(null);
	const currentCycleValueRef = useRef<string | null>(null);
	const previousRemountKeyRef = useRef(remountKey);
	const resetPromptCycle = () => {
		setCycleIndex(null);
		setCycleSavedDraft(null);
		cycleHistorySnapshotRef.current = null;
		currentCycleValueRef.current = null;
	};
	const applyCycleValue = (text: string) => {
		const editor = internalRef.current;
		if (!editor) return;
		currentCycleValueRef.current = text;
		editor.setValue(text);
		editor.focus();
	};
	useEffect(() => {
		if (previousRemountKeyRef.current === remountKey) return;
		previousRemountKeyRef.current = remountKey;
		// Keep in sync with resetPromptCycle without a callback dependency.
		setCycleIndex(null);
		setCycleSavedDraft(null);
		cycleHistorySnapshotRef.current = null;
		currentCycleValueRef.current = null;
	}, [remountKey]);
	const speech = useSpeechRecognition();
	const [preRecordingValue, setPreRecordingValue] = useState<string>("");
	useEffect(() => {
		if (!speech.isRecording) return;
		const editor = internalRef.current;
		if (!editor) return;
		editor.clear();
		const combined = preRecordingValue
			? `${preRecordingValue} ${speech.transcript}`
			: speech.transcript;
		if (combined) editor.insertText(combined);
	}, [speech.transcript, speech.isRecording, preRecordingValue]);
	// Delegate lazily so the forwarded handle survives Lexical remounts.
	useImperativeHandle(
		inputRef,
		() => ({
			setValue: (text) => internalRef.current?.setValue(text),
			insertText: (text) => internalRef.current?.insertText(text),
			clear: () => internalRef.current?.clear(),
			focus: () => internalRef.current?.focus(),
			getValue: () => internalRef.current?.getValue() ?? "",
			addFileReference: (ref) => internalRef.current?.addFileReference(ref),
			getContentParts: () => internalRef.current?.getContentParts() ?? [],
		}),
		[],
	);

	// Eager workspace uploads must not outlive a pending send's draft reset.
	const workspaceAttachBlockedBySend =
		isLoading && workspaceUploads?.onAttach !== undefined;
	const onWorkspaceAttach =
		isDisabled || isLoading ? undefined : workspaceUploads?.onAttach;
	const routeFiles = (files: File[]): boolean => {
		const attachable: File[] = [];
		const forWorkspace: File[] = [];
		const rejected: File[] = [];
		const workspaceRequired: File[] = [];
		for (const file of files) {
			if (onWorkspaceAttach && shouldRouteFileToWorkspace(file)) {
				forWorkspace.push(file);
			} else if (isChatAttachmentFile(file)) {
				attachable.push(file);
			} else if (workspaceUploads && shouldRouteFileToWorkspace(file)) {
				workspaceRequired.push(file);
			} else {
				rejected.push(file);
			}
		}
		if (workspaceRequired.length > 0) {
			toast.error(
				workspaceAttachBlockedBySend
					? workspaceUploadPendingSendMessage
					: (workspaceUploads?.unavailableMessage ??
							workspaceRequiredAttachmentMessage),
			);
		}
		if (rejected.length > 0) {
			toast.error(
				`Unsupported file type: ${rejected.map((file) => file.name).join(", ")}`,
			);
		}
		if (attachable.length === 0 && forWorkspace.length === 0) return false;
		resetPromptCycle();
		if (attachable.length > 0) onAttach?.(attachable);
		if (forWorkspace.length > 0) onWorkspaceAttach?.(forWorkspace);
		return true;
	};
	const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (e.target.files?.length) routeFiles(Array.from(e.target.files));
		// Reset so the same file can be selected again.
		e.target.value = "";
	};
	const handleFilePaste = (file: File) => routeFiles([file]);
	const openFilePicker = () => {
		resetPromptCycle();
		fileInputRef.current?.click();
	};
	const handleInlineText = (file: File, nextContent?: string) => {
		const content = nextContent ?? textContents?.get(file);
		if (content === undefined) return;
		const editor = internalRef.current;
		if (!editor) return;
		resetPromptCycle();
		editor.insertText(content);
		onRemoveAttachment?.(file);
	};
	const handleTextPreview = (
		content: string,
		fileName: string,
		mediaType: string,
	) => {
		if (onTextPreview) {
			onTextPreview(content, fileName, mediaType);
		} else {
			setPreviewText(content);
			setPreviewTextFileName(fileName);
			setPreviewTextMediaType(mediaType);
		}
	};
	const [isDragging, setIsDragging] = useState(false);
	const handleDragOver = (e: React.DragEvent) => {
		e.preventDefault();
		if (e.dataTransfer.types.includes("Files")) setIsDragging(true);
	};
	const handleDragLeave = (e: React.DragEvent) => {
		if (
			!(e.relatedTarget instanceof Node) ||
			!e.currentTarget.contains(e.relatedTarget)
		) {
			setIsDragging(false);
		}
	};
	const handleDrop = (e: React.DragEvent) => {
		e.preventDefault();
		setIsDragging(false);
		if (!e.dataTransfer.files.length) return;
		routeFiles(Array.from(e.dataTransfer.files));
	};
	const [hasContent, setHasContent] = useState(() =>
		Boolean(initialValue.trim()),
	);
	const [invisibleCharCount, setInvisibleCharCount] = useState(() =>
		countInvisibleCharacters(initialValue),
	);
	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasRefs: boolean,
	) => {
		// Ignore synchronous setValue echoes while cycling, but reset on user input.
		if (cycleIndex !== null && content !== currentCycleValueRef.current)
			resetPromptCycle();
		setHasContent(Boolean(content.trim()));
		setHasFileReferences(hasRefs);
		setInvisibleCharCount(countInvisibleCharacters(content));
		onContentChange(content, serializedEditorState, hasRefs);
	};
	const prevIsLoadingRef = useRef(isLoading);
	useEffect(() => {
		const wasLoading = prevIsLoadingRef.current;
		prevIsLoadingRef.current = isLoading;
		if (wasLoading && !isLoading && !isMobileViewport())
			internalRef.current?.focus();
	}, [isLoading]);
	const workspaceUploadEntries = workspaceUploads?.uploads ?? [];
	const hasActiveUploads =
		attachments.some((file) => isUploadInProgress(uploadStates?.get(file))) ||
		workspaceUploadEntries.some(isWorkspaceUploadInProgress);
	// Deferred failures remain sendable because the next send re-uploads them.
	const hasUploadedAttachments =
		attachments.some((f) => uploadStates?.get(f)?.status === "uploaded") ||
		workspaceUploadEntries.some(
			(upload) =>
				upload.status === "uploaded" ||
				upload.status === "deferred" ||
				(workspaceUploads?.deferred === true && upload.status === "error"),
		);
	const hasDraftContext =
		hasContent ||
		attachments.length > 0 ||
		workspaceUploadEntries.length > 0 ||
		hasFileReferences;
	const isComposerEffectivelyEmpty = !hasDraftContext;
	const hasSendableContent =
		hasContent || hasUploadedAttachments || hasFileReferences;
	const canSend =
		!isDisabled &&
		!isReadOnly &&
		!isLoading &&
		hasModelOptions &&
		hasSendableContent &&
		!hasActiveUploads;
	const handleSubmit = () => {
		const text = internalRef.current?.getValue()?.trim() ?? "";
		if (
			!text &&
			!hasUploadedAttachments &&
			!hasFileReferences &&
			!isDisabled &&
			!isReadOnly &&
			!isLoading &&
			!hasActiveUploads &&
			queuedMessages.length > 0 &&
			onPromoteQueuedMessage
		) {
			void onPromoteQueuedMessage(queuedMessages[0].id);
			return;
		}
		if (
			(!text && !hasUploadedAttachments && !hasFileReferences) ||
			isDisabled ||
			isReadOnly ||
			isLoading ||
			hasActiveUploads ||
			!hasModelOptions
		)
			return;
		onSend(text);
		resetPromptCycle();
		if (!isMobileViewport()) internalRef.current?.focus();
	};
	const handleStartRecording = () => {
		resetPromptCycle();
		setPreRecordingValue(internalRef.current?.getValue()?.trim() ?? "");
		speech.start();
	};
	const handleAcceptRecording = () => {
		speech.stop();
		if (!isMobileViewport()) internalRef.current?.focus();
	};
	const handleCancelRecording = () => {
		const original = preRecordingValue;
		speech.cancel();
		const editor = internalRef.current;
		if (editor) {
			editor.clear();
			if (original) editor.insertText(original);
			if (!isMobileViewport()) editor.focus();
		}
		setPreRecordingValue("");
	};
	const handleComposerKeyDown = (e: React.KeyboardEvent) => {
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
	const restoreCycleDraft = () => {
		const savedDraft = cycleSavedDraft ?? "";
		setCycleIndex(null);
		setCycleSavedDraft(null);
		cycleHistorySnapshotRef.current = null;
		applyCycleValue(savedDraft);
	};
	const handleEditorKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Escape" && cycleIndex !== null) {
			e.preventDefault();
			e.stopPropagation();
			restoreCycleDraft();
			return;
		}
		// Streaming permits cycling; cycle-aware Escape must not interrupt it.
		if (isEditingHistoryMessage || isReadOnly || isLoading) return;
		if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
		if (cycleIndex === null) {
			if (e.key !== "ArrowUp" || !isComposerEffectivelyEmpty) return;
			const cycleHistory = [...userPromptHistory];
			const latestPrompt = cycleHistory[0];
			if (latestPrompt === undefined) return;
			e.preventDefault();
			cycleHistorySnapshotRef.current = cycleHistory;
			setCycleIndex(0);
			setCycleSavedDraft(internalRef.current?.getValue() ?? "");
			applyCycleValue(latestPrompt);
			return;
		}
		e.preventDefault();
		const cycleHistory = cycleHistorySnapshotRef.current ?? userPromptHistory;
		if (e.key === "ArrowDown") {
			if (cycleIndex === 0) {
				restoreCycleDraft();
				return;
			}
			const nextIndex = cycleIndex - 1;
			const nextPrompt = cycleHistory[nextIndex];
			if (nextPrompt === undefined) {
				restoreCycleDraft();
				return;
			}
			setCycleIndex(nextIndex);
			applyCycleValue(nextPrompt);
			return;
		}
		const lastIndex = cycleHistory.length - 1;
		if (lastIndex < 0) {
			restoreCycleDraft();
			return;
		}
		const nextIndex = Math.min(cycleIndex + 1, lastIndex);
		if (nextIndex === cycleIndex) return;
		const nextPrompt = cycleHistory[nextIndex];
		if (nextPrompt === undefined) {
			restoreCycleDraft();
			return;
		}
		setCycleIndex(nextIndex);
		applyCycleValue(nextPrompt);
	};
	const sendButtonLabel = isEditingHistoryMessage
		? "Save Edit"
		: isStreaming
			? "Queue"
			: "Send";
	const draftOccupiesSlot =
		hasSendableContent || hasActiveUploads || speech.isRecording;
	const editingHoldsStop = isEditingHistoryMessage && !speech.isRecording;
	const showStopButton =
		isStreaming &&
		onInterrupt !== undefined &&
		(!draftOccupiesSlot || editingHoldsStop);
	const showSendButton =
		!isStreaming || (draftOccupiesSlot && !editingHoldsStop);
	const sendShortcutLabel = isMobile
		? undefined
		: sendShortcut === MODIFIER_AGENT_CHAT_SEND_SHORTCUT
			? "Cmd/Ctrl+Enter"
			: "Enter";
	const sendButtonKeyShortcuts = isMobile
		? undefined
		: sendShortcut === MODIFIER_AGENT_CHAT_SEND_SHORTCUT
			? "Control+Enter Meta+Enter"
			: "Enter";
	const sendButtonTooltip = sendShortcutLabel
		? `${sendButtonLabel}: ${sendShortcutLabel}`
		: sendButtonLabel;
	const context: ComposerContextValue = {
		state: {
			isDisabled,
			isReadOnly,
			isLoading,
			isStreaming,
			isInterruptPending,
			isEditingHistoryMessage,
			warning,
			isDragging,
			invisibleCharCount,
			canSend,
			showSendButton,
			showStopButton,
			canAttachFiles: onAttach !== undefined,
			speechSupported: speech.isSupported,
			speechRecording: speech.isRecording,
			speechError: speech.error,
		},
		actions: {
			openFilePicker,
			resetPromptCycle,
			submit: handleSubmit,
			startRecording: handleStartRecording,
			acceptRecording: handleAcceptRecording,
			cancelRecording: handleCancelRecording,
			interrupt: onInterrupt,
			cancelHistoryEdit: onCancelHistoryEdit,
			fileSelect: handleFileSelect,
			filePaste: handleFilePaste,
			inlineText: handleInlineText,
			textPreview: handleTextPreview,
			imagePreview: setPreviewImage,
			contentChange: handleContentChange,
			editorKeyDown: handleEditorKeyDown,
			composerKeyDown: handleComposerKeyDown,
			dragOver: handleDragOver,
			dragLeave: handleDragLeave,
			drop: handleDrop,
		},
		meta: {
			warningId,
			composerElement,
			setComposerElement,
			initialValue,
			initialEditorState,
			remountKey,
			sendShortcut,
			sendButtonLabel,
			sendButtonTooltip,
			sendButtonKeyShortcuts,
			attachments,
			onRemoveAttachment,
			uploadStates,
			previewUrls,
			textContents,
			workspaceUploads,
		},
	};
	return (
		<ComposerContext value={context}>
			<ComposerRefsContext value={{ editorRef: internalRef, fileInputRef }}>
				{children}
			</ComposerRefsContext>
			{previewImage && (
				<ImageLightbox
					src={previewImage}
					onClose={() => setPreviewImage(null)}
				/>
			)}
			{previewText !== null && (
				<TextPreviewDialog
					content={previewText}
					fileName={previewTextFileName ?? undefined}
					mediaType={previewTextMediaType ?? undefined}
					onClose={() => {
						setPreviewText(null);
						setPreviewTextFileName(null);
						setPreviewTextMediaType(null);
					}}
				/>
			)}
		</ComposerContext>
	);
}

function Frame({
	children,
	className,
	showSetupNotice = false,
}: {
	children: React.ReactNode;
	className?: string;
	showSetupNotice?: boolean;
}) {
	const { state, actions, meta } = useAgentComposer();
	const { composerElement, setComposerElement } = meta;
	useEffect(() => {
		if (!composerElement) return;
		// Radix popover wrappers are fixed-positioned, so their
		// inset values need to be in layout-viewport coordinates.
		// The visual viewport can be offset inside the layout
		// viewport when the mobile keyboard is open. Treat
		// `visualViewport.offsetTop` as a clamp only when it yields
		// a positive height, since mobile WebKit can report mixed
		// coordinate systems while the keyboard is settling.
		const viewport = globalThis.visualViewport;
		const root = document.documentElement;
		const fixedProbe = document.createElement("div");
		Object.assign(fixedProbe.style, {
			position: "fixed",
			bottom: "0",
			left: "0",
			width: "0",
			height: "0",
			pointerEvents: "none",
			visibility: "hidden",
		});
		document.body.appendChild(fixedProbe);
		const composerGap = 8;
		const viewportPadding = 16;
		const minimumMenuHeight = 96;
		const update = () => {
			const rect = composerElement.getBoundingClientRect();
			const fixedViewportBottom = fixedProbe.getBoundingClientRect().bottom;
			const visibleViewportTop = viewport?.offsetTop ?? 0;
			const bottom = Math.max(0, fixedViewportBottom - rect.bottom);
			// Keep the dropdown's bottom edge above the software keyboard,
			// which covers the bottom of the layout viewport without moving
			// fixed-positioned elements.
			const keyboardInset = viewport
				? Math.max(
						0,
						fixedViewportBottom - (viewport.offsetTop + viewport.height),
					)
				: 0;
			const aboveComposerBottom = Math.max(
				0,
				fixedViewportBottom - rect.top + composerGap,
				keyboardInset + composerGap,
			);
			const dropdownBottomEdgeTop = fixedViewportBottom - aboveComposerBottom;
			const maxHeightCandidates = [
				dropdownBottomEdgeTop - visibleViewportTop - viewportPadding,
				dropdownBottomEdgeTop - viewportPadding,
			].filter((height) => height > 0);
			const aboveComposerMaxHeight = Math.max(
				minimumMenuHeight,
				maxHeightCandidates.length > 0 ? Math.min(...maxHeightCandidates) : 0,
			);
			root.style.setProperty("--mobile-dropdown-bottom", `${bottom}px`);
			root.style.setProperty("--mobile-dropdown-left", `${rect.left}px`);
			root.style.setProperty("--mobile-dropdown-width", `${rect.width}px`);
			root.style.setProperty(
				"--mobile-dropdown-above-composer-bottom",
				`${aboveComposerBottom}px`,
			);
			root.style.setProperty(
				"--mobile-dropdown-above-composer-max-height",
				`${aboveComposerMaxHeight}px`,
			);
		};
		const animationFrameIDs = new Set<number>();
		const timeoutIDs = new Set<ReturnType<typeof setTimeout>>();
		const cancelScheduledUpdates = () => {
			for (const id of animationFrameIDs) cancelAnimationFrame(id);
			animationFrameIDs.clear();
			for (const id of timeoutIDs) clearTimeout(id);
			timeoutIDs.clear();
		};
		const queueAnimationFrame = (callback: () => void) => {
			const id = requestAnimationFrame(() => {
				animationFrameIDs.delete(id);
				callback();
			});
			animationFrameIDs.add(id);
		};
		const scheduleUpdate = () => {
			cancelScheduledUpdates();
			update();
			// Mobile WebKit can finish keyboard panning after focus and
			// input events. Re-read geometry after the viewport settles so
			// the first slash-menu render is not stuck under the composer.
			queueAnimationFrame(() => {
				update();
				queueAnimationFrame(update);
			});
			for (const delay of [50, 150, 300]) {
				const id = setTimeout(() => {
					timeoutIDs.delete(id);
					update();
				}, delay);
				timeoutIDs.add(id);
			}
		};
		scheduleUpdate();
		const ro = new ResizeObserver(scheduleUpdate);
		ro.observe(composerElement);
		addEventListener("resize", scheduleUpdate);
		addEventListener("scroll", scheduleUpdate, { passive: true });
		addEventListener("focusin", scheduleUpdate);
		addEventListener("focusout", scheduleUpdate);
		composerElement.addEventListener("input", scheduleUpdate);
		composerElement.addEventListener("keyup", scheduleUpdate);
		document.addEventListener("selectionchange", scheduleUpdate);
		viewport?.addEventListener("resize", scheduleUpdate);
		viewport?.addEventListener("scroll", scheduleUpdate);
		viewport?.addEventListener("scrollend", scheduleUpdate);
		return () => {
			ro.disconnect();
			cancelScheduledUpdates();
			removeEventListener("resize", scheduleUpdate);
			removeEventListener("scroll", scheduleUpdate);
			removeEventListener("focusin", scheduleUpdate);
			removeEventListener("focusout", scheduleUpdate);
			composerElement.removeEventListener("input", scheduleUpdate);
			composerElement.removeEventListener("keyup", scheduleUpdate);
			document.removeEventListener("selectionchange", scheduleUpdate);
			viewport?.removeEventListener("resize", scheduleUpdate);
			viewport?.removeEventListener("scroll", scheduleUpdate);
			viewport?.removeEventListener("scrollend", scheduleUpdate);
			fixedProbe.remove();
			root.style.removeProperty("--mobile-dropdown-bottom");
			root.style.removeProperty("--mobile-dropdown-left");
			root.style.removeProperty("--mobile-dropdown-width");
			root.style.removeProperty("--mobile-dropdown-above-composer-bottom");
			root.style.removeProperty("--mobile-dropdown-above-composer-max-height");
		};
	}, [composerElement]);
	return (
		<div
			ref={setComposerElement}
			data-testid="chat-composer"
			className={cn(
				"relative z-10 rounded-2xl bg-surface-secondary sm:bg-surface-secondary/45 p-1 shadow-xs has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40",
				showSetupNotice && "sm:bg-surface-secondary",
				state.isDragging && "ring-2 ring-content-link/40",
				(state.isEditingHistoryMessage || state.warning) &&
					"shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				className,
			)}
			onKeyDown={actions.composerKeyDown}
			onDragOver={state.canAttachFiles ? actions.dragOver : undefined}
			onDragLeave={state.canAttachFiles ? actions.dragLeave : undefined}
			onDrop={state.canAttachFiles ? actions.drop : undefined}
		>
			{children}
		</div>
	);
}

function Editor({
	placeholder = "Type a message...",
	workspaceSkills,
	slashCommands,
	hasWorkspace,
}: {
	placeholder?: string;
	workspaceSkills?: readonly SkillMetadata[];
	slashCommands?: readonly ChatSlashCommand[];
	hasWorkspace: boolean;
}) {
	const { state, actions, meta } = useAgentComposer();
	const refs = use(ComposerRefsContext);
	if (!refs)
		throw new Error("Editor must be used inside AgentComposer.Provider");
	const { editorRef } = refs;
	return (
		<ChatMessageInput
			ref={editorRef}
			onFilePaste={state.canAttachFiles ? actions.filePaste : undefined}
			acceptFilePasteWhileDisabled={state.isLoading && !state.isReadOnly}
			onPaste={actions.resetPromptCycle}
			aria-label="Chat message"
			aria-describedby={state.warning ? meta.warningId : undefined}
			className="min-h-[60px] sm:min-h-24 w-full resize-none bg-transparent px-3 py-2 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
			placeholder={placeholder}
			initialValue={meta.initialValue}
			initialEditorState={meta.initialEditorState}
			remountKey={meta.remountKey}
			onChange={actions.contentChange}
			onKeyDown={actions.editorKeyDown}
			onEnter={actions.submit}
			sendShortcut={meta.sendShortcut}
			disabled={state.isReadOnly || state.isLoading}
			hasWorkspace={hasWorkspace}
			workspaceSkills={workspaceSkills}
			slashCommands={slashCommands}
			skillsMenuAnchor={meta.composerElement}
		/>
	);
}

function Attachments() {
	const { state, actions, meta } = useAgentComposer();
	const refs = use(ComposerRefsContext);
	if (!refs)
		throw new Error("Attachments must be used inside AgentComposer.Provider");
	const { fileInputRef } = refs;
	return (
		<>
			{meta.onRemoveAttachment && (
				<AttachmentPreview
					attachments={meta.attachments}
					onRemove={meta.onRemoveAttachment}
					uploadStates={meta.uploadStates}
					previewUrls={meta.previewUrls}
					onPreview={actions.imagePreview}
					textContents={meta.textContents}
					onTextPreview={actions.textPreview}
					onInlineText={actions.inlineText}
				/>
			)}
			{meta.workspaceUploads && (
				<WorkspaceUploadPreview
					uploads={meta.workspaceUploads.uploads}
					onRemove={meta.workspaceUploads.onRemove}
				/>
			)}
			{/* Allow all workspace upload types so routeFiles can explain refusals on iOS. */}
			{state.canAttachFiles && (
				<input
					ref={fileInputRef}
					type="file"
					data-testid="chat-attachment-file-input"
					multiple
					accept={
						meta.workspaceUploads ? undefined : chatAttachmentAcceptAttribute
					}
					onChange={actions.fileSelect}
					className="hidden"
				/>
			)}
		</>
	);
}

function Toolbar({ children }: { children: React.ReactNode }) {
	return (
		<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
			{children}
		</div>
	);
}

function VoiceInput() {
	const { state, actions } = useAgentComposer();
	if (!state.speechSupported) return null;
	return (
		<>
			<Button
				type="button"
				variant="subtle"
				size="icon"
				className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
				onClick={
					state.speechRecording
						? actions.cancelRecording
						: actions.startRecording
				}
				disabled={state.isDisabled}
				aria-label={
					state.speechRecording ? "Cancel voice input" : "Voice input"
				}
			>
				{state.speechRecording ? <XIcon /> : <MicIcon strokeWidth={1.5} />}
			</Button>
			{state.speechError && !state.speechRecording && (
				<span className="text-2xs text-content-destructive" role="alert">
					{state.speechError === "not-allowed"
						? "Mic access denied"
						: "Voice input failed"}
				</span>
			)}
		</>
	);
}

function PrimaryAction() {
	const { state, actions, meta } = useAgentComposer();
	return (
		<>
			{state.showSendButton && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							size="icon"
							variant="default"
							className="size-7 rounded-full transition-colors [&>svg]:size-5! [&>svg]:p-0"
							onClick={
								state.speechRecording ? actions.acceptRecording : actions.submit
							}
							disabled={state.speechRecording ? false : !state.canSend}
							aria-keyshortcuts={meta.sendButtonKeyShortcuts}
						>
							{state.isLoading && !state.isInterruptPending ? (
								<Spinner size="sm" loading aria-hidden="true" />
							) : state.speechRecording ? (
								<CheckIcon />
							) : (
								<ArrowUpIcon />
							)}
							<span className="sr-only">
								{state.speechRecording
									? "Accept voice input"
									: meta.sendButtonLabel}
							</span>
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">
						{state.speechRecording
							? "Accept voice input"
							: meta.sendButtonTooltip}
					</TooltipContent>
				</Tooltip>
			)}
			{state.showStopButton && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							size="icon"
							variant="default"
							className="size-7 rounded-full transition-colors [&>svg]:size-3! [&>svg]:p-0"
							onClick={actions.interrupt}
							disabled={state.isInterruptPending}
						>
							<SquareIcon className="fill-current" />
							<span className="sr-only">Stop</span>
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">
						{state.isInterruptPending ? "Interrupting…" : "Stop"}
					</TooltipContent>
				</Tooltip>
			)}
			{state.isInterruptPending && state.isStreaming && (
				<span role="status" className="sr-only">
					Interrupting. Waiting for the agent to stop.
				</span>
			)}
		</>
	);
}

function InvisibleCharacterWarning() {
	const { state } = useAgentComposer();
	const { invisibleCharCount } = state;
	if (invisibleCharCount === 0) return null;
	// Unlike admin/user prompt textareas (which strip invisible chars
	// server-side on save), chat messages are free-form user input. Warn
	// without silently mutating them so users can review hidden content.
	// This guards against social engineering that tricks users into pasting
	// prompts with hidden LLM instructions encoded as zero-width characters.
	return (
		<div className="px-3 pb-1">
			<Alert severity="warning">
				<AlertDescription>
					This message contains {invisibleCharCount} invisible Unicode character
					{invisibleCharCount !== 1 ? "s" : ""} that could hide content. Review
					carefully before sending.
				</AlertDescription>
			</Alert>
		</div>
	);
}

function Warning() {
	const { state, meta } = useAgentComposer();
	if (!state.warning) return null;
	return (
		<div
			id={meta.warningId}
			className="flex items-start gap-1.5 border-b border-border/70 px-3 py-1.5 text-xs font-medium text-content-warning"
		>
			<TriangleAlertIcon className="mt-px size-3.5 shrink-0" />
			{state.warning}
		</div>
	);
}

function EditBanner() {
	const { state, actions } = useAgentComposer();
	if (!state.isEditingHistoryMessage) return null;
	return (
		<div className="flex items-center justify-between border-b border-border/70 px-3 py-1.5">
			<span className="flex items-center gap-1.5 text-xs font-medium text-content-warning">
				<PencilIcon className="size-3.5" />
				Editing will delete all subsequent messages and restart the conversation
				here.
			</span>
			<Button
				type="button"
				variant="subtle"
				size="icon"
				aria-label="Cancel editing"
				onClick={actions.cancelHistoryEdit}
				disabled={state.isLoading}
				className="size-6 rounded text-content-warning hover:text-content-primary"
			>
				<XIcon className="size-3.5" />
			</Button>
		</div>
	);
}

/** Composable UI for the agent chat draft runtime. */
export const AgentComposer = {
	Provider,
	Frame,
	Editor,
	Attachments,
	Toolbar,
	VoiceInput,
	PrimaryAction,
	InvisibleCharacterWarning,
	Warning,
	EditBanner,
};
