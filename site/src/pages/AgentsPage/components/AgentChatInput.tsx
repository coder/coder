import type * as TypesGen from "api/typesGenerated";
import type { ChatMessagePart, ChatQueuedMessage } from "api/typesGenerated";
import { useSpeechRecognition } from "hooks/useSpeechRecognition";
import {
	AlertTriangleIcon,
	ArrowUpIcon,
	CheckIcon,
	ClipboardPasteIcon,
	ImageIcon,
	MicIcon,
	PencilIcon,
	Square,
	XIcon,
} from "lucide-react";
import type React from "react";
import {
	type FC,
	type ReactNode,
	useEffect,
	useImperativeHandle,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import { isMobileViewport } from "utils/mobile";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/components/ai-elements";
import { Button } from "#/components/Button/Button";
import {
	ChatMessageInput,
	type ChatMessageInputRef,
} from "#/components/ChatMessageInput/ChatMessageInput";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
} from "../utils/fetchTextAttachment";
import { formatProviderLabel } from "../utils/modelOptions";
import { ImageLightbox } from "./ImageLightbox";
import { MCPServerPicker } from "./MCPServerPicker";
import { QueuedMessagesList } from "./QueuedMessagesList";
import { TextPreviewDialog } from "./TextPreviewDialog";

export type { ChatMessageInputRef } from "#/components/ChatMessageInput/ChatMessageInput";

export type UploadState = {
	status: "uploading" | "uploaded" | "error";
	fileId?: string;
	error?: string;
};

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
	// Percentage (0–100) at which the context will be compacted.
	readonly compressionThreshold?: number;
}

interface AgentChatInputProps {
	onSend: (message: string) => void;
	placeholder?: string;
	isDisabled: boolean;
	isLoading: boolean;
	// Ref for the Lexical editor, exposed for imperative access.
	inputRef?: React.Ref<ChatMessageInputRef>;
	// Initial text to seed the editor with.
	initialValue?: string;
	// Called on every text change inside the editor.
	onContentChange?: (content: string) => void;
	// Model selector.
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	// Status messages.
	inputStatusText: string | null;
	modelCatalogStatusMessage: string | null;
	// Streaming controls (optional, for the detail page).
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	// Extra controls rendered in the left action area (e.g. workspace
	// selector on the create page).
	leftActions?: ReactNode;
	// Queued user messages rendered above the textarea.
	queuedMessages?: readonly ChatQueuedMessage[];
	onDeleteQueuedMessage?: (id: number) => Promise<void> | void;
	onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
	// Queue editing state, owned by the parent.
	editingQueuedMessageID?: number | null;
	onStartQueueEdit?: (
		id: number,
		text: string,
		fileBlocks: readonly ChatMessagePart[],
	) => void;
	onCancelQueueEdit?: () => void;
	// History editing state, owned by the parent.
	isEditingHistoryMessage?: boolean;
	onCancelHistoryEdit?: () => void;

	// Optional context-usage summary shown to the left of the send button.
	// Pass `null` to render fallback values (e.g. when limit is unknown).
	// Omit entirely to hide the indicator.
	contextUsage?: AgentContextUsage | null;
	attachments?: readonly File[];
	onAttach?: (files: File[]) => void;
	onRemoveAttachment?: (attachment: number | File) => void;
	uploadStates?: Map<File, UploadState>;
	previewUrls?: Map<File, string>;
	textContents?: Map<File, string>;
	onTextPreview?: (content: string, fileName: string) => void;
	// MCP Server picker.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
}
const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

const formatTokenCount = (value: number | undefined): string =>
	hasFiniteTokenValue(value) ? value.toLocaleString() : "--";

