import {
	ArrowLeftIcon,
	ArrowUpIcon,
	CheckIcon,
	ChevronRightIcon,
	MicIcon,
	MonitorIcon,
	PaperclipIcon,
	PencilIcon,
	PlusIcon,
	ServerIcon,
	SquareIcon,
	XIcon,
} from "lucide-react";
import type React from "react";
import {
	type FC,
	useEffect,
	useImperativeHandle,
	useRef,
	useState,
} from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatMessagePart, ChatQueuedMessage } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { isBelowMdViewport, isMobileViewport } from "#/utils/mobile";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import { useOverflowCount } from "../hooks/useOverflowCount";
import { useSpeechRecognition } from "../hooks/useSpeechRecognition";
import {
	chatAttachmentAcceptAttribute,
	isChatAttachmentFile,
} from "../utils/chatAttachments";
import { formatProviderLabel } from "../utils/modelOptions";
import {
	AttachmentPreview,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
import { ModelSelector, type ModelSelectorOption } from "./ChatElements";
import {
	ChatMessageInput,
	type ChatMessageInputRef,
} from "./ChatMessageInput/ChatMessageInput";
import type { AgentContextUsage } from "./ContextUsageIndicator";
import { ContextUsageIndicator } from "./ContextUsageIndicator";
import { ImageLightbox } from "./ImageLightbox";
import { QueuedMessagesList } from "./QueuedMessagesList";
import { TextPreviewDialog } from "./TextPreviewDialog";
import { WorkspacePill } from "./WorkspacePill";

export {
	ImageThumbnail,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
export type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";
export type { AgentContextUsage } from "./ContextUsageIndicator";

interface AgentChatInputProps {
	onSend: (message: string) => void;
	placeholder?: string;
	isDisabled: boolean;
	isLoading: boolean;
	// Ref for the Lexical editor, exposed for imperative access.
	inputRef?: React.Ref<ChatMessageInputRef>;
	// Initial text to seed the editor on first mount only.
	initialValue?: string;
	// Serialized Lexical editor state for restoring drafts with
	// file-reference chips. Takes precedence over initialValue.
	initialEditorState?: string;
	// Monotonic counter to force editor remount.
	remountKey?: number;
	// Called on every content change inside the editor.
	onContentChange?: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
	// Model selector.
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	planModeEnabled?: boolean;
	onPlanModeToggle?: (enabled: boolean) => void;
	isModelCatalogLoading?: boolean;
	// Streaming controls (optional, for the detail page).
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	// Workspace picker.
	workspaceOptions?: ReadonlyArray<{
		id: string;
		name: string;
		owner_name: string;
	}>;
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (id: string | null) => void;
	isWorkspaceLoading?: boolean;
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
	onEditLastUserMessage?: () => void;

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
	onTextPreview?: (
		content: string,
		fileName: string,
		mediaType?: string,
	) => void;
	// MCP Server picker.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	chatId?: string;
	sshCommand?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	folder?: string;
}

export interface AttachedWorkspaceInfo {
	id: string;
	name: string;
	route: string;
	statusIcon: React.ReactNode;
	statusLabel: string;
}
type ToolBadgeData =
	| { kind: "workspace"; name: string }
	| ({ kind: "attached-workspace" } & AttachedWorkspaceInfo)
	| { kind: "mcp"; server: TypesGen.MCPServerConfig };

// Small `X` button rendered inside pill-style badges (attached
// workspace, MCP server, planning indicator) to dismiss or disable
// the badge without opening the `+` menu. Callers pass the action
// handler and a descriptive aria-label.
const BadgeDismissButton: FC<{
	onClick: () => void;
	ariaLabel: string;
	isDisabled?: boolean;
}> = ({ onClick, ariaLabel, isDisabled = false }) => (
	<button
		type="button"
		onClick={onClick}
		disabled={isDisabled}
		className="ml-0.5 inline-flex cursor-pointer items-center justify-center rounded-full border-0 bg-transparent p-0.5 text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent disabled:hover:text-content-secondary"
		aria-label={ariaLabel}
	>
		<XIcon className="!size-2.5" />
	</button>
);

const ToolBadge: FC<{
	badge: ToolBadgeData;
	onRemoveWorkspace?: () => void;
	onRemoveMcp?: (serverId: string) => void;
	className?: string;
}> = ({ badge, onRemoveWorkspace, onRemoveMcp, className }) => {
	const badgeCls = cn(
		"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
		className,
	);

	if (badge.kind === "attached-workspace") {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Link
						to={badge.route}
						target="_blank"
						rel="noreferrer"
						className={cn(
							badgeCls,
							"no-underline transition-colors hover:bg-surface-tertiary hover:text-content-primary",
						)}
					>
						{badge.statusIcon}
						<span className="truncate">{badge.name}</span>
					</Link>
				</TooltipTrigger>
				<TooltipContent>{badge.statusLabel}</TooltipContent>
			</Tooltip>
		);
	}

	if (badge.kind === "workspace") {
		return (
			<span className={badgeCls}>
				<MonitorIcon className="size-3" />
				<span className="truncate">{badge.name}</span>
				{onRemoveWorkspace && (
					<BadgeDismissButton
						onClick={onRemoveWorkspace}
						ariaLabel={`Remove workspace ${badge.name}`}
					/>
				)}
			</span>
		);
	}

	const isForceOn = badge.server.availability === "force_on";
	return (
		<span className={badgeCls}>
			{badge.server.icon_url ? (
				<ExternalImage
					src={badge.server.icon_url}
					alt=""
					className="size-3 rounded-sm"
				/>
			) : (
				<ServerIcon className="size-3" />
			)}
			{badge.server.display_name}
			{!isForceOn && onRemoveMcp && (
				<BadgeDismissButton
					onClick={() => onRemoveMcp(badge.server.id)}
					ariaLabel={`Remove ${badge.server.display_name}`}
				/>
			)}
		</span>
	);
};

export const AgentChatInput: FC<AgentChatInputProps> = ({
	onSend,
	placeholder = "Type a message...",
	isDisabled,
	isLoading,
	inputRef,
	initialValue,
	initialEditorState,
	remountKey,
	onContentChange,
	selectedModel,
	onModelChange,
	modelOptions,
	modelSelectorPlaceholder,
	hasModelOptions,
	planModeEnabled = false,
	onPlanModeToggle,
	isModelCatalogLoading = false,
	isStreaming = false,
	onInterrupt,
	isInterruptPending = false,
	workspaceOptions,
	selectedWorkspaceId,
	onWorkspaceChange,
	isWorkspaceLoading,
	queuedMessages = [],
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	editingQueuedMessageID = null,
	onStartQueueEdit,
	onCancelQueueEdit,
	isEditingHistoryMessage = false,
	onCancelHistoryEdit,
	onEditLastUserMessage,
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
	workspace,
	workspaceAgent,
	chatId,
	sshCommand,
	attachedWorkspace,
	folder,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const internalRef = useRef<ChatMessageInputRef>(null);
	const [previewImage, setPreviewImage] = useState<string | null>(null);
	const [previewText, setPreviewText] = useState<string | null>(null);
	const [previewTextFileName, setPreviewTextFileName] = useState<string | null>(
		null,
	);
	const [previewTextMediaType, setPreviewTextMediaType] = useState<
		string | null
	>(null);
	const [plusMenuOpen, setPlusMenuOpen] = useState(false);
	const [plusMenuView, setPlusMenuView] = useState<"main" | "workspace">(
		"main",
	);
	const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false);
	const [mcpConnectingId, setMcpConnectingId] = useState<string | null>(null);
	const mcpPopupRef = useRef<Window | null>(null);

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

	// Listen for OAuth2 completion postMessage from popup.
	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (event.origin !== location.origin) return;
			if (
				event.data?.type === "mcp-oauth2-complete" &&
				typeof event.data.serverID === "string"
			) {
				setMcpConnectingId(null);
				onMCPAuthComplete?.(event.data.serverID);
				mcpPopupRef.current = null;
			}
		};
		window.addEventListener("message", handler);
		return () => window.removeEventListener("message", handler);
	}, [onMCPAuthComplete]);

	// Poll for popup close and clean up on unmount.
	useEffect(() => {
		if (!mcpConnectingId || !mcpPopupRef.current) return;
		const interval = setInterval(() => {
			if (mcpPopupRef.current?.closed) {
				setMcpConnectingId(null);
				mcpPopupRef.current = null;
			}
		}, 500);
		return () => {
			clearInterval(interval);
			if (mcpPopupRef.current && !mcpPopupRef.current.closed) {
				mcpPopupRef.current.close();
				mcpPopupRef.current = null;
			}
		};
	}, [mcpConnectingId]);

	const handleMcpToggle = (serverId: string, checked: boolean) => {
		if (!onMCPSelectionChange || !selectedMCPServerIds) return;
		if (checked) {
			onMCPSelectionChange([...selectedMCPServerIds, serverId]);
		} else {
			onMCPSelectionChange(
				selectedMCPServerIds.filter((id) => id !== serverId),
			);
		}
	};

	const handleMcpConnect = (server: TypesGen.MCPServerConfig) => {
		setMcpConnectingId(server.id);
		const connectUrl = `/api/experimental/mcp/servers/${encodeURIComponent(server.id)}/oauth2/connect`;
		mcpPopupRef.current = window.open(
			connectUrl,
			"_blank",
			"width=900,height=600",
		);
	};

	const selectedWorkspace = workspaceOptions?.find(
		(ws) => ws.id === selectedWorkspaceId,
	);

	const shouldShowSelectedWorkspaceBadge = selectedWorkspace
		? Boolean(onWorkspaceChange) &&
			selectedWorkspace.id !== attachedWorkspace?.id
		: false;

	const enabledMcpServers = mcpServers?.filter((s) => s.enabled) ?? [];
	const activeMcpServers = enabledMcpServers.filter(
		(s) =>
			(s.availability === "force_on" || selectedMCPServerIds?.includes(s.id)) &&
			!(s.auth_type === "oauth2" && !s.auth_connected),
	);

	const badgeContainerRef = useRef<HTMLDivElement>(null);

	const [overflowPopoverOpen, setOverflowPopoverOpen] = useState(false);

	// Ordered list of active tool badge data so we can determine
	// which ones ended up in the overflow popover.
	const allBadges: ToolBadgeData[] = [];
	// When workspace data is available, WorkspacePill handles
	// the display (including app dropdown). Otherwise fall back
	// to the simple attached-workspace ToolBadge.
	if (!(workspace && workspaceAgent && chatId) && attachedWorkspace) {
		allBadges.push({ kind: "attached-workspace", ...attachedWorkspace });
	}
	if (shouldShowSelectedWorkspaceBadge && selectedWorkspace) {
		allBadges.push({ kind: "workspace", name: selectedWorkspace.name });
	}
	for (const s of activeMcpServers) {
		allBadges.push({ kind: "mcp", server: s });
	}

	const overflowCount = useOverflowCount(badgeContainerRef, allBadges.length);
	const visibleCount = Math.max(0, allBadges.length - overflowCount);
	const overflowBadges = allBadges.slice(visibleCount);

	const handleRemoveWorkspace = () => onWorkspaceChange?.(null);
	const handleRemoveMcp = (serverId: string) =>
		handleMcpToggle(serverId, false);

	const handlePlanModeToggle = () => {
		onPlanModeToggle?.(!planModeEnabled);
		setPlusMenuOpen(false);
	};

	const handleDisablePlanMode = () => onPlanModeToggle?.(false);

	const fileInputRef = useRef<HTMLInputElement>(null);
	const [composerElement, setComposerElement] = useState<HTMLDivElement | null>(
		null,
	);
	useEffect(() => {
		if (!composerElement) return;
		const update = () => {
			const rect = composerElement.getBoundingClientRect();
			const bottom = Math.max(0, window.innerHeight - rect.bottom);
			document.documentElement.style.setProperty(
				"--mobile-dropdown-bottom",
				`${bottom}px`,
			);
		};
		update();
		const ro = new ResizeObserver(update);
		ro.observe(composerElement);
		window.addEventListener("resize", update);
		const viewport = window.visualViewport;
		viewport?.addEventListener("resize", update);
		viewport?.addEventListener("scroll", update);
		return () => {
			ro.disconnect();
			window.removeEventListener("resize", update);
			viewport?.removeEventListener("resize", update);
			viewport?.removeEventListener("scroll", update);
			document.documentElement.style.removeProperty("--mobile-dropdown-bottom");
		};
	}, [composerElement]);

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

	const handleTextPreview = (
		content: string,
		fileName: string,
		mediaType?: string,
	) => {
		if (onTextPreview) {
			onTextPreview(content, fileName, mediaType);
		} else {
			setPreviewText(content);
			setPreviewTextFileName(fileName);
			setPreviewTextMediaType(mediaType ?? null);
		}
	};

	// Drag-and-drop support for any chat-supported file type.
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
		const attachable = Array.from(e.dataTransfer.files).filter(
			isChatAttachmentFile,
		);
		if (attachable.length > 0) {
			onAttach(attachable);
		}
	};

	// Track whether the editor has content so we can gate the
	// send button without a controlled value prop.
	const [hasContent, setHasContent] = useState(() =>
		Boolean(initialValue?.trim()),
	);

	const [invisibleCharCount, setInvisibleCharCount] = useState(() =>
		countInvisibleCharacters(initialValue ?? ""),
	);

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasRefs: boolean,
	) => {
		setHasContent(Boolean(content.trim()));
		setHasFileReferences(hasRefs);
		setInvisibleCharCount(countInvisibleCharacters(content));
		onContentChange?.(content, serializedEditorState, hasRefs);
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
	const hasActiveUploads = attachments.some((file) =>
		isUploadInProgress(uploadStates?.get(file)),
	);
	const hasUploadedAttachments = attachments.some(
		(f) => uploadStates?.get(f)?.status === "uploaded",
	);
	const hasDraftContext =
		hasContent || attachments.length > 0 || hasFileReferences;
	const isComposerEffectivelyEmpty = !hasDraftContext;
	const hasSendableContent =
		hasContent || hasUploadedAttachments || hasFileReferences;
	const canSend =
		!isDisabled &&
		!isLoading &&
		hasModelOptions &&
		hasSendableContent &&
		!hasActiveUploads;
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
			isLoading ||
			hasActiveUploads ||
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

	const handleComposerKeyDown = (e: React.KeyboardEvent) => {
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
	const handleEditorKeyDown = (e: React.KeyboardEvent) => {
		if (
			e.key !== "ArrowUp" ||
			editingQueuedMessageID !== null ||
			isEditingHistoryMessage ||
			!onEditLastUserMessage ||
			!isComposerEffectivelyEmpty
		) {
			return;
		}
		e.preventDefault();
		onEditLastUserMessage();
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
				"mx-auto w-full pb-0 sm:pb-4",
				chatWidthClass(chatFullWidth),
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
				ref={setComposerElement}
				data-testid="chat-composer"
				className={cn(
					"rounded-2xl border border-border-default/80 bg-surface-secondary sm:bg-surface-secondary/45 p-1 shadow-sm has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40",
					isDragging && "ring-2 ring-content-link/40",
					isEditingHistoryMessage &&
						"shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				)}
				onKeyDown={handleComposerKeyDown}
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
							Editing will delete all subsequent messages and restart the
							conversation here.
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
					className="min-h-[60px] sm:min-h-24 w-full resize-none bg-transparent px-3 py-2 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
					placeholder={placeholder}
					initialValue={initialValue}
					initialEditorState={initialEditorState}
					remountKey={remountKey}
					onChange={handleContentChange}
					onKeyDown={handleEditorKeyDown}
					onEnter={handleSubmit}
					disabled={isDisabled || isLoading}
					autoFocus
				/>
				{/* Warn about invisible Unicode in the message text.
				 * Unlike the admin/user prompt textareas (which strip
				 * invisible chars server-side on save), the chat input
				 * is the user's free-form message — we don't silently
				 * mutate it. Instead we surface a warning so the user
				 * can make an informed decision. This guards against
				 * social engineering attacks where a user is tricked
				 * into pasting a "prompt" containing hidden LLM
				 * instructions encoded as zero-width characters. */}
				{invisibleCharCount > 0 && (
					<div className="px-3 pb-1">
						<Alert severity="warning">
							<AlertDescription>
								This message contains {invisibleCharCount} invisible Unicode
								character{invisibleCharCount !== 1 ? "s" : ""} that could hide
								content. Review carefully before sending.
							</AlertDescription>
						</Alert>
					</div>
				)}
				{/* Hidden file input for attaching any server-accepted file type. */}
				{onAttach && (
					<input
						ref={fileInputRef}
						type="file"
						multiple
						accept={chatAttachmentAcceptAttribute}
						onChange={handleFileSelect}
						className="hidden"
					/>
				)}
				<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
					<div className="flex min-w-0 items-center gap-1">
						{/* Plus menu */}
						<Popover
							modal={false}
							open={plusMenuOpen}
							onOpenChange={(open) => {
								setPlusMenuOpen(open);
								if (!open) setPlusMenuView("main");
							}}
						>
							{" "}
							<PopoverTrigger asChild>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-7 shrink-0 rounded-full [&>svg]:!size-icon-sm [&>svg]:p-0"
									disabled={isDisabled}
									aria-label="More options"
								>
									<PlusIcon />
								</Button>
							</PopoverTrigger>
							<PopoverContent
								side="bottom"
								align="start"
								className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto min-w-[200px] p-1"
							>
								{plusMenuView === "workspace" ? (
									<div className="p-0">
										<button
											type="button"
											onClick={() => setPlusMenuView("main")}
											className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
										>
											<ArrowLeftIcon className="size-3.5 shrink-0" />
											<span>Back</span>
										</button>
										<Separator className="my-1" />
										<Command loop>
											<CommandInput
												placeholder="Search workspaces..."
												className="text-xs"
											/>
											<CommandList>
												<CommandEmpty className="text-xs">
													No workspaces found
												</CommandEmpty>
												<CommandGroup>
													{workspaceOptions?.map((workspace) => (
														<CommandItem
															className="text-xs font-normal"
															key={workspace.id}
															value={workspace.name}
															onSelect={() => {
																onWorkspaceChange?.(workspace.id);
																setPlusMenuOpen(false);
															}}
														>
															{workspace.name}
															{selectedWorkspaceId === workspace.id && (
																<CheckIcon className="ml-auto size-icon-sm shrink-0" />
															)}
														</CommandItem>
													))}
												</CommandGroup>
											</CommandList>
										</Command>
									</div>
								) : (
									<>
										{onAttach && (
											<button
												type="button"
												onClick={() => {
													setPlusMenuOpen(false);
													fileInputRef.current?.click();
												}}
												className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
											>
												<PaperclipIcon className="size-3.5 shrink-0" />
												Attach file
											</button>
										)}
										{onPlanModeToggle && (
											<button
												type="button"
												role="menuitemcheckbox"
												aria-checked={planModeEnabled}
												onClick={handlePlanModeToggle}
												disabled={isDisabled}
												className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
											>
												<PencilIcon className="size-3.5 shrink-0" />
												<span>Plan first</span>
												{planModeEnabled && (
													<CheckIcon className="ml-auto size-icon-sm shrink-0" />
												)}
											</button>
										)}
										{workspaceOptions &&
											onWorkspaceChange &&
											(isBelowMdViewport() ? (
												<button
													type="button"
													disabled={isDisabled || isWorkspaceLoading}
													onClick={() => setPlusMenuView("workspace")}
													className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
												>
													<MonitorIcon className="size-3.5 shrink-0" />
													<span>Attach workspace</span>
													<ChevronRightIcon className="ml-auto size-icon-sm" />
												</button>
											) : (
												<Popover
													open={workspacePickerOpen}
													onOpenChange={setWorkspacePickerOpen}
												>
													<PopoverTrigger asChild>
														<button
															type="button"
															disabled={isDisabled || isWorkspaceLoading}
															className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
														>
															<MonitorIcon className="size-3.5 shrink-0" />
															<span>Attach workspace</span>
															<ChevronRightIcon
																className={cn(
																	"ml-auto size-icon-sm transition-transform",
																	workspacePickerOpen && "rotate-180",
																)}
															/>
														</button>
													</PopoverTrigger>
													<PopoverContent
														side="right"
														align="start"
														sideOffset={8}
														className="w-64 p-0"
													>
														<Command loop>
															<CommandInput
																placeholder="Search workspaces..."
																className="text-xs"
															/>
															<CommandList>
																<CommandEmpty className="text-xs">
																	No workspaces found
																</CommandEmpty>
																<CommandGroup>
																	{workspaceOptions.map((workspace) => (
																		<CommandItem
																			className="text-xs font-normal"
																			key={workspace.id}
																			value={workspace.name}
																			onSelect={() => {
																				onWorkspaceChange(workspace.id);
																				setWorkspacePickerOpen(false);
																				setPlusMenuOpen(false);
																			}}
																		>
																			{workspace.name}
																			{selectedWorkspaceId === workspace.id && (
																				<CheckIcon className="ml-auto size-icon-sm shrink-0" />
																			)}
																		</CommandItem>
																	))}
																</CommandGroup>
															</CommandList>
														</Command>
													</PopoverContent>
												</Popover>
											))}
										{enabledMcpServers.length > 0 && (
											<>
												<Separator className="my-1" />
												{enabledMcpServers.map((server) => {
													const isForceOn = server.availability === "force_on";
													const isSelected =
														isForceOn ||
														(selectedMCPServerIds?.includes(server.id) ??
															false);
													const needsAuth =
														server.auth_type === "oauth2" &&
														!server.auth_connected;
													const isConnecting = mcpConnectingId === server.id;
													return (
														<div
															key={server.id}
															className="flex items-center gap-1.5 px-1 py-1.5"
														>
															{server.icon_url ? (
																<ExternalImage
																	src={server.icon_url}
																	alt=""
																	className="size-3.5 shrink-0 rounded-sm"
																/>
															) : (
																<ServerIcon className="size-3.5 shrink-0 text-content-secondary" />
															)}
															<span className="min-w-0 flex-1 truncate text-xs text-content-secondary">
																{server.display_name}
															</span>
															{needsAuth ? (
																<Button
																	variant="outline"
																	size="sm"
																	className="h-6 shrink-0 px-2 text-[10px] leading-none"
																	onClick={() => handleMcpConnect(server)}
																	disabled={
																		isDisabled || mcpConnectingId !== null
																	}
																>
																	{isConnecting ? (
																		<Spinner loading className="size-2.5" />
																	) : null}
																	Auth
																</Button>
															) : (
																<Switch
																	size="sm"
																	checked={isSelected}
																	onCheckedChange={(checked) =>
																		handleMcpToggle(server.id, checked)
																	}
																	disabled={isDisabled || isForceOn}
																	aria-label={`${isSelected ? "Disable" : "Enable"} ${server.display_name}`}
																/>
															)}
														</div>
													);
												})}
											</>
										)}
									</>
								)}
							</PopoverContent>
						</Popover>
						{isModelCatalogLoading ? (
							<Skeleton className="h-6 w-24 rounded" />
						) : (
							<ModelSelector
								value={selectedModel}
								onValueChange={onModelChange}
								options={modelOptions}
								disabled={isDisabled}
								placeholder={modelSelectorPlaceholder}
								formatProviderLabel={formatProviderLabel}
								dropdownSide="top"
								dropdownAlign="center"
								enableMobileFullWidthDropdown
							/>
						)}
						{planModeEnabled && (
							<span className="hidden shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary sm:inline-flex">
								<PencilIcon className="size-3" />
								Planning
								{onPlanModeToggle && (
									<BadgeDismissButton
										onClick={handleDisablePlanMode}
										ariaLabel="Disable plan mode"
										isDisabled={isDisabled}
									/>
								)}
							</span>
						)}{" "}
						{/* Badge row — all badges and the pill always
						 * render so the DOM structure never changes.
						 * Overflow badges use invisible + order-1 to
						 * hide and reorder via CSS. The pill is invisible
						 * when there's no overflow but still occupies
						 * layout space, preventing measurement flicker. */}
						{workspace && workspaceAgent && chatId && (
							<span className="ml-1 sm:ml-0">
								<WorkspacePill
									workspace={workspace}
									agent={workspaceAgent}
									chatId={chatId}
									sshCommand={sshCommand}
									folder={folder}
								/>
							</span>
						)}
						<div
							ref={badgeContainerRef}
							className="flex min-w-0 items-center gap-1 overflow-hidden"
						>
							{allBadges.map((badge, i) => {
								const isOverflow = overflowCount > 0 && i >= visibleCount;
								return (
									<ToolBadge
										key={badge.kind === "mcp" ? badge.server.id : badge.kind}
										badge={badge}
										onRemoveWorkspace={handleRemoveWorkspace}
										onRemoveMcp={handleRemoveMcp}
										className={isOverflow ? "invisible order-1" : undefined}
									/>
								);
							})}
							{/* Pill — always in the DOM so it permanently
							 * reserves layout space. Invisible when nothing
							 * overflows. CSS order keeps it before order-1
							 * (overflow) badges. */}
							<Popover
								open={overflowPopoverOpen && overflowCount > 0}
								onOpenChange={setOverflowPopoverOpen}
							>
								<PopoverTrigger asChild>
									<button
										type="button"
										className={cn(
											"inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full border-0 bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary",
											overflowCount === 0 && "invisible",
										)}
										aria-label={`${overflowCount} more item${overflowCount !== 1 ? "s" : ""}`}
										aria-hidden={overflowCount === 0}
									>
										+{overflowCount}
									</button>
								</PopoverTrigger>
								<PopoverContent
									side="top"
									align="start"
									className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom flex w-auto max-w-64 flex-wrap gap-1 p-2"
								>
									{overflowBadges.map((badge) => (
										<ToolBadge
											key={
												badge.kind === "mcp"
													? badge.server.id
													: `${badge.kind}-overflow`
											}
											badge={badge}
											onRemoveWorkspace={handleRemoveWorkspace}
											onRemoveMcp={handleRemoveMcp}
										/>
									))}
								</PopoverContent>
							</Popover>
						</div>
					</div>
					<div className="flex items-center gap-2">
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
								<SquareIcon className="fill-current" />
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
					mediaType={previewTextMediaType ?? undefined}
					onClose={() => {
						setPreviewText(null);
						setPreviewTextFileName(null);
						setPreviewTextMediaType(null);
					}}
				/>
			)}
		</>
	);
};
