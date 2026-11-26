import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ChevronRightIcon,
	MicIcon,
	MonitorIcon,
	PaperclipIcon,
	PencilIcon,
	PlugIcon,
	PlusIcon,
	SquareIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import type React from "react";
import { useEffect, useId, useImperativeHandle, useRef, useState } from "react";
import { useQuery } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
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
import { Kbd, KbdGroup } from "#/components/Kbd/Kbd";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { toast } from "#/components/Toaster/toast";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import {
	coarsePointerMediaQuery,
	isBelowMdViewport,
	isMobileViewport,
} from "#/utils/mobile";
import { getOSKey } from "#/utils/platform";
import { useChatDropdownPosition } from "../hooks/useChatDropdownPosition";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import { useSpeechRecognition } from "../hooks/useSpeechRecognition";
import {
	isWorkspaceUploadInProgress,
	type WorkspaceFileUpload,
} from "../hooks/useWorkspaceFileUploads";
import {
	enterSendsAgentChatMessage,
	getAgentChatSendShortcut,
} from "../utils/agentChatSendShortcut";
import {
	chatAttachmentAcceptAttribute,
	isChatAttachmentFile,
	shouldRouteFileToWorkspace,
} from "../utils/chatAttachments";
import type { ChatSlashCommand } from "../utils/slashCommands";
import { AgentChatSendButton } from "./AgentChatSendButton";
import {
	AgentSetupNotice,
	shouldShowAgentSetupNotice,
} from "./AgentSetupNotice";
import {
	AttachmentPreview,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
import { isPendingRowID } from "./ChatConversation/chatStore";
import type { EditingTarget } from "./ChatConversation/types";
import {
	ChatMessageInput,
	type ChatMessageInputRef,
} from "./ChatMessageInput/ChatMessageInput";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import { type AttachedWorkspaceInfo, ChatToolBadges } from "./ChatToolBadges";
import type { AgentContextUsage } from "./ContextUsageIndicator";
import { ContextUsageIndicator } from "./ContextUsageIndicator";
import { ImageLightbox } from "./ImageLightbox";
import { MCPServersMenuContent } from "./MCPServersMenuContent";
import { getActiveMCPServers } from "./mcpMenuServers";
import { ProjectSkillActions } from "./ProjectSkillActions";
import {
	canSendQueuedMessageNow,
	QueuedMessagesList,
} from "./QueuedMessagesList";
import { TextPreviewDialog } from "./TextPreviewDialog";
import { WorkspaceUploadPreview } from "./WorkspaceUploadPreview";

export {
	ImageThumbnail,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
export type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";
export type { AgentContextUsage } from "./ContextUsageIndicator";

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

const workspaceRequiredAttachmentMessage =
	"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.";
const workspaceUploadPendingSendMessage =
	"Wait for the current message to finish sending, then add the file again.";

type AgentChatInputProps = {
	onSend: (message: string, busyBehavior?: TypesGen.ChatBusyBehavior) => void;
	placeholder?: string;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading: boolean;
	// Locks the editor while isLoading. Set it when the caller does not
	// clear the composer at submit, so text typed during the request would
	// join the sent text.
	lockWhileLoading?: boolean;
	// Ref for the Lexical editor, exposed for imperative access.
	inputRef?: React.Ref<ChatMessageInputRef>;
	// Initial text to seed the editor on first mount only.
	initialValue: string;
	// Serialized Lexical editor state for restoring drafts with
	// file-reference chips. Takes precedence over initialValue.
	initialEditorState?: string;
	// Monotonic counter to force editor remount.
	remountKey?: number;
	// Called on every content change inside the editor.
	onContentChange: (
		content: string,
		serializedEditorState: string,
		hasChips: boolean,
	) => void;
	// Model selector.
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	planModeEnabled: boolean;
	onPlanModeToggle: (enabled: boolean) => void;
	isModelCatalogLoading: boolean;
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
	// Project of the chat, whose skills the "/" menu offers.
	chatProjectId?: string;
	isWorkspaceLoading?: boolean;
	// Opts in to project action skills above the composer, which also need
	// a composer the user can send from.
	showProjectActions?: boolean;
	// Queued user messages rendered above the textarea.
	queuedMessages?: readonly ChatQueuedMessage[];
	onDeleteQueuedMessage?: (id: number) => Promise<void> | void;
	onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
	onEditQueuedMessage?: (id: number) => void;
	onEndQueuedMessageEdit?: (id: number) => Promise<void> | void;
	// Caution shown at the top of the composer, owned by the parent.
	warning?: string;
	isChatPaused?: boolean;
	// Editing state, owned by the parent. The kind selects the banner text.
	editingKind?: EditingTarget["kind"];
	queuedMessageUnderEditID?: number | null;
	composerQueuedMessageID?: number | null;
	onCancelEdit?: () => void;
	// Newest-first list of non-empty user prompts for local history cycling.
	userPromptHistory?: readonly string[];

	// Optional context-usage summary shown to the left of the send button.
	// Fields the provider has not reported render as fallback values.
	// Omit entirely to hide the indicator.
	contextUsage?: AgentContextUsage;
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
	workspaceUploads?: WorkspaceUploadsProps;
	onTextPreview?: (
		content: string,
		fileName: string,
		mediaType: string,
	) => void;
	// MCP Server picker.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	workspaceSkills?: readonly SkillMetadata[];
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	chatId?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	folder?: string;
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	// AI Gateway is disabled deployment-wide, independent of provider/model
	// configuration. Forces the setup notice regardless of the counts above.
	aiGatewayDisabled?: boolean;
	// Built-in commands offered by the "/" trigger menu ahead of
	// personal skills.
	slashCommands?: readonly ChatSlashCommand[];
	fillWidth?: boolean;
	highlightKey?: string;
};

type MCPServersMenuRowContentProps = {
	activeCount: number;
	isOpen?: boolean;
};

const MCPServersMenuRowContent: React.FC<MCPServersMenuRowContentProps> = ({
	activeCount,
	isOpen,
}) => (
	<>
		<PlugIcon className="size-3.5 shrink-0" />
		<span>MCP servers</span>
		<span className="ml-auto">{activeCount} on</span>
		<ChevronRightIcon
			className={cn(
				"size-icon-sm transition-transform",
				isOpen && "rotate-180",
			)}
		/>
	</>
);

type PlusMenuSubmenu = "workspace" | "mcp";

// Below md, swapping the plus menu's views unmounts the focused control.
// Move focus to its replacement so keyboard navigation can continue.
const focusOnMount = (element: HTMLElement | null) => {
	element?.focus();
};

type PlusMenuSubmenuPanelProps = {
	onBack: () => void;
	children: React.ReactNode;
};

const PlusMenuSubmenuPanel: React.FC<PlusMenuSubmenuPanelProps> = ({
	onBack,
	children,
}) => (
	<div className="p-0">
		<button
			ref={focusOnMount}
			type="button"
			onClick={onBack}
			className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
		>
			<ArrowLeftIcon className="size-3.5 shrink-0" />
			<span>Back</span>
		</button>
		<Separator className="my-1" />
		{children}
	</div>
);

export const AgentChatInput: React.FC<AgentChatInputProps> = ({
	onSend,
	placeholder = "Type a message...",
	isDisabled,
	isReadOnly = false,
	lockWhileLoading = false,
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
	planModeEnabled,
	onPlanModeToggle,
	isModelCatalogLoading,
	isStreaming = false,
	onInterrupt,
	isInterruptPending = false,
	workspaceOptions,
	selectedWorkspaceId,
	onWorkspaceChange,
	chatOrganizationId,
	chatProjectId,
	isWorkspaceLoading,
	showProjectActions = false,
	queuedMessages = [],
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	onEditQueuedMessage,
	onEndQueuedMessageEdit,
	warning,
	isChatPaused = false,
	editingKind,
	queuedMessageUnderEditID = null,
	composerQueuedMessageID = null,
	onCancelEdit,
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
	workspaceUploads,
	onTextPreview,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	workspaceSkills,
	workspace,
	workspaceAgent,
	chatId,
	attachedWorkspace,
	folder,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames = [],
	aiGatewayDisabled,
	slashCommands,
	fillWidth = false,
	highlightKey,
}) => {
	const isEditingMessage = editingKind !== undefined;
	const enterSendsHead =
		!isEditingMessage &&
		queuedMessages.length > 0 &&
		queuedMessages[0].id !== queuedMessageUnderEditID &&
		canSendQueuedMessageNow(queuedMessages[0].busy_behavior) &&
		!isPendingRowID(queuedMessages[0].id);
	const warningId = useId();
	const preferencesQuery = useQuery(preferenceSettings());
	const sendShortcut = getAgentChatSendShortcut(
		preferencesQuery.data?.agent_chat_send_shortcut,
		preferencesQuery.isLoading,
	);
	const [chatFullWidth] = useChatFullWidth();
	const { organizations } = useDashboard();
	const chatOrganization = organizations.find(
		(organization) => organization.id === chatOrganizationId,
	);
	const showAgentSetupNotice = shouldShowAgentSetupNotice({
		isAdmin: canConfigureAgentSetup,
		providerCount,
		modelCount,
		aiGatewayDisabled,
	});
	const internalRef = useRef<ChatMessageInputRef>(null);
	useEffect(() => {
		if (highlightKey !== undefined) {
			internalRef.current?.focus();
		}
	}, [highlightKey]);
	// Browsers treat any contenteditable focus as focus-visible, including
	// autofocus, so only ring focus that arrived via Tab.
	const lastInputWasTabRef = useRef(false);
	const [isEditorKeyboardFocused, setIsEditorKeyboardFocused] = useState(false);
	useEffect(() => {
		const handleKeyDown = (event: KeyboardEvent) => {
			lastInputWasTabRef.current = event.key === "Tab";
		};
		const handlePointerDown = () => {
			lastInputWasTabRef.current = false;
		};
		document.addEventListener("keydown", handleKeyDown, true);
		document.addEventListener("pointerdown", handlePointerDown, true);
		return () => {
			document.removeEventListener("keydown", handleKeyDown, true);
			document.removeEventListener("pointerdown", handlePointerDown, true);
		};
	}, []);
	const [previewImage, setPreviewImage] = useState<string | null>(null);
	const [previewText, setPreviewText] = useState<string | null>(null);
	const [previewTextFileName, setPreviewTextFileName] = useState<string | null>(
		null,
	);
	const [previewTextMediaType, setPreviewTextMediaType] = useState<
		string | null
	>(null);
	const [plusMenuOpen, setPlusMenuOpen] = useState(false);
	const [plusMenuView, setPlusMenuView] = useState<"main" | PlusMenuSubmenu>(
		"main",
	);
	// The submenu that Back closed, so the main view focuses its row.
	const [lastPlusMenuSubmenu, setLastPlusMenuSubmenu] =
		useState<PlusMenuSubmenu | null>(null);
	const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false);
	const [mcpMenuOpen, setMcpMenuOpen] = useState(false);
	const navigate = useNavigate();
	const location = useLocation();

	const [hasChips, setHasChips] = useState(false);
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
		// callbacks but the exhaustive-deps lint does not.
		setCycleIndex(null);
		setCycleSavedDraft(null);
		cycleHistorySnapshotRef.current = null;
		currentCycleValueRef.current = null;
		// Keep in sync with resetPromptCycle above.
	}, [remountKey]);

	const speech = useSpeechRecognition();
	// The draft, including skill chips, as it was when recording started.
	const [preRecordingState, setPreRecordingState] = useState<string | null>(
		null,
	);

	useEffect(() => {
		if (!speech.isRecording || preRecordingState === null) return;
		internalRef.current?.restoreEditorState(
			preRecordingState,
			speech.transcript,
		);
	}, [speech.transcript, speech.isRecording, preRecordingState]);

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
			insertSkill: (source, name) =>
				internalRef.current?.insertSkill(source, name),
			removeSkills: (sources) => internalRef.current?.removeSkills(sources),
			getEditorState: () => internalRef.current?.getEditorState() ?? "",
			restoreEditorState: (serializedEditorState, appendText) =>
				internalRef.current?.restoreEditorState(
					serializedEditorState,
					appendText,
				),
			getContentParts: () => internalRef.current?.getContentParts() ?? [],
		}),
		[],
	);

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

	// Only a chat-bound workspace counts: an unbound selection (new chat
	// form, or a picked workspace before the first send) has no pinned
	// context to resolve, so treating it as a workspace would leave the
	// menu in the loading state forever.
	const hasSkillsWorkspace = Boolean(attachedWorkspace?.id ?? workspace?.id);

	const selectedWorkspace = workspaceOptions?.find(
		(ws) => ws.id === selectedWorkspaceId,
	);
	const canUseWorkspacePicker =
		Boolean(onWorkspaceChange) && !isWorkspaceLoading;
	const enabledMcpServers = mcpServers?.filter((s) => s.enabled) ?? [];
	const activeMcpServers = getActiveMCPServers(
		enabledMcpServers,
		selectedMCPServerIds,
	);

	const handleRemoveWorkspace = onWorkspaceChange
		? () => onWorkspaceChange(null)
		: undefined;

	const closePlusMenuSubmenu = (submenu: PlusMenuSubmenu) => {
		setLastPlusMenuSubmenu(submenu);
		setPlusMenuView("main");
	};
	const plusMenuRowRef = (submenu: PlusMenuSubmenu) =>
		lastPlusMenuSubmenu === submenu ? focusOnMount : undefined;

	const handleManageConnections = () => {
		setMcpMenuOpen(false);
		setPlusMenuOpen(false);
		setPlusMenuView("main");
		navigate("/agents/settings/mcp-connections", {
			state: {
				from: location.pathname + location.search,
				organizationId: chatOrganizationId,
			},
		});
	};
	const mcpMenuContent = (
		<MCPServersMenuContent
			servers={enabledMcpServers}
			selectedServerIds={selectedMCPServerIds}
			isDisabled={isDisabled}
			onToggle={handleMcpToggle}
			onManageConnections={handleManageConnections}
		/>
	);

	let plusMenuSubmenuPanel: React.ReactNode = null;
	if (plusMenuView === "mcp") {
		plusMenuSubmenuPanel = (
			<PlusMenuSubmenuPanel onBack={() => closePlusMenuSubmenu("mcp")}>
				{mcpMenuContent}
			</PlusMenuSubmenuPanel>
		);
	} else if (plusMenuView === "workspace") {
		plusMenuSubmenuPanel = (
			<PlusMenuSubmenuPanel onBack={() => closePlusMenuSubmenu("workspace")}>
				<WorkspacePickerList
					workspaceOptions={workspaceOptions}
					selectedWorkspaceId={selectedWorkspaceId}
					chatOrganizationId={chatOrganizationId}
					onSelect={(id) => {
						onWorkspaceChange?.(id);
						setPlusMenuOpen(false);
					}}
				/>
			</PlusMenuSubmenuPanel>
		);
	}

	const handlePlanModeToggle = () => {
		onPlanModeToggle(!planModeEnabled);
		setPlusMenuOpen(false);
	};

	const handleDisablePlanMode = () => onPlanModeToggle(false);

	const fileInputRef = useRef<HTMLInputElement>(null);
	const [composerElement, setComposerElement] = useState<HTMLDivElement | null>(
		null,
	);
	useChatDropdownPosition(composerElement);

	// Workspace uploads eagerly write bytes into the workspace, so a
	// disabled (read-only) composer must not route files to them. The
	// same holds while a send is pending: the post-send reset would drop
	// the chip after the bytes already landed in the workspace.
	const workspaceAttachBlockedBySend =
		isLoading && workspaceUploads?.onAttach !== undefined;
	const onWorkspaceAttach =
		isDisabled || isLoading ? undefined : workspaceUploads?.onAttach;

	// Splits files between the attachment pipeline and the workspace
	// upload path by declared MIME type. Unknown or octet-stream files
	// stay on the attachment path where the server classifies bytes.
	// Returns whether any file was routed.
	const routeFiles = (files: File[]): boolean => {
		const attachable: File[] = [];
		const forWorkspace: File[] = [];
		const rejected: File[] = [];
		// Workspace-routable files that arrived while the workspace
		// upload feature is wired but currently unavailable (no
		// connected agent, read-only composer, pending send). They need
		// a workspace, not a different file type, so they get their own
		// toast.
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
		if (attachable.length === 0 && forWorkspace.length === 0) {
			return false;
		}
		resetPromptCycle();
		if (attachable.length > 0) {
			onAttach?.(attachable);
		}
		if (forWorkspace.length > 0) {
			onWorkspaceAttach?.(forWorkspace);
		}
		return true;
	};

	const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (e.target.files?.length) {
			routeFiles(Array.from(e.target.files));
		}
		// Reset so the same file can be selected again.
		e.target.value = "";
	};

	const handleFilePaste = (file: File) => routeFiles([file]);

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
		if (!e.dataTransfer.files.length) return;
		routeFiles(Array.from(e.dataTransfer.files));
	};

	// Track whether the editor has content so we can gate the
	// send button without a controlled value prop.
	const [hasContent, setHasContent] = useState(() =>
		Boolean(initialValue.trim()),
	);

	const [invisibleCharCount, setInvisibleCharCount] = useState(() =>
		countInvisibleCharacters(initialValue),
	);

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasChipsInEditor: boolean,
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
		setHasChips(hasChipsInEditor);
		setInvisibleCharCount(countInvisibleCharacters(content));
		onContentChange(content, serializedEditorState, hasChipsInEditor);
	};

	// A locked editor loses focus during the request, so re-focus it after
	// a send completes.
	const prevIsLoadingRef = useRef(isLoading);
	useEffect(() => {
		const wasLoading = prevIsLoadingRef.current;
		prevIsLoadingRef.current = isLoading;
		if (lockWhileLoading && wasLoading && !isLoading && !isMobileViewport()) {
			internalRef.current?.focus();
		}
	}, [isLoading, lockWhileLoading]);
	const workspaceUploadEntries = workspaceUploads?.uploads ?? [];
	const hasActiveUploads =
		attachments.some((file) => isUploadInProgress(uploadStates?.get(file))) ||
		workspaceUploadEntries.some(isWorkspaceUploadInProgress);
	// Deferred workspace entries upload during submit, so they count as
	// sendable content just like finished uploads. In deferred mode
	// failed entries stay sendable too: the next send re-uploads them
	// against the fresh chat.
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
		hasChips;
	const isComposerEffectivelyEmpty = !hasDraftContext;
	const hasSendableContent = hasContent || hasUploadedAttachments || hasChips;
	const canSend =
		!isDisabled &&
		!isReadOnly &&
		!isLoading &&
		hasModelOptions &&
		hasSendableContent &&
		!hasActiveUploads;
	const submit = (busyBehavior?: TypesGen.ChatBusyBehavior) => {
		const text = internalRef.current?.getValue()?.trim() ?? "";

		if (
			!busyBehavior &&
			!text &&
			!hasUploadedAttachments &&
			!hasChips &&
			!isDisabled &&
			!isReadOnly &&
			!isLoading &&
			!hasActiveUploads &&
			enterSendsHead &&
			onPromoteQueuedMessage
		) {
			void onPromoteQueuedMessage(queuedMessages[0].id);
			return;
		}

		if (
			(!text && !hasUploadedAttachments && !hasChips) ||
			isDisabled ||
			isReadOnly ||
			isLoading ||
			hasActiveUploads ||
			!hasModelOptions
		) {
			return;
		}

		onSend(text, busyBehavior);
		resetPromptCycle();
		if (!isMobileViewport()) {
			internalRef.current?.focus();
		}
	};
	const handleSubmit = () => submit();
	const offersSendAfterTurn = isStreaming && !isEditingMessage;
	const handleSubmitAfterTurn = () => submit("queue");
	const handleStartRecording = () => {
		resetPromptCycle();
		setPreRecordingState(internalRef.current?.getEditorState() ?? null);
		speech.start();
	};

	const handleAcceptRecording = () => {
		speech.stop();
		if (!isMobileViewport()) {
			internalRef.current?.focus();
		}
	};

	const handleCancelRecording = () => {
		speech.cancel();
		const editor = internalRef.current;
		if (editor) {
			if (preRecordingState !== null) {
				editor.restoreEditorState(preRecordingState);
			}
			if (!isMobileViewport()) {
				editor.focus();
			}
		}
		setPreRecordingState(null);
	};

	const handleComposerKeyDown = (e: React.KeyboardEvent) => {
		// React bubbles keydown from portaled popovers (MCP servers, model
		// selector) through the composer; Escape there only closes the popover.
		if (e.key === "Escape" && e.currentTarget.contains(e.target as Node)) {
			if (isEditingMessage) {
				e.preventDefault();
				onCancelEdit?.();
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
			isEditingMessage || isReadOnly || isLoading;
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

	const sendButtonLabel = isEditingMessage ? "Save edit" : "Send";
	// Stop and the send button are mutually exclusive while streaming; a
	// non-empty draft or a live recording selects the send button. A live
	// recording also takes precedence over history editing. A queued edit
	// does not start a turn, so it takes the slot like a draft.
	const draftOccupiesSlot =
		hasSendableContent || hasActiveUploads || speech.isRecording;
	const editingHoldsStop = editingKind === "history" && !speech.isRecording;
	const canShowProjectActions =
		showProjectActions &&
		!isReadOnly &&
		!isLoading &&
		!isDisabled &&
		hasModelOptions &&
		!aiGatewayDisabled;
	const showStopButton =
		isStreaming &&
		onInterrupt !== undefined &&
		(!draftOccupiesSlot || editingHoldsStop);
	const showSendButton =
		!isStreaming || (draftOccupiesSlot && !editingHoldsStop);

	const content = (
		<div
			className={cn(
				"mx-auto w-full",
				fillWidth
					? "max-w-full"
					: [chatWidthClass(chatFullWidth), "pb-0 sm:pb-4"],
				isEditingMessage && "pt-1",
			)}
		>
			{canShowProjectActions && chatOrganizationId && chatProjectId && (
				<ProjectSkillActions
					organizationId={chatOrganizationId}
					projectId={chatProjectId}
					onInvoke={(name) => internalRef.current?.insertSkill("project", name)}
				/>
			)}
			{queuedMessages.length > 0 && (
				<QueuedMessagesList
					messages={queuedMessages}
					onDelete={onDeleteQueuedMessage}
					onPromote={onPromoteQueuedMessage}
					onEdit={onEditQueuedMessage}
					onEndEdit={onEndQueuedMessageEdit}
					isChatPaused={isChatPaused}
					queuedMessageUnderEditID={queuedMessageUnderEditID}
					composerQueuedMessageID={composerQueuedMessageID}
					sendShortcutHint={
						enterSendsHead ? (
							<QueuedMessageSendShortcutHint sendShortcut={sendShortcut} />
						) : undefined
					}
					className="mb-2"
				/>
			)}
			{showAgentSetupNotice && (
				<div className="relative z-0 -mb-10">
					{canConfigureAgentSetup ? (
						<AgentSetupNotice
							isAdmin
							providerCount={providerCount ?? 0}
							modelCount={modelCount ?? 0}
							organization={chatOrganization}
							unsupportedProviderNames={unsupportedProviderNames}
							aiGatewayDisabled={aiGatewayDisabled}
						/>
					) : (
						<AgentSetupNotice
							isAdmin={false}
							providerCount={0}
							modelCount={0}
							organization={chatOrganization}
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
					"relative z-10 rounded-2xl bg-surface-secondary sm:bg-surface-secondary/45 p-1 shadow-xs",
					isEditorKeyboardFocused && "ring-2 ring-content-link",
					showAgentSetupNotice && "sm:bg-surface-secondary",
					isDragging && "ring-2 ring-content-link/40",
					(isEditingMessage || warning) && "shadow-(--shadow-border-warning)",
				)}
				onKeyDown={handleComposerKeyDown}
				onFocus={(event) => {
					if (
						event.target instanceof HTMLElement &&
						event.target.isContentEditable
					) {
						setIsEditorKeyboardFocused(lastInputWasTabRef.current);
					}
				}}
				onBlur={(event) => {
					if (
						event.target instanceof HTMLElement &&
						event.target.isContentEditable
					) {
						setIsEditorKeyboardFocused(false);
					}
				}}
				onDragOver={onAttach ? handleDragOver : undefined}
				onDragLeave={onAttach ? handleDragLeave : undefined}
				onDrop={onAttach ? handleDrop : undefined}
			>
				{highlightKey !== undefined && (
					<span
						key={highlightKey}
						aria-hidden="true"
						className="pointer-events-none absolute inset-0 rounded-2xl opacity-0 ring-2 ring-content-link animate-flash"
					/>
				)}
				{warning && (
					<div
						id={warningId}
						className="flex items-start gap-1.5 border-b border-border/70 px-3 py-1.5 text-xs text-content-warning"
					>
						<TriangleAlertIcon className="mt-px size-3.5 shrink-0" />
						{warning}
					</div>
				)}
				{isEditingMessage && (
					<div className="flex items-center justify-between border-b border-border/70 px-3 py-1.5">
						<span className="flex items-center gap-1.5 text-xs text-content-warning">
							<PencilIcon className="size-3.5" />
							{editingKind === "queued"
								? "Editing a queued message. It is not sent until you save or cancel."
								: "Editing will delete all subsequent messages and restart the conversation here."}
						</span>
						<Button
							type="button"
							variant="subtle"
							size="icon"
							aria-label={
								editingKind === "queued" && isChatPaused
									? "Cancel edit and send unchanged"
									: "Cancel editing"
							}
							onClick={onCancelEdit}
							disabled={isLoading}
							className="size-6 rounded text-border-warning hover:text-content-primary"
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
				{workspaceUploads && (
					<WorkspaceUploadPreview
						uploads={workspaceUploads.uploads}
						onRemove={workspaceUploads.onRemove}
					/>
				)}
				<ChatMessageInput
					ref={internalRef}
					onFilePaste={onAttach ? handleFilePaste : undefined}
					acceptFilePasteWhileDisabled={
						lockWhileLoading && isLoading && !isReadOnly
					}
					onPaste={resetPromptCycle}
					aria-label="Chat message"
					aria-describedby={warning ? warningId : undefined}
					className="min-h-15 sm:min-h-24 w-full resize-none bg-transparent px-3 py-2 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
					placeholder={placeholder}
					initialValue={initialValue}
					initialEditorState={initialEditorState}
					remountKey={remountKey}
					onChange={handleContentChange}
					onKeyDown={handleEditorKeyDown}
					onEnter={handleSubmit}
					onSendAfterTurn={
						offersSendAfterTurn ? handleSubmitAfterTurn : undefined
					}
					sendShortcut={sendShortcut}
					// An unlocked editor stays editable while isLoading, and
					// submit sends nothing then.
					disabled={isReadOnly || (lockWhileLoading && isLoading)}
					hasWorkspace={hasSkillsWorkspace}
					organizationId={chatOrganizationId}
					projectId={chatProjectId}
					workspaceSkills={workspaceSkills}
					slashCommands={slashCommands}
					skillsMenuAnchor={composerElement}
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
				{/* Hidden file input for attaching files. When workspace
				uploads are wired, every file type stays selectable even
				while no workspace is ready: iOS silently greys out
				filtered types, so routeFiles explains the refusal. */}
				{onAttach && (
					<input
						ref={fileInputRef}
						type="file"
						data-testid="chat-attachment-file-input"
						multiple
						accept={
							workspaceUploads ? undefined : chatAttachmentAcceptAttribute
						}
						onChange={handleFileSelect}
						className="hidden"
					/>
				)}
				<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
					{/* flex-1 routes free row space to the growing pills. */}
					<div className="flex min-w-0 flex-1 items-center gap-1">
						{/* Plus menu */}
						<Popover
							modal={false}
							open={plusMenuOpen}
							onOpenChange={(open) => {
								setPlusMenuOpen(open);
								setLastPlusMenuSubmenu(null);
								if (!open) {
									setPlusMenuView("main");
									setMcpMenuOpen(false);
								}
							}}
						>
							{" "}
							<PopoverTrigger asChild>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
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
								className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto min-w-50 p-1"
							>
								{plusMenuView !== "main" ? (
									plusMenuSubmenuPanel
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
										{workspaceOptions &&
											onWorkspaceChange &&
											(isBelowMdViewport() ? (
												<button
													type="button"
													disabled={!canUseWorkspacePicker}
													ref={plusMenuRowRef("workspace")}
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
												{isBelowMdViewport() ? (
													<button
														type="button"
														ref={plusMenuRowRef("mcp")}
														onClick={() => setPlusMenuView("mcp")}
														className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
													>
														<MCPServersMenuRowContent
															activeCount={activeMcpServers.length}
														/>
													</button>
												) : (
													<Popover
														open={mcpMenuOpen}
														onOpenChange={setMcpMenuOpen}
													>
														<PopoverTrigger asChild>
															<button
																type="button"
																className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
															>
																<MCPServersMenuRowContent
																	activeCount={activeMcpServers.length}
																	isOpen={mcpMenuOpen}
																/>
															</button>
														</PopoverTrigger>
														<PopoverContent
															side="right"
															align="end"
															sideOffset={8}
															className="w-72 p-1"
														>
															{mcpMenuContent}
														</PopoverContent>
													</Popover>
												)}
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
								className="grow shrink-0 basis-[calc(8ch_+_3.125rem)] max-w-max md:h-auto focus-visible:ring-inset"
								dropdownSide="top"
								dropdownAlign="start"
								enableMobileFullWidthDropdown
								reasoningEffort={reasoningEffort}
								onReasoningEffortChange={onReasoningEffortChange}
							/>
						)}
						<ChatToolBadges
							planModeEnabled={planModeEnabled}
							collapsePlanningBadge={contextUsage !== undefined}
							workspace={workspace}
							workspaceAgent={workspaceAgent}
							chatId={chatId}
							folder={folder}
							attachedWorkspace={attachedWorkspace}
							selectedWorkspace={selectedWorkspace}
							mcpServers={
								enabledMcpServers.length > 0 ? activeMcpServers : undefined
							}
							mcpMenu={mcpMenuContent}
							onRemoveWorkspace={handleRemoveWorkspace}
							onRemovePlanning={handleDisablePlanMode}
							isDisabled={isDisabled}
						/>
					</div>
					<div className="flex shrink-0 items-center gap-2">
						{contextUsage !== undefined && (
							<div
								className={cn(
									"flex",
									speech.isSupported && !speech.error && "-ml-2",
								)}
							>
								<ContextUsageIndicator
									usage={contextUsage}
									onRefreshContext={onRefreshContext}
									isRefreshingContext={isRefreshingContext}
									onManageServers={handleManageConnections}
								/>
							</div>
						)}
						{speech.isSupported && (
							<>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
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
									{speech.isRecording ? (
										<XIcon />
									) : (
										<MicIcon strokeWidth={1.5} />
									)}
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
						{showSendButton && (
							<AgentChatSendButton
								label={sendButtonLabel}
								sendShortcut={sendShortcut}
								canSend={canSend}
								offersSendAfterTurn={offersSendAfterTurn}
								isRecording={speech.isRecording}
								isSending={isLoading && !isInterruptPending}
								onSend={handleSubmit}
								onSendAfterTurn={handleSubmitAfterTurn}
								onAcceptRecording={handleAcceptRecording}
							/>
						)}
						{showStopButton && (
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										size="icon"
										variant="default"
										className="size-7 rounded-full transition-colors [&>svg]:size-3! [&>svg]:p-0"
										onClick={onInterrupt}
										disabled={isInterruptPending}
									>
										<SquareIcon className="fill-current" />
										<span className="sr-only">Stop</span>
									</Button>
								</TooltipTrigger>
								<TooltipContent side="top">
									{isInterruptPending ? "Interrupting…" : "Stop"}
								</TooltipContent>
							</Tooltip>
						)}
						{isInterruptPending && isStreaming && (
							// Live region announcing the pending interruption.
							<span role="status" className="sr-only">
								Interrupting. Waiting for the agent to stop.
							</span>
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

type QueuedMessageSendShortcutHintProps = {
	sendShortcut: TypesGen.AgentChatSendShortcut;
};

const QueuedMessageSendShortcutHint: React.FC<
	QueuedMessageSendShortcutHintProps
> = ({ sendShortcut }) => {
	const isCoarsePointer = useMediaQuery(coarsePointerMediaQuery);

	return (
		<KbdGroup>
			{!enterSendsAgentChatMessage(sendShortcut, isCoarsePointer) && (
				<Kbd>{getOSKey()}</Kbd>
			)}
			<Kbd>Enter</Kbd>
		</KbdGroup>
	);
};

/**
 * Shared workspace picker used by both the mobile and desktop
 * "Attach workspace" menus. Workspaces from a different organization
 * than the chat are disabled unless already selected, so stale bindings
 * can still be cleared.
 */
type WorkspacePickerListProps = {
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
};

const WorkspacePickerList: React.FC<WorkspacePickerListProps> = ({
	workspaceOptions,
	selectedWorkspaceId,
	chatOrganizationId,
	onSelect,
}) => {
	return (
		<Command loop label="Search workspaces">
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
								aria-selected={isSelected}
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