const formatTokenCountCompact = (value: number | undefined): string => {
	if (!hasFiniteTokenValue(value)) {
		return "--";
	}
	if (value >= 1_000_000) {
		const m = value / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1).replace(/\.0$/, "")}M`;
	}
	if (value >= 1_000) {
		const k = value / 1_000;
		return `${Number.isInteger(k) ? k : k.toFixed(1).replace(/\.0$/, "")}K`;
	}
	return String(value);
};

const getIndicatorToneClassName = (percentUsed: number | null): string => {
	if (percentUsed === null) {
		return "text-content-secondary/60";
	}
	if (percentUsed >= 95) {
		return "text-content-destructive";
	}
	if (percentUsed >= 85) {
		return "text-content-warning";
	}
	return "text-content-secondary/60";
};

const RING_SIZE = 18;
const RING_STROKE = 2.5;
const RING_RADIUS = (RING_SIZE - RING_STROKE) / 2;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

const ContextUsageIndicator: FC<{ usage: AgentContextUsage | null }> = ({
	usage,
}) => {
	const usedTokens = hasFiniteTokenValue(usage?.usedTokens)
		? usage.usedTokens
		: undefined;
	const contextLimitTokens = hasFiniteTokenValue(usage?.contextLimitTokens)
		? usage.contextLimitTokens
		: undefined;
	const percentUsed =
		usedTokens !== undefined &&
		contextLimitTokens !== undefined &&
		contextLimitTokens > 0
			? (usedTokens / contextLimitTokens) * 100
			: null;
	const hasPercent = percentUsed !== null;
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 100;
	const dashOffset =
		RING_CIRCUMFERENCE - (clampedPercent / 100) * RING_CIRCUMFERENCE;
	const toneClassName = getIndicatorToneClassName(percentUsed);
	const ariaLabel = hasPercent
		? `Context usage ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.`
		: "Context usage";

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={ariaLabel}
					className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-none transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
				>
					<svg
						className={cn("size-icon-sm -rotate-90", toneClassName)}
						viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}
						aria-hidden
					>
						<circle
							cx={RING_SIZE / 2}
							cy={RING_SIZE / 2}
							r={RING_RADIUS}
							fill="none"
							strokeWidth={RING_STROKE}
							className="stroke-content-secondary/25"
						/>
						<circle
							cx={RING_SIZE / 2}
							cy={RING_SIZE / 2}
							r={RING_RADIUS}
							fill="none"
							strokeWidth={RING_STROKE}
							strokeLinecap="round"
							className="stroke-current transition-all duration-300 ease-out"
							style={{
								strokeDasharray: `${RING_CIRCUMFERENCE} ${RING_CIRCUMFERENCE}`,
								strokeDashoffset: dashOffset,
							}}
						/>
					</svg>
				</button>
			</TooltipTrigger>
			<TooltipContent side="top">
				<div className="text-xs text-content-primary">
					{hasPercent
						? `${percentLabel} – ${formatTokenCountCompact(usedTokens)} / ${formatTokenCountCompact(contextLimitTokens)} context used`
						: "Context usage unavailable"}
					{hasPercent &&
						usage?.compressionThreshold !== undefined &&
						usage.compressionThreshold > 0 && (
							<div className="mt-1 text-content-secondary">
								Compacts at {usage.compressionThreshold}%
							</div>
						)}
				</div>
			</TooltipContent>
		</Tooltip>
	);
};

/** Renders an image thumbnail from a pre-created preview URL. */
export const ImageThumbnail: FC<{
	previewUrl: string;
	name: string;
	className?: string;
}> = ({ previewUrl, name, className }) => (
	<img
		src={previewUrl}
		alt={name}
		className={cn(
			"h-16 w-16 rounded-md border border-border-default object-cover",
			className,
		)}
	/>
);

/** Renders a horizontal strip of attachment thumbnails above the input. */
export const AttachmentPreview: FC<{
	attachments: readonly File[];
	onRemove: (attachment: number | File) => void;
	uploadStates?: Map<File, UploadState>;
	previewUrls?: Map<File, string>;
	onPreview?: (url: string) => void;
	textContents?: Map<File, string>;
	onTextPreview?: (content: string, fileName: string) => void;
	onInlineText?: (file: File, content?: string) => void;
}> = ({
	attachments,
	onRemove,
	uploadStates,
	previewUrls,
	onPreview,
	textContents,
	onTextPreview,
	onInlineText,
}) => {
	const textAttachmentLoadControllerRef = useRef<AbortController | null>(null);

	useEffect(() => {
		return () => textAttachmentLoadControllerRef.current?.abort();
	}, []);

	if (attachments.length === 0) return null;

	const loadTextAttachmentContent = async (
		content: string | undefined,
		fileId: string | undefined,
	): Promise<string | undefined> => {
		textAttachmentLoadControllerRef.current?.abort();
		if (content !== undefined || !fileId) {
			textAttachmentLoadControllerRef.current = null;
			return content;
		}
		const controller = new AbortController();
		textAttachmentLoadControllerRef.current = controller;
		try {
			const fetchedContent = await fetchTextAttachmentContent(
				fileId,
				controller.signal,
			);
			if (textAttachmentLoadControllerRef.current === controller) {
				textAttachmentLoadControllerRef.current = null;
			}
			return fetchedContent;
		} catch (err) {
			if (textAttachmentLoadControllerRef.current === controller) {
				textAttachmentLoadControllerRef.current = null;
			}
			if (err instanceof Error && err.name === "AbortError") {
				return undefined;
			}
			console.error("Failed to load text attachment:", err);
			return undefined;
		}
	};

	return (
		<div className="flex gap-2 overflow-x-auto border-b border-border-default/50 px-3 py-2">
			{attachments.map((file, index) => {
				const uploadState = uploadStates?.get(file);
				const previewUrl = previewUrls?.get(file) ?? "";
				const textContent = textContents?.get(file);
				const textFileId =
					uploadState?.status === "uploaded" ? uploadState.fileId : undefined;
				const hasTextAttachment =
					file.type === "text/plain" &&
					(textContent !== undefined || textFileId !== undefined);
				return (
					<div
						// Key combines file metadata with index as a fallback for
						// duplicate names. Acceptable for a small, append-only list.
						key={`${file.name}-${file.size}-${file.lastModified}-${index}`}
						className="group relative"
					>
						{file.type.startsWith("image/") && previewUrl ? (
							<button
								type="button"
								className="border-0 bg-transparent p-0 cursor-pointer transition-opacity hover:opacity-80"
								onClick={() => onPreview?.(previewUrl)}
							>
								<ImageThumbnail previewUrl={previewUrl} name={file.name} />
							</button>
						) : hasTextAttachment ? (
							<button
								type="button"
								aria-label="View text attachment"
								className="flex h-16 w-28 flex-col items-start justify-start overflow-hidden rounded-md border-0 bg-surface-tertiary p-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
								onClick={async () => {
									const nextContent = await loadTextAttachmentContent(
										textContent,
										textFileId,
									);
									if (nextContent !== undefined) {
										onTextPreview?.(nextContent, file.name);
									}
								}}
							>
								<span className="line-clamp-3 w-full font-mono text-2xs text-content-secondary">
									{formatTextAttachmentPreview(textContent ?? "")}
								</span>
							</button>
						) : (
							<div className="flex h-16 w-16 items-center justify-center rounded-md border border-border-default bg-surface-secondary text-xs text-content-secondary">
								{file.name.split(".").pop()?.toUpperCase() || "FILE"}
							</div>
						)}
						{hasTextAttachment && (
							<button
								type="button"
								onClick={async () => {
									const nextContent = await loadTextAttachmentContent(
										textContent,
										textFileId,
									);
									onInlineText?.(file, nextContent);
								}}
								className="absolute -bottom-2 -right-2 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full border-0 bg-surface-primary text-content-secondary shadow-sm opacity-0 transition-opacity hover:bg-surface-secondary hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
								aria-label="Paste inline"
							>
								<ClipboardPasteIcon className="h-3.5 w-3.5" />
							</button>
						)}
						{uploadState?.status === "uploading" && (
							<div className="absolute inset-0 flex items-center justify-center rounded-md bg-overlay">
								<Spinner className="h-5 w-5 text-white" loading />
							</div>
						)}
						{uploadState?.status === "error" && (
							<Tooltip>
								<TooltipTrigger asChild>
									<div
										className="absolute inset-0 flex items-center justify-center rounded-md bg-overlay"
										role="img"
										aria-label="Upload error"
									>
										<AlertTriangleIcon className="h-5 w-5 text-content-warning" />
									</div>
								</TooltipTrigger>
								<TooltipContent side="top">
									<p className="max-w-xs text-xs">
										{uploadState.error ?? "Upload failed"}
									</p>
								</TooltipContent>
							</Tooltip>
						)}
						<button
							type="button"
							onClick={() => onRemove(index)}
							className="absolute -right-2 -top-2 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full border-0 bg-surface-primary text-content-secondary shadow-sm opacity-0 transition-opacity hover:bg-surface-secondary hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
							aria-label={`Remove ${file.name}`}
						>
							<XIcon className="h-3.5 w-3.5" />
						</button>
					</div>
				);
			})}
		</div>
	);
};

export const AgentChatInput: FC<AgentChatInputProps> = ({
	onSend,
	placeholder = "Type a message...",
	isDisabled,
	isLoading,
	inputRef,
	initialValue,
	onContentChange,
	selectedModel,
	onModelChange,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	inputStatusText,
	modelCatalogStatusMessage,
	isStreaming = false,
	onInterrupt,
	isInterruptPending = false,
	leftActions,
	queuedMessages = [],
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	editingQueuedMessageID = null,
	onStartQueueEdit,
	onCancelQueueEdit,
	isEditingHistoryMessage = false,
	onCancelHistoryEdit,
	contextUsage,
	attachments = [],
	onAttach,
	onRemoveAttachment,
	uploadStates,
	previewUrls,
	textContents,
	onTextPreview,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
}) => {
	const internalRef = useRef<ChatMessageInputRef>(null);
	const [previewImage, setPreviewImage] = useState<string | null>(null);
	const [previewText, setPreviewText] = useState<string | null>(null);
	const [previewTextFileName, setPreviewTextFileName] = useState<string | null>(
		null,
	);

	const [hasFileReferences, setHasFileReferences] = useState(false);

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
		if (combined) {
			editor.insertText(combined);
		}
	}, [speech.transcript, speech.isRecording, preRecordingValue]);

	// Forward the internal ref to the parent-supplied inputRef
	// so both point to the same ChatMessageInputRef instance.
	useImperativeHandle(inputRef, () => internalRef.current!, []);

	const fileInputRef = useRef<HTMLInputElement>(null);

	const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (e.target.files && onAttach) {
			onAttach(Array.from(e.target.files));
		}
		// Reset so the same file can be selected again.
		e.target.value = "";
	};

	const handleFilePaste = (file: File) => {
		onAttach?.([file]);
	};

	const handleInlineText = (file: File, nextContent?: string) => {
		const content = nextContent ?? textContents?.get(file);
		if (content === undefined) return;
		const editor = internalRef.current;
		if (!editor) return;
		editor.insertText(content);
		onRemoveAttachment?.(file);
	};

	const handleTextPreview = (content: string, fileName: string) => {
		if (onTextPreview) {
			onTextPreview(content, fileName);
		} else {
			setPreviewText(content);
			setPreviewTextFileName(fileName);
		}
	};

	// Drag-and-drop support for image files.
	const [isDragging, setIsDragging] = useState(false);

	const handleDragOver = (e: React.DragEvent) => {
		e.preventDefault();
		if (e.dataTransfer.types.includes("Files")) {
			setIsDragging(true);
		}
	};

	const handleDragLeave = (e: React.DragEvent) => {
		if (!e.currentTarget.contains(e.relatedTarget as Node)) {
			setIsDragging(false);
		}
	};

	const handleDrop = (e: React.DragEvent) => {
		e.preventDefault();
		setIsDragging(false);
		if (!onAttach || !e.dataTransfer.files.length) return;
		const images = Array.from(e.dataTransfer.files).filter((f) =>
			f.type.startsWith("image/"),
		);
		if (images.length > 0) {
			onAttach(images);
		}
	};

	// Track whether the editor has content so we can gate the
	// send button without a controlled value prop.
	const [hasContent, setHasContent] = useState(() =>
		Boolean(initialValue?.trim()),
	);

	const handleContentChange = (content: string, hasRefs: boolean) => {
		setHasContent(Boolean(content.trim()));
		setHasFileReferences(hasRefs);
		onContentChange?.(content);
	};

	// Re-focus the editor after a send completes (isLoading goes
	// from true → false) so the user can immediately type again.
	const prevIsLoadingRef = useRef(isLoading);
	useEffect(() => {
		const wasLoading = prevIsLoadingRef.current;
		prevIsLoadingRef.current = isLoading;
		if (wasLoading && !isLoading && !isMobileViewport()) {
			internalRef.current?.focus();
		}
	}, [isLoading]);
	const isUploading = attachments.some(
		(f) => uploadStates?.get(f)?.status === "uploading",
	);
	const hasUploadedAttachments = attachments.some(
		(f) => uploadStates?.get(f)?.status === "uploaded",
	);
	const canSend =
		!isDisabled &&
		!isLoading &&
		hasModelOptions &&
		(hasContent || hasUploadedAttachments || hasFileReferences) &&
		!isUploading;
	const handleSubmit = () => {
		const text = internalRef.current?.getValue()?.trim() ?? "";

		// If the input is empty and there are queued messages,
		// promote the first one instead of submitting.
		if (
			!text &&
			!hasUploadedAttachments &&
			!hasFileReferences &&
			!isDisabled &&
			!isLoading &&
			!isUploading &&
			queuedMessages.length > 0 &&
			onPromoteQueuedMessage
		) {
			void onPromoteQueuedMessage(queuedMessages[0].id);
			return;
		}

		if (
			(!text && !hasUploadedAttachments && !hasFileReferences) ||
			isDisabled ||
			isLoading ||
			isUploading ||
			!hasModelOptions
		) {
			return;
		}

		onSend(text);
		if (!isMobileViewport()) {
			internalRef.current?.focus();
		}
	};
	const handleStartRecording = () => {
		setPreRecordingValue(internalRef.current?.getValue()?.trim() ?? "");
		speech.start();
	};

	const handleAcceptRecording = () => {
		speech.stop();
	};

	const handleCancelRecording = () => {
		const original = preRecordingValue;
		speech.cancel();
		const editor = internalRef.current;
		if (editor) {
			editor.clear();
			if (original) {
				editor.insertText(original);
			}
		}
		setPreRecordingValue("");
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Escape") {
			if (editingQueuedMessageID !== null) {
				e.preventDefault();
				onCancelQueueEdit?.();
			} else if (isEditingHistoryMessage) {
				e.preventDefault();
				onCancelHistoryEdit?.();
			} else if (isStreaming && onInterrupt && !isInterruptPending) {
				e.preventDefault();
				onInterrupt();
			}
		}
	};

	const sendButtonLabel =
		editingQueuedMessageID !== null
			? "Save"
			: isEditingHistoryMessage
				? "Save Edit"
				: "Send";

	const content = (
		<div
			className={cn(
				"mx-auto w-full max-w-3xl pb-0 sm:pb-4",
				isEditingHistoryMessage && "pt-1",
			)}
		>
			{queuedMessages.length > 0 && (
				<QueuedMessagesList
					messages={queuedMessages}
					onDelete={(id) => {
						if (id === editingQueuedMessageID) {
							onCancelQueueEdit?.();
						}
						void onDeleteQueuedMessage?.(id);
					}}
					onPromote={(id) => {
						if (id === editingQueuedMessageID) {
							onCancelQueueEdit?.();
						}
						void onPromoteQueuedMessage?.(id);
					}}
					onEdit={onStartQueueEdit}
					editingMessageID={editingQueuedMessageID}
					className="mb-2"
				/>
			)}
			<div
				className={cn(
					"rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40",
					isDragging && "ring-2 ring-content-link/40",
					isEditingHistoryMessage &&
						"shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				)}
				onKeyDown={handleKeyDown}
				onDragOver={onAttach ? handleDragOver : undefined}
				onDragLeave={onAttach ? handleDragLeave : undefined}
				onDrop={onAttach ? handleDrop : undefined}
			>
				{editingQueuedMessageID !== null && (
					<div className="flex items-center justify-between border-b border-border-default/70 bg-surface-primary/25 px-3 py-1.5">
						<span className="text-sm text-content-secondary">
							Editing queued message
						</span>
						<Button
							type="button"
							variant="subtle"
							size="sm"
							onClick={onCancelQueueEdit}
							className="h-7 px-2 text-content-secondary hover:text-content-primary"
						>
							Cancel
						</Button>
					</div>
				)}
				{isEditingHistoryMessage && editingQueuedMessageID === null && (
					<div className="flex items-center justify-between border-b border-border-warning/50 px-3 py-1.5">
						<span className="flex items-center gap-1.5 text-xs font-medium text-content-warning">
							<PencilIcon className="h-3.5 w-3.5" />
							{isLoading
								? "Saving edit..."
								: "Editing will delete all subsequent messages and restart the conversation here."}
						</span>
						<Button
							type="button"
							variant="subtle"
							size="icon"
							aria-label="Cancel editing"
							onClick={onCancelHistoryEdit}
							disabled={isLoading}
							className="size-6 rounded text-content-warning hover:text-content-primary"
						>
							<XIcon className="h-3.5 w-3.5" />
						</Button>
					</div>
				)}
				{onRemoveAttachment && (
					<AttachmentPreview
						attachments={attachments}
						onRemove={onRemoveAttachment}
						uploadStates={uploadStates}
						previewUrls={previewUrls}
						onPreview={setPreviewImage}
						textContents={textContents}
						onTextPreview={handleTextPreview}
						onInlineText={handleInlineText}
					/>
				)}
				<ChatMessageInput
					ref={internalRef}
					onFilePaste={onAttach ? handleFilePaste : undefined}
					aria-label="Chat message"
					className="min-h-[60px] sm:min-h-24 w-full resize-none bg-transparent px-3 py-2 font-sans text-[15px] leading-6 text-content-primary placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
					placeholder={placeholder}
					initialValue={initialValue}
					onChange={handleContentChange}
					onEnter={handleSubmit}
					disabled={isDisabled || isLoading}
					autoFocus
				/>

				<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
					<div className="flex min-w-0 items-center gap-2">
						<ModelSelector
							value={selectedModel}
							onValueChange={onModelChange}
							options={modelOptions}
							disabled={isDisabled}
							placeholder={modelSelectorPlaceholder}
							formatProviderLabel={formatProviderLabel}
							dropdownSide="top"
							dropdownAlign="center"
						/>
						{mcpServers &&
							mcpServers.length > 0 &&
							onMCPSelectionChange &&
							onMCPAuthComplete && (
								<MCPServerPicker
									servers={mcpServers}
									selectedServerIds={selectedMCPServerIds ?? []}
									onSelectionChange={onMCPSelectionChange}
									onAuthComplete={onMCPAuthComplete}
									disabled={isDisabled}
								/>
							)}
						{leftActions}
						{inputStatusText && (
							<span className="hidden text-xs text-content-secondary sm:inline">
								{inputStatusText}
							</span>
						)}
					</div>
					<div className="flex items-center gap-2">
						{onAttach && (
							<>
								<input
									ref={fileInputRef}
									type="file"
									multiple
									accept="image/*"
									onChange={handleFileSelect}
									className="hidden"
								/>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-7 shrink-0 rounded-full [&>svg]:!size-icon-sm [&>svg]:p-0"
									onClick={() => fileInputRef.current?.click()}
									disabled={isDisabled}
									aria-label="Attach files"
								>
									<ImageIcon />
								</Button>
							</>
						)}
						{speech.isSupported && !isStreaming && (
							<>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-7 shrink-0 rounded-full [&>svg]:!size-icon-sm [&>svg]:p-0"
									onClick={
										speech.isRecording
											? handleCancelRecording
											: handleStartRecording
									}
									disabled={isDisabled}
									aria-label={
										speech.isRecording ? "Cancel voice input" : "Voice input"
									}
								>
									{speech.isRecording ? <XIcon /> : <MicIcon />}
								</Button>
								{speech.error && !speech.isRecording && (
									<span
										className="text-2xs text-content-destructive"
										role="alert"
									>
										{speech.error === "not-allowed"
											? "Mic access denied"
											: "Voice input failed"}
									</span>
								)}
							</>
						)}
						{contextUsage !== undefined && (
							<ContextUsageIndicator usage={contextUsage} />
						)}
						{isStreaming && onInterrupt && (
							<Button
								size="icon"
								variant="default"
								className="size-7 rounded-full transition-colors [&>svg]:!size-3 [&>svg]:p-0"
								onClick={onInterrupt}
								disabled={isInterruptPending}
							>
								<Square className="fill-current" />
								<span className="sr-only">Stop</span>
							</Button>
						)}
						{!(isStreaming && editingQueuedMessageID === null) && (
							<Button
								size="icon"
								variant="default"
								className="size-7 rounded-full transition-colors [&>svg]:!size-5 [&>svg]:p-0"
								onClick={
									speech.isRecording ? handleAcceptRecording : handleSubmit
								}
								disabled={speech.isRecording ? false : !canSend}
							>
								{isLoading ? (
									<Spinner size="sm" loading aria-hidden="true" />
								) : speech.isRecording ? (
									<CheckIcon />
								) : (
									<ArrowUpIcon />
								)}
								<span className="sr-only">
									{speech.isRecording ? "Accept voice input" : sendButtonLabel}
								</span>
							</Button>
						)}
					</div>
				</div>
				{inputStatusText && (
					<div className="px-2.5 pb-1 text-xs text-content-secondary sm:hidden">
						{inputStatusText}
					</div>
				)}
				{modelCatalogStatusMessage && (
					<div className="px-2.5 pb-1 text-2xs text-content-secondary">
						{modelCatalogStatusMessage}
					</div>
				)}
			</div>
		</div>
	);

	return (
		<>
			{content}
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
					onClose={() => {
						setPreviewText(null);
						setPreviewTextFileName(null);
					}}
				/>
			)}
		</>
	);
};
