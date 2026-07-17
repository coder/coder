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
	UnlinkIcon,
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
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { disconnectMCPServerOAuth2 } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	AgentChatSendShortcut,
	ChatMessagePart,
	ChatQueuedMessage,
} from "#/api/typesGenerated";
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
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
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
	DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
	MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
} from "../utils/agentChatSendShortcut";
import {
	chatAttachmentAcceptAttribute,
	isChatAttachmentFile,
} from "../utils/chatAttachments";
import { AgentSetupNotice } from "./AgentSetupNotice";
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
	sendShortcut?: AgentChatSendShortcut;
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
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
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
		organization_id: string;
	}>;
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (id: string | null) => void;
	// Organization ID of the current chat. When set, workspaces from
	// other organizations are shown as disabled in the picker.
	chatOrganizationId?: string;
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
	// Newest-first list of non-empty user prompts for local history cycling.
	userPromptHistory?: readonly string[];

	// Optional context-usage summary shown to the left of the send button.
	// Pass `null` to render fallback values (e.g. when limit is unknown).
	// Omit entirely to hide the indicator.
	contextUsage?: AgentContextUsage | null;
	// Re-pins the chat to the workspace's latest context snapshot,
	// surfaced by the context indicator when the pinned context has
	// drifted.
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
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
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	// AI Gateway is disabled deployment-wide, independent of provider/model
	// configuration. Forces the setup notice regardless of the counts above.
	aiGatewayDisabled?: boolean;
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
	| { kind: "mcp"; server: TypesGen.MCPServerConfig }
	| { kind: "planning" };

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
		className="group -mx-1 -my-1 inline-flex size-5 cursor-pointer items-center justify-center rounded-full border-0 bg-transparent p-0 text-content-secondary disabled:cursor-not-allowed disabled:opacity-50"
		aria-label={ariaLabel}
	>
		<span className="inline-flex size-3.5 items-center justify-center rounded-full transition-colors group-hover:bg-surface-tertiary group-hover:text-content-primary">
			<XIcon className="!size-2.5" />
		</span>
	</button>
);

