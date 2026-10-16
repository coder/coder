import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	InfoIcon,
	LockIcon,
	MonitorIcon,
	PaperclipIcon,
	PencilIcon,
	PlusIcon,
	ServerIcon,
	UnlinkIcon,
	XIcon,
} from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { disconnectMCPServerOAuth2 } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	type PopoverContentProps,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { toast } from "#/components/Toaster/toast";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { isBelowMdViewport } from "#/utils/mobile";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import { useMCPOAuthFlow } from "../hooks/useMCPOAuthFlow";
import { useOverflowCount } from "../hooks/useOverflowCount";
import { chatAttachmentAcceptAttribute } from "../utils/chatAttachments";
import type { ChatSlashCommand } from "../utils/slashCommands";
import { AgentComposer } from "./AgentComposer";
import type { ComposerFileBindings } from "./AgentComposer/context";
import { useComposerRuntime } from "./AgentComposer/useComposerRuntime";
import { AgentSetupNotice } from "./AgentSetupNotice";
import type { UploadState } from "./AttachmentPreview";
import type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";
import type { SkillMetadata } from "./ChatMessageInput/SkillsTriggerMenu";
import type { AgentContextUsage } from "./ContextUsageIndicator";
import { ContextUsageIndicator } from "./ContextUsageIndicator";
import { MCPServerIconStack } from "./MCPServerIconStack";
import { QueuedMessagesList } from "./QueuedMessagesList";
import { WorkspacePill } from "./WorkspacePill";

export {
	ImageThumbnail,
	isUploadInProgress,
	type UploadState,
} from "./AttachmentPreview";
export type { AgentContextUsage } from "./ContextUsageIndicator";

type AgentChatInputProps = {
	onSend: (message: string) => void;
	placeholder?: string;
	isDisabled: boolean;
	isReadOnly?: boolean;
	isLoading: boolean;
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
	isWorkspaceLoading?: boolean;
	// Queued user messages rendered above the textarea.
	queuedMessages?: readonly ChatQueuedMessage[];
	onDeleteQueuedMessage?: (id: number) => Promise<void> | void;
	onPromoteQueuedMessage?: (id: number) => Promise<void> | void;
	// Caution shown at the top of the composer, owned by the parent.
	warning?: string;
	// When set, also hides the editor and every control (view-only mode).
	readOnlyNotice?: string;
	// History editing state, owned by the parent.
	isEditingHistoryMessage?: boolean;
	onCancelHistoryEdit?: () => void;
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
	workspaceUploads?: ComposerFileBindings["workspaceUploads"];
	onTextPreview?: (
		content: string,
		fileName: string,
		mediaType: string,
	) => void;
	// MCP Server picker.
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	workspaceSkills?: readonly SkillMetadata[];
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
	// Built-in commands offered by the "/" trigger menu ahead of
	// personal skills.
	slashCommands?: readonly ChatSlashCommand[];
	fillWidth?: boolean;
};

export type AttachedWorkspaceInfo = {
	id: string;
	name: string;
	route: string;
	statusIcon: React.ReactNode;
	statusLabel: string;
};
// Shared pill sizing: flex-basis sets a ~8ch floor (shrink-0 enforces
// it), grow expands into free row space, and max-w-max caps at the
// label's natural width. Below the floor the +N overflow takes over.
const pillSizingClasses =
	"grow shrink-0 basis-[calc(8ch_+_3.125rem)] max-w-max";

// Pills clamp to the popover width so a long name truncates instead
// of pushing its X out of view.
const BadgePopoverContent: React.FC<PopoverContentProps> = ({
	className,
	...props
}) => (
	<PopoverContent
		side="top"
		align="start"
		className={cn(
			"flex w-auto max-w-64 flex-wrap gap-1 p-2 *:max-w-full",
			className,
		)}
		{...props}
	/>
);

type ToolBadgeData =
	| { kind: "workspace"; name: string }
	| ({ kind: "attached-workspace" } & AttachedWorkspaceInfo)
	| { kind: "mcp"; server: TypesGen.MCPServerConfig }
	| { kind: "mcp-group"; servers: readonly TypesGen.MCPServerConfig[] }
	| { kind: "planning" };