const ToolBadge: FC<{
	badge: ToolBadgeData;
	onRemoveWorkspace?: () => void;
	onRemoveMcp?: (serverId: string) => void;
	onRemovePlanning?: () => void;
	isDisabled?: boolean;
	className?: string;
}> = ({
	badge,
	onRemoveWorkspace,
	onRemoveMcp,
	onRemovePlanning,
	isDisabled,
	className,
}) => {
	const badgeCls = cn(
		"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
		className,
	);

	if (badge.kind === "planning") {
		return (
			<span data-testid="planning-badge" className={badgeCls}>
				<PencilIcon className="size-3" />
				Planning
				{onRemovePlanning && (
					<BadgeDismissButton
						onClick={onRemovePlanning}
						ariaLabel="Disable plan mode"
						isDisabled={isDisabled}
					/>
				)}
			</span>
		);
	}

	if (badge.kind === "attached-workspace") {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						className={cn(
							badgeCls,
							"transition-colors hover:bg-surface-tertiary hover:text-content-primary",
						)}
					>
						<Link
							to={badge.route}
							target="_blank"
							rel="noreferrer"
							className="inline-flex min-w-0 items-center gap-1 text-inherit no-underline"
						>
							{badge.statusIcon}
							<span className="truncate">{badge.name}</span>
						</Link>
						{onRemoveWorkspace && (
							<BadgeDismissButton
								onClick={onRemoveWorkspace}
								ariaLabel={`Remove workspace ${badge.name}`}
							/>
						)}
					</span>
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
	sendShortcut = DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
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
	reasoningEffort,
	onReasoningEffortChange,
	planModeEnabled = false,
	onPlanModeToggle,
	isModelCatalogLoading = false,
	isStreaming = false,
	onInterrupt,
	isInterruptPending = false,
	workspaceOptions,
	selectedWorkspaceId,
	onWorkspaceChange,
	chatOrganizationId,
	isWorkspaceLoading,
	queuedMessages = [],
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	editingQueuedMessageID = null,
	onStartQueueEdit,
	onCancelQueueEdit,
	isEditingHistoryMessage = false,
	onCancelHistoryEdit,
	userPromptHistory = [],
	contextUsage,
	onRefreshContext,
	isRefreshingContext,
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
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames = [],
	aiGatewayDisabled,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const showAgentSetupNotice =
		aiGatewayDisabled ||
		(canConfigureAgentSetup
			? providerCount !== undefined &&
				modelCount !== undefined &&
				(providerCount === 0 || modelCount === 0)
			: modelCount !== undefined && modelCount === 0);
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
	const [mcpDisconnectTarget, setMcpDisconnectTarget] =
		useState<TypesGen.MCPServerConfig | null>(null);
	const queryClient = useQueryClient();
	const mcpDisconnectMutation = useMutation(
		disconnectMCPServerOAuth2(queryClient),
	);

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
		// Inlined resetPromptCycle body. Calling resetPromptCycle directly
		// would force it into the dep array; the React Compiler stabilises
		// callbacks but biome's react-hooks lint does not.
		setCycleIndex(null);
		setCycleSavedDraft(null);
		cycleHistorySnapshotRef.current = null;
		currentCycleValueRef.current = null;
		// Keep in sync with resetPromptCycle above.
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
		if (combined) {
			editor.insertText(combined);
		}
	}, [speech.transcript, speech.isRecording, preRecordingValue]);

	// Forward a stable delegating handle to the parent-supplied inputRef.
	// Delegates lazily to internalRef.current so methods see the current
	// Lexical instance after a remount, not the orphaned ref captured at
	// factory time.
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

	const handleMcpDisconnectConfirm = () => {
		if (!mcpDisconnectTarget) {
			return;
		}
		const name = mcpDisconnectTarget.display_name;
		mcpDisconnectMutation.mutate(mcpDisconnectTarget.id, {
			onSuccess: () => {
				setMcpDisconnectTarget(null);
				toast.success(`Disconnected ${name}.`);
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, `Failed to disconnect ${name}.`));
			},
		});
	};

	const selectedWorkspace = workspaceOptions?.find(
		(ws) => ws.id === selectedWorkspaceId,
	);
	const canUseWorkspacePicker =
		Boolean(onWorkspaceChange) && !isWorkspaceLoading;
	const linkedWorkspaceId = workspace?.id ?? attachedWorkspace?.id;

	const shouldShowSelectedWorkspaceBadge = selectedWorkspace
		? selectedWorkspace.id !== linkedWorkspaceId
		: false;

	const enabledMcpServers = mcpServers?.filter((s) => s.enabled) ?? [];
	const activeMcpServers = enabledMcpServers.filter(
		(s) =>
			(s.availability === "force_on" || selectedMCPServerIds?.includes(s.id)) &&
			!(s.auth_type === "oauth2" && !s.auth_connected),
	);

	const badgeContainerRef = useRef<HTMLDivElement>(null);

	const [overflowPopoverOpen, setOverflowPopoverOpen] = useState(false);
	const shouldOverflowPlanningBadge =
		planModeEnabled && contextUsage !== undefined;

	// Ordered list of active tool badge data so we can determine
	// which ones ended up in the overflow popover.
	const allBadges: ToolBadgeData[] = [];
	if (shouldOverflowPlanningBadge) {
		allBadges.push({ kind: "planning" });
	}
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
	const removeWorkspaceHandler = onWorkspaceChange
		? handleRemoveWorkspace
		: undefined;
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
			for (const id of animationFrameIDs) {
				cancelAnimationFrame(id);
			}
			animationFrameIDs.clear();
			for (const id of timeoutIDs) {
				clearTimeout(id);
			}
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

	const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (e.target.files && onAttach) {
			resetPromptCycle();
			onAttach(Array.from(e.target.files));
		}
		// Reset so the same file can be selected again.
		e.target.value = "";
	};

	const handleFilePaste = (file: File) => {
		resetPromptCycle();
		onAttach?.([file]);
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
		if (attachable.length === 0) return;
		resetPromptCycle();
		onAttach(attachable);
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
		// Lexical fires onChange synchronously from editor.setValue().
		// While cycling, compare incoming content to currentCycleValueRef,
		// the last value we applied. Different content means user input,
		// so reset; matching content is our own setValue echo, so keep cycling.
		// This works because React batches state updates within event handlers
		// and commits them after the handler returns, so the synchronous onChange
		// callback sees the pre-batch cycleIndex value, not the queued update.
		if (cycleIndex !== null && content !== currentCycleValueRef.current) {
			resetPromptCycle();
		}
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
		resetPromptCycle();
		if (!isMobileViewport()) {
			internalRef.current?.focus();
		}
	};
	const handleStartRecording = () => {
		resetPromptCycle();
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

		// isStreaming is intentionally excluded. Cycling is allowed while
		// streaming so the user can prepare the next prompt. Escape is
		// cycle-aware so it does not accidentally interrupt streaming.
		const isPromptCyclingSuppressed =
			editingQueuedMessageID !== null ||
			isEditingHistoryMessage ||
			isDisabled ||
			isLoading;
		if (isPromptCyclingSuppressed) {
			return;
		}

		if (e.key !== "ArrowUp" && e.key !== "ArrowDown") {
			return;
		}

		if (cycleIndex === null) {
			if (e.key !== "ArrowUp" || !isComposerEffectivelyEmpty) {
				return;
			}
			const cycleHistory = [...userPromptHistory];
			const latestPrompt = cycleHistory[0];
			if (latestPrompt === undefined) {
				return;
			}
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

		// ArrowUp: load an older prompt.
		const lastIndex = cycleHistory.length - 1;
		if (lastIndex < 0) {
			restoreCycleDraft();
			return;
		}
		const nextIndex = Math.min(cycleIndex + 1, lastIndex);
		if (nextIndex === cycleIndex) {
			return;
		}
		const nextPrompt = cycleHistory[nextIndex];
		if (nextPrompt === undefined) {
			restoreCycleDraft();
			return;
		}
		setCycleIndex(nextIndex);
		applyCycleValue(nextPrompt);
	};

	const sendButtonLabel =
		editingQueuedMessageID !== null
			? "Save"
			: isEditingHistoryMessage
				? "Save Edit"
				: "Send";
	const sendShortcutLabel =
		sendShortcut === MODIFIER_AGENT_CHAT_SEND_SHORTCUT
			? "Cmd/Ctrl+Enter"
			: "Enter";
	const sendButtonKeyShortcuts =
		sendShortcut === MODIFIER_AGENT_CHAT_SEND_SHORTCUT
			? "Control+Enter Meta+Enter"
			: "Enter";

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
			{showAgentSetupNotice && (
				<div className="relative z-0 mb-[-2.5rem]">
					{(aiGatewayDisabled ||
						(providerCount !== undefined && modelCount !== undefined)) &&
					canConfigureAgentSetup ? (
						<AgentSetupNotice
							isAdmin
							providerCount={providerCount ?? 0}
							modelCount={modelCount ?? 0}
							unsupportedProviderNames={unsupportedProviderNames}
							aiGatewayDisabled={aiGatewayDisabled}
						/>
					) : (
						<AgentSetupNotice
							isAdmin={false}
							providerCount={0}
							modelCount={0}
							unsupportedProviderNames={unsupportedProviderNames}
							aiGatewayDisabled={aiGatewayDisabled}
						/>
					)}
				</div>
			)}
			<div
				ref={setComposerElement}
				data-testid="chat-composer"
				className={cn(
					"relative z-10 rounded-2xl border border-border-default/80 bg-surface-secondary sm:bg-surface-secondary/45 p-1 shadow-sm has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40",
					showAgentSetupNotice && "sm:bg-surface-secondary",
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
							<PencilIcon className="size-3.5" />
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
							<XIcon className="size-3.5" />
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
					onPaste={resetPromptCycle}
					aria-label="Chat message"
					className="min-h-[60px] sm:min-h-24 w-full resize-none bg-transparent px-3 py-2 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
					placeholder={placeholder}
					initialValue={initialValue}
					initialEditorState={initialEditorState}
					remountKey={remountKey}
					onChange={handleContentChange}
					onKeyDown={handleEditorKeyDown}
					onEnter={handleSubmit}
					sendShortcut={sendShortcut}
					disabled={isDisabled || isLoading}
					autoFocus
				/>
				{/* Warn about invisible Unicode in the message text.
				 * Unlike the admin/user prompt textareas (which strip
				 * invisible chars server-side on save), the chat input
				 * is the user's free-form message; we don't silently
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
									disabled={
										isDisabled &&
										!showAgentSetupNotice &&
										!canUseWorkspacePicker
									}
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
										<WorkspacePickerList
											workspaceOptions={workspaceOptions}
											selectedWorkspaceId={selectedWorkspaceId}
											chatOrganizationId={chatOrganizationId}
											onSelect={(id) => {
												onWorkspaceChange?.(id);
												setPlusMenuOpen(false);
											}}
										/>
									</div>
								) : (
									<>
										{onAttach && (
											<button
												type="button"
												onClick={() => {
													resetPromptCycle();
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
													disabled={!canUseWorkspacePicker}
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
															disabled={!canUseWorkspacePicker}
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
														<WorkspacePickerList
															workspaceOptions={workspaceOptions}
															selectedWorkspaceId={selectedWorkspaceId}
															chatOrganizationId={chatOrganizationId}
															onSelect={(id) => {
																onWorkspaceChange(id);
																setWorkspacePickerOpen(false);
																setPlusMenuOpen(false);
															}}
														/>
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
																		<Spinner loading className="h-2.5 w-2.5" />
																	) : null}
																	Auth
																</Button>
															) : (
																<>
																	{server.auth_type === "oauth2" && (
																		<Button
																			variant="subtle"
																			size="icon"
																			className="size-6 shrink-0 text-content-secondary [&>svg]:size-3"
																			onClick={() => {
																				setPlusMenuOpen(false);
																				setMcpDisconnectTarget(server);
																			}}
																			disabled={isDisabled}
																			aria-label={`Disconnect ${server.display_name}`}
																		>
																			<UnlinkIcon />
																		</Button>
																	)}
																	<Switch
																		size="sm"
																		checked={isSelected}
																		onCheckedChange={(checked) =>
																			handleMcpToggle(server.id, checked)
																		}
																		disabled={isDisabled || isForceOn}
																		aria-label={`${isSelected ? "Disable" : "Enable"} ${server.display_name}`}
																	/>
																</>
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
								className="md:shrink"
								dropdownSide="top"
								dropdownAlign="start"
								enableMobileFullWidthDropdown
								reasoningEffort={reasoningEffort}
								onReasoningEffortChange={onReasoningEffortChange}
							/>
						)}
						{planModeEnabled && !shouldOverflowPlanningBadge && (
							<span
								data-testid="planning-badge"
								className="hidden shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary sm:inline-flex"
							>
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
						)}
						{/* Badge row; all badges and the pill always
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
									onRemoveWorkspace={removeWorkspaceHandler}
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
										onRemoveWorkspace={removeWorkspaceHandler}
										onRemoveMcp={handleRemoveMcp}
										onRemovePlanning={
											onPlanModeToggle ? handleDisablePlanMode : undefined
										}
										isDisabled={isDisabled}
										className={isOverflow ? "invisible order-1" : undefined}
									/>
								);
							})}
							{/* Pill; always in the DOM so it permanently
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
											onRemoveWorkspace={removeWorkspaceHandler}
											onRemoveMcp={handleRemoveMcp}
											onRemovePlanning={
												onPlanModeToggle ? handleDisablePlanMode : undefined
											}
											isDisabled={isDisabled}
										/>
									))}
								</PopoverContent>
							</Popover>
						</div>
					</div>
					<div className="flex shrink-0 items-center gap-2">
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
							<ContextUsageIndicator
								usage={contextUsage}
								onRefreshContext={onRefreshContext}
								isRefreshingContext={isRefreshingContext}
							/>
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
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										size="icon"
										variant="default"
										className="size-7 rounded-full transition-colors [&>svg]:!size-5 [&>svg]:p-0"
										onClick={
											speech.isRecording ? handleAcceptRecording : handleSubmit
										}
										disabled={speech.isRecording ? false : !canSend}
										aria-keyshortcuts={sendButtonKeyShortcuts}
									>
										{isLoading ? (
											<Spinner size="sm" loading aria-hidden="true" />
										) : speech.isRecording ? (
											<CheckIcon />
										) : (
											<ArrowUpIcon />
										)}
										<span className="sr-only">
											{speech.isRecording
												? "Accept voice input"
												: sendButtonLabel}
										</span>
									</Button>
								</TooltipTrigger>
								<TooltipContent side="top">
									{speech.isRecording
										? "Accept voice input"
										: `${sendButtonLabel}: ${sendShortcutLabel}`}
								</TooltipContent>
							</Tooltip>
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
			<ConfirmDialog
				open={mcpDisconnectTarget !== null}
				title={`Disconnect ${mcpDisconnectTarget?.display_name ?? "MCP server"}?`}
				description="This removes your credentials for this MCP server from Coder. You can authenticate again later."
				type="delete"
				confirmText="Disconnect"
				confirmLoading={mcpDisconnectMutation.isPending}
				onConfirm={handleMcpDisconnectConfirm}
				onClose={() => setMcpDisconnectTarget(null)}
			/>
		</>
	);
};

/**
 * Shared workspace picker used by both the mobile and desktop
 * "Attach workspace" menus. Workspaces from a different organization
 * than the chat are disabled unless already selected, so stale bindings
 * can still be cleared.
 */
interface WorkspacePickerListProps {
	workspaceOptions:
		| ReadonlyArray<{
				id: string;
				name: string;
				organization_id: string;
		  }>
		| undefined;
	selectedWorkspaceId?: string | null;
	chatOrganizationId?: string;
	onSelect: (id: string | null) => void;
}

const WorkspacePickerList: FC<WorkspacePickerListProps> = ({
	workspaceOptions,
	selectedWorkspaceId,
	chatOrganizationId,
	onSelect,
}) => {
	return (
		<Command loop>
			<CommandInput placeholder="Search workspaces..." className="text-xs" />
			<CommandList>
				<CommandEmpty className="text-xs">No workspaces found</CommandEmpty>
				<CommandGroup>
					{workspaceOptions?.map((workspace) => {
						const isCrossOrg =
							!!chatOrganizationId &&
							workspace.organization_id !== chatOrganizationId;
						const isSelected = selectedWorkspaceId === workspace.id;
						const isUnavailable = isCrossOrg && !isSelected;

						const item = (
							<CommandItem
								className={cn(
									"text-xs font-normal",
									isUnavailable &&
										"cursor-not-allowed opacity-50 data-[disabled=true]:pointer-events-auto",
								)}
								key={workspace.id}
								value={workspace.name}
								disabled={isUnavailable}
								onSelect={() => {
									if (!isUnavailable) {
										onSelect(isSelected ? null : workspace.id);
									}
								}}
							>
								{workspace.name}
								{isSelected && (
									<CheckIcon className="ml-auto size-icon-sm shrink-0" />
								)}
							</CommandItem>
						);

						if (isUnavailable) {
							return (
								<Tooltip key={workspace.id}>
									<TooltipTrigger asChild>
										<div>{item}</div>
									</TooltipTrigger>
									<TooltipContent side="top">
										Chat and workspace must be in the same organization
									</TooltipContent>
								</Tooltip>
							);
						}

						return item;
					})}
				</CommandGroup>
			</CommandList>
		</Command>
	);
};