// Non-MCP badges can share a kind, so their keys are position-qualified.
const badgeKey = (badge: ToolBadgeData, index: number) => {
	switch (badge.kind) {
		case "mcp":
			return badge.server.id;
		case "mcp-group":
			return badge.kind;
		default:
			return `${badge.kind}-${index}`;
	}
};

// Small `X` button rendered inside pill-style badges (attached
// workspace, MCP server, planning indicator) to dismiss or disable
// the badge without opening the `+` menu. Callers pass the action
// handler and a descriptive aria-label.
const BadgeDismissButton: React.FC<{
	onClick: () => void;
	ariaLabel: string;
	isDisabled?: boolean;
}> = ({ onClick, ariaLabel, isDisabled = false }) => (
	<button
		type="button"
		onClick={onClick}
		disabled={isDisabled}
		className="group -mx-1 -my-1 inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-full border-0 bg-transparent p-0 text-content-secondary disabled:cursor-not-allowed disabled:opacity-50"
		aria-label={ariaLabel}
	>
		<span className="inline-flex size-3.5 items-center justify-center rounded-full transition-colors group-hover:bg-surface-tertiary group-hover:text-content-primary">
			<XIcon className="size-2.5!" />
		</span>
	</button>
);

type MCPGroupBadgeProps = {
	servers: readonly TypesGen.MCPServerConfig[];
	onRemoveMcp?: (serverId: string) => void;
	isDisabled: boolean;
	className: string;
};

const MCPGroupBadge: React.FC<MCPGroupBadgeProps> = ({
	servers,
	onRemoveMcp,
	isDisabled,
	className,
}) => {
	const [open, setOpen] = useState(false);
	const label = `${servers.length} MCPs`;

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label={label}
					className={cn(
						className,
						"cursor-pointer border-0 transition-colors hover:bg-surface-tertiary hover:text-content-primary",
					)}
				>
					<MCPServerIconStack servers={servers} />
					{label}
					<ChevronDownIcon
						className={cn("size-3 transition-transform", open && "rotate-180")}
					/>
				</button>
			</PopoverTrigger>
			<BadgePopoverContent>
				{servers.map((server) => (
					<ToolBadge
						key={server.id}
						badge={{ kind: "mcp", server }}
						onRemoveMcp={onRemoveMcp}
						isDisabled={isDisabled}
					/>
				))}
			</BadgePopoverContent>
		</Popover>
	);
};

const ToolBadge: React.FC<{
	badge: ToolBadgeData;
	onRemoveWorkspace?: () => void;
	onRemoveMcp?: (serverId: string) => void;
	onRemovePlanning?: () => void;
	isDisabled: boolean;
	className?: string;
	// The overflow popover auto-focuses badges; suppress the tooltip there.
	disableTooltip?: boolean;
}> = ({
	badge,
	onRemoveWorkspace,
	onRemoveMcp,
	onRemovePlanning,
	isDisabled,
	className,
	disableTooltip,
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
								isDisabled={isDisabled}
							/>
						)}
					</span>
				</TooltipTrigger>
				{/* Hidden below md: touch focus would stick the tooltip open. */}
				{!disableTooltip && (
					<TooltipContent className="hidden md:block">
						{badge.statusLabel}
					</TooltipContent>
				)}
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
						isDisabled={isDisabled}
					/>
				)}
			</span>
		);
	}

	if (badge.kind === "mcp-group") {
		return (
			<MCPGroupBadge
				servers={badge.servers}
				onRemoveMcp={onRemoveMcp}
				isDisabled={isDisabled}
				className={badgeCls}
			/>
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
			<span className="truncate">{badge.server.display_name}</span>
			{isForceOn ? (
				<>
					<LockIcon className="size-3 shrink-0" />
					<span className="sr-only">Always on</span>
				</>
			) : (
				onRemoveMcp && (
					<BadgeDismissButton
						onClick={() => onRemoveMcp(badge.server.id)}
						ariaLabel={`Remove ${badge.server.display_name}`}
						isDisabled={isDisabled}
					/>
				)
			)}
		</span>
	);
};

export const AgentChatInput: React.FC<AgentChatInputProps> = ({
	onSend,
	placeholder = "Type a message...",
	isDisabled,
	isReadOnly = false,
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
	isWorkspaceLoading,
	queuedMessages = [],
	onDeleteQueuedMessage,
	onPromoteQueuedMessage,
	warning,
	readOnlyNotice,
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
	workspaceUploads,
	onTextPreview,
	mcpServers,
	selectedMCPServerIds,
	onMCPSelectionChange,
	onMCPAuthComplete,
	workspaceSkills,
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
	slashCommands,
	fillWidth = false,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const { organizations } = useDashboard();
	const chatOrganization = organizations.find(
		(organization) => organization.id === chatOrganizationId,
	);
	const showAgentSetupNotice =
		aiGatewayDisabled ||
		(canConfigureAgentSetup
			? providerCount !== undefined &&
				modelCount !== undefined &&
				(providerCount === 0 || modelCount === 0)
			: modelCount !== undefined && modelCount === 0);
	const { state, actions, meta } = useComposerRuntime(
		{
			onSend,
			isDisabled,
			isReadOnly,
			isLoading,
			hasModelOptions,
			warning,
			inputRef,
			initialValue,
			initialEditorState,
			remountKey,
			onContentChange,
			isStreaming,
			onInterrupt,
			isInterruptPending,
			isEditingHistoryMessage,
			onCancelHistoryEdit,
			userPromptHistory,
			files: {
				attachments,
				onAttach,
				onRemoveAttachment,
				uploadStates,
				previewUrls,
				textContents,
				workspaceUploads,
			},
			queuedMessages,
			onPromoteQueuedMessage,
		},
		showAgentSetupNotice,
	);
	const [plusMenuOpen, setPlusMenuOpen] = useState(false);
	const [plusMenuView, setPlusMenuView] = useState<"main" | "workspace">(
		"main",
	);
	const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false);
	const { connectingServerId: mcpConnectingId, connect: connectMCPServer } =
		useMCPOAuthFlow({
			organizationId: chatOrganizationId,
			onAuthComplete: onMCPAuthComplete,
			onFlowSuccess: (serverID) => {
				if (
					onMCPSelectionChange &&
					selectedMCPServerIds &&
					mcpServers?.some(
						(server) => server.id === serverID && server.enabled,
					) &&
					!selectedMCPServerIds.includes(serverID)
				) {
					onMCPSelectionChange([...selectedMCPServerIds, serverID]);
				}
			},
		});
	const [mcpDisconnectTarget, setMcpDisconnectTarget] =
		useState<TypesGen.MCPServerConfig | null>(null);
	const queryClient = useQueryClient();
	const mcpDisconnectMutation = useMutation(
		disconnectMCPServerOAuth2(queryClient),
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

	const handleMcpDisconnectConfirm = () => {
		if (!mcpDisconnectTarget) {
			return;
		}
		const name = mcpDisconnectTarget.display_name;
		mcpDisconnectMutation.mutate(mcpDisconnectTarget.id, {
			onSuccess: (response) => {
				setMcpDisconnectTarget(null);
				if (response.token_revocation_error) {
					toast.warning(`Disconnected ${name}.`, {
						description: response.token_revocation_error,
					});
				} else {
					toast.success(`Disconnected ${name}.`);
				}
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, `Failed to disconnect ${name}.`));
			},
		});
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

	let workspacePillBadge: ToolBadgeData | undefined;
	if (workspace && workspaceAgent && chatId) {
		workspacePillBadge = attachedWorkspace
			? { kind: "attached-workspace", ...attachedWorkspace }
			: { kind: "workspace", name: workspace.name };
	}

	// Ordered list of active tool badge data so we can determine
	// which ones ended up in the overflow popover.
	const allBadges: ToolBadgeData[] = [];
	if (shouldOverflowPlanningBadge) {
		allBadges.push({ kind: "planning" });
	}
	if (workspacePillBadge) {
		allBadges.push(workspacePillBadge);
	} else if (attachedWorkspace) {
		allBadges.push({ kind: "attached-workspace", ...attachedWorkspace });
	}
	if (shouldShowSelectedWorkspaceBadge && selectedWorkspace) {
		allBadges.push({ kind: "workspace", name: selectedWorkspace.name });
	}
	if (activeMcpServers.length >= 3) {
		allBadges.push({ kind: "mcp-group", servers: activeMcpServers });
	} else {
		for (const server of activeMcpServers) {
			allBadges.push({ kind: "mcp", server });
		}
	}

	const overflowCount = useOverflowCount(badgeContainerRef, allBadges.length);
	const visibleCount = Math.max(0, allBadges.length - overflowCount);
	const overflowBadges = allBadges.slice(visibleCount);

	const isReadOnlyView = readOnlyNotice !== undefined;
	const readOnlyModel = isReadOnlyView
		? modelOptions.find((option) => option.id === selectedModel)
		: undefined;
	const handleRemoveWorkspace = () => onWorkspaceChange?.(null);
	const removeWorkspaceHandler =
		onWorkspaceChange && !isReadOnlyView ? handleRemoveWorkspace : undefined;
	const handleRemoveMcp = isReadOnlyView
		? undefined
		: (serverId: string) => handleMcpToggle(serverId, false);

	const handlePlanModeToggle = () => {
		onPlanModeToggle(!planModeEnabled);
		setPlusMenuOpen(false);
	};

	const handleDisablePlanMode = isReadOnlyView
		? undefined
		: () => onPlanModeToggle(false);

	const fileInputRef = useRef<HTMLInputElement>(null);

	const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (e.target.files?.length) {
			actions.attachFiles(Array.from(e.target.files));
		}
		// Reset so the same file can be selected again.
		e.target.value = "";
	};

	const content = (
		<div
			className={cn(
				"mx-auto w-full",
				fillWidth
					? "max-w-full"
					: [chatWidthClass(chatFullWidth), "pb-0 sm:pb-4"],
				isEditingHistoryMessage && "pt-1",
			)}
		>
			{queuedMessages.length > 0 && (
				<QueuedMessagesList
					messages={queuedMessages}
					onDelete={(id) => onDeleteQueuedMessage?.(id)}
					onPromote={(id) => onPromoteQueuedMessage?.(id)}
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
			<AgentComposer.Frame
				className={
					isReadOnlyView ? "border border-solid border-border" : undefined
				}
			>
				<AgentComposer.Warning />
				{isEditingHistoryMessage && <AgentComposer.EditBanner />}
				<AgentComposer.Attachments onTextPreview={onTextPreview} />
				{isReadOnlyView ? (
					<div
						role="status"
						className="flex items-center gap-2 px-3 py-2 text-[13px] text-content-primary"
					>
						<InfoIcon className="size-4 shrink-0 text-content-link" />
						{readOnlyNotice}
					</div>
				) : (
					<AgentComposer.Editor
						placeholder={placeholder}
						hasWorkspace={hasSkillsWorkspace}
						organizationId={chatOrganizationId}
						workspaceSkills={workspaceSkills}
						slashCommands={slashCommands}
					/>
				)}
				<AgentComposer.InvisibleCharacterWarning />
				{/* Hidden file input for attaching files. When workspace
				uploads are wired, every file type stays selectable even
				while no workspace is ready: iOS silently greys out
				filtered types, so actions.attachFiles explains the refusal. */}
				{state.canAttachFiles && (
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
				<AgentComposer.Toolbar>
					{/* flex-1 routes free row space to the growing pills. */}
					<div className="flex min-w-0 flex-1 items-center gap-1">
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
									className={cn(
										"size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0",
										isReadOnlyView && "hidden",
									)}
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
										{state.canAttachFiles && (
											<button
												type="button"
												onClick={() => {
													actions.resetPromptCycle();
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
															{isForceOn && (
																<LockIcon className="size-3 shrink-0 text-content-secondary" />
															)}
															{needsAuth ? (
																<>
																	{isForceOn && (
																		<span className="sr-only">Always on</span>
																	)}
																	<Button
																		variant="outline"
																		size="sm"
																		className="h-6 shrink-0 px-2 text-[10px] leading-none"
																		onClick={() => connectMCPServer(server.id)}
																		disabled={
																			isDisabled || mcpConnectingId !== null
																		}
																	>
																		{isConnecting ? (
																			<Spinner
																				loading
																				className="h-2.5 w-2.5"
																			/>
																		) : null}
																		Auth
																	</Button>
																</>
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
																		aria-label={
																			isForceOn
																				? `${server.display_name} always on`
																				: `${isSelected ? "Disable" : "Enable"} ${server.display_name}`
																		}
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
						) : isReadOnlyView ? (
							<span className="inline-flex min-w-0 shrink items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary">
								{readOnlyModel && (
									<ProviderIcon
										provider={readOnlyModel.provider}
										icon={readOnlyModel.providerIcon}
										className="size-3 shrink-0"
									/>
								)}
								<span className="truncate">
									{readOnlyModel?.displayName ?? modelSelectorPlaceholder}
								</span>
							</span>
						) : (
							<ModelSelector
								value={selectedModel}
								onValueChange={onModelChange}
								options={modelOptions}
								disabled={isDisabled}
								placeholder={modelSelectorPlaceholder}
								className={cn(pillSizingClasses, "md:h-auto")}
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
								{handleDisablePlanMode && (
									<BadgeDismissButton
										onClick={handleDisablePlanMode}
										ariaLabel="Disable plan mode"
										isDisabled={isDisabled}
									/>
								)}
							</span>
						)}
						{/* Badges and the +N pill stay mounted for measurement:
						 * overflowed badges are display:none, the pill merely
						 * invisible so its width stays readable. */}
						<div
							ref={badgeContainerRef}
							className="flex min-w-0 items-center gap-1 overflow-hidden"
						>
							{allBadges.map((badge, i) => {
								const isOverflow = overflowCount > 0 && i >= visibleCount;
								if (
									badge === workspacePillBadge &&
									workspace &&
									workspaceAgent &&
									chatId
								) {
									return (
										<span
											key="workspace-pill"
											className={cn(
												"flex min-w-0 text-xs",
												pillSizingClasses,
												isOverflow && "hidden",
											)}
										>
											<WorkspacePill
												workspace={workspace}
												agent={workspaceAgent}
												chatId={chatId}
												sshCommand={sshCommand}
												folder={folder}
												onRemoveWorkspace={removeWorkspaceHandler}
											/>
										</span>
									);
								}
								return (
									<ToolBadge
										key={badgeKey(badge, i)}
										badge={badge}
										onRemoveWorkspace={removeWorkspaceHandler}
										onRemoveMcp={handleRemoveMcp}
										onRemovePlanning={handleDisablePlanMode}
										isDisabled={isDisabled}
										className={isOverflow ? "hidden" : undefined}
									/>
								);
							})}
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
								<BadgePopoverContent
									onInteractOutside={(event) => {
										// The workspace pill portals its menu outside
										// this popover; dismissing would unmount the
										// open menu. Ignore focus shifts and pointer
										// presses inside the menu.
										if (event.detail.originalEvent.type !== "pointerdown") {
											event.preventDefault();
											return;
										}
										if (
											event.target instanceof Element &&
											event.target.closest('[role="menu"]')
										) {
											event.preventDefault();
										}
									}}
								>
									{overflowBadges.map((badge, i) => {
										if (
											badge === workspacePillBadge &&
											workspace &&
											workspaceAgent &&
											chatId
										) {
											return (
												<span
													key="workspace-pill-overflow"
													className="flex min-w-0 text-xs"
												>
													<WorkspacePill
														workspace={workspace}
														agent={workspaceAgent}
														chatId={chatId}
														sshCommand={sshCommand}
														folder={folder}
														onRemoveWorkspace={removeWorkspaceHandler}
														inOverflowPopover
													/>
												</span>
											);
										}
										return (
											<ToolBadge
												key={badgeKey(badge, visibleCount + i)}
												badge={badge}
												onRemoveWorkspace={removeWorkspaceHandler}
												onRemoveMcp={handleRemoveMcp}
												onRemovePlanning={handleDisablePlanMode}
												isDisabled={isDisabled}
												disableTooltip
											/>
										);
									})}
								</BadgePopoverContent>
							</Popover>
						</div>
					</div>
					<div className="flex shrink-0 items-center gap-2">
						{!isReadOnlyView && <AgentComposer.VoiceInput />}
						{contextUsage !== undefined && (
							<div
								className={cn(
									"flex",
									state.speechSupported && !state.speechError && "-ml-2",
								)}
							>
								<ContextUsageIndicator
									usage={contextUsage}
									onRefreshContext={onRefreshContext}
									isRefreshingContext={isRefreshingContext}
								/>
							</div>
						)}
						{!isReadOnlyView && <AgentComposer.Submit />}
						{!isReadOnlyView && <AgentComposer.Stop />}
						<AgentComposer.InterruptStatus />
					</div>
				</AgentComposer.Toolbar>
			</AgentComposer.Frame>
		</div>
	);

	return (
		<>
			<AgentComposer.Provider state={state} actions={actions} meta={meta}>
				{content}
			</AgentComposer.Provider>
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
