import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	LockIcon,
	type LucideIcon,
	MonitorIcon,
	PaperclipIcon,
	PencilIcon,
	PlusIcon,
	ServerIcon,
	UnlinkIcon,
	XIcon,
	ZapIcon,
} from "lucide-react";
import type React from "react";
import { useId, useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { disconnectMCPServerOAuth2 } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { isBelowMdViewport } from "#/utils/mobile";
import { useMCPOAuthFlow } from "../hooks/useMCPOAuthFlow";
import { useOverflowCount } from "../hooks/useOverflowCount";
import { MCPServerIconStack } from "./MCPServerIconStack";
import { WorkspacePill } from "./WorkspacePill";

/** Display information for a workspace attached to the chat. */
export type AttachedWorkspaceInfo = {
	id: string;
	name: string;
	route: string;
	statusIcon: React.ReactNode;
	statusLabel: string;
};

type AgentComposerOptionsProps = {
	isDisabled: boolean;
	showAgentSetupNotice?: boolean;
	hasContextUsage?: boolean;
	onAttachClick?: () => void;
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	planModeEnabled: boolean;
	onPlanModeToggle: (enabled: boolean) => void;
	manageAutomationsEnabled?: boolean;
	onManageAutomationsToggle?: (enabled: boolean) => void;
	isModelCatalogLoading: boolean;
	workspaceOptions?: ReadonlyArray<{
		id: string;
		name: string;
		owner_name: string;
		organization_id: string;
	}>;
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (id: string | null) => void;
	chatOrganizationId?: string;
	isWorkspaceLoading?: boolean;
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
	onRemoveMcp: (serverId: string) => void;
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
	onRemoveMcp: (serverId: string) => void;
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
				<BadgeDismissButton
					onClick={() => onRemoveMcp(badge.server.id)}
					ariaLabel={`Remove ${badge.server.display_name}`}
					isDisabled={isDisabled}
				/>
			)}
		</span>
	);
};

type PlusMenuCheckboxItemProps = {
	icon: LucideIcon;
	label: string;
	description?: string;
	checked: boolean;
	onToggle: () => void;
	disabled: boolean;
};

const PlusMenuCheckboxItem: React.FC<PlusMenuCheckboxItemProps> = ({
	icon: Icon,
	label,
	description,
	checked,
	onToggle,
	disabled,
}) => {
	const id = useId();
	return (
		<button
			type="button"
			role="menuitemcheckbox"
			aria-checked={checked}
			aria-labelledby={`${id}-label`}
			aria-describedby={description ? `${id}-description` : undefined}
			onClick={onToggle}
			disabled={disabled}
			className={cn(
				"flex w-full cursor-pointer gap-1.5 border-none bg-transparent px-1 text-left text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50",
				description ? "items-start py-1.5" : "h-8 items-center",
			)}
		>
			<Icon className={cn("size-3.5 shrink-0", description && "mt-px")} />
			<span className="flex min-w-0 flex-col gap-0.5">
				<span id={`${id}-label`}>{label}</span>
				{description && (
					<span id={`${id}-description`} className="max-w-48 text-2xs">
						{description}
					</span>
				)}
			</span>
			{checked && <CheckIcon className="ml-auto size-icon-sm shrink-0" />}
		</button>
	);
};

/** Controlled composer options with local menu, OAuth, and overflow state. */
export const AgentComposerOptions: React.FC<AgentComposerOptionsProps> = ({
	isDisabled,
	showAgentSetupNotice = false,
	hasContextUsage = false,
	onAttachClick,
	selectedModel,
	onModelChange,
	modelOptions,
	modelSelectorPlaceholder,
	reasoningEffort,
	onReasoningEffortChange,
	planModeEnabled,
	onPlanModeToggle,
	manageAutomationsEnabled = false,
	onManageAutomationsToggle,
	isModelCatalogLoading,
	workspaceOptions,
	selectedWorkspaceId,
	onWorkspaceChange,
	chatOrganizationId,
	isWorkspaceLoading,
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
	const shouldOverflowPlanningBadge = planModeEnabled && hasContextUsage;

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

	const handleRemoveWorkspace = () => onWorkspaceChange?.(null);
	const removeWorkspaceHandler = onWorkspaceChange
		? handleRemoveWorkspace
		: undefined;
	const handleRemoveMcp = (serverId: string) =>
		handleMcpToggle(serverId, false);

	const handlePlanModeToggle = () => {
		onPlanModeToggle(!planModeEnabled);
		setPlusMenuOpen(false);
	};

	const handleDisablePlanMode = () => onPlanModeToggle(false);

	const handleManageAutomationsToggle = () => {
		onManageAutomationsToggle?.(!manageAutomationsEnabled);
		setPlusMenuOpen(false);
	};

	return (
		<>
			{/* flex-1 routes free row space to the growing pills. */}
			<div className="flex min-w-0 flex-1 items-center gap-1">
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
							className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
							disabled={
								isDisabled && !showAgentSetupNotice && !canUseWorkspacePicker
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
								{onAttachClick && (
									<button
										type="button"
										onClick={() => {
											setPlusMenuOpen(false);
											onAttachClick();
										}}
										className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
									>
										<PaperclipIcon className="size-3.5 shrink-0" />
										Attach file
									</button>
								)}
								<PlusMenuCheckboxItem
									icon={PencilIcon}
									label="Plan first"
									checked={planModeEnabled}
									onToggle={handlePlanModeToggle}
									disabled={isDisabled}
								/>
								{onManageAutomationsToggle && (
									<PlusMenuCheckboxItem
										icon={ZapIcon}
										label="Manage automations"
										description="Let the agent create and manage automations for you."
										checked={manageAutomationsEnabled}
										onToggle={handleManageAutomationsToggle}
										disabled={isDisabled}
									/>
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
												(selectedMCPServerIds?.includes(server.id) ?? false);
											const needsAuth =
												server.auth_type === "oauth2" && !server.auth_connected;
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
																	<Spinner loading className="h-2.5 w-2.5" />
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
						<BadgeDismissButton
							onClick={handleDisablePlanMode}
							ariaLabel="Disable plan mode"
							isDisabled={isDisabled}
						/>
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
			{/* Portaled dialog keys still bubble through the React composer tree. */}
			<div
				className="contents"
				role="presentation"
				onKeyDown={(event) => {
					if (event.key === "Escape") event.stopPropagation();
				}}
			>
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
			</div>
		</>
	);
};

/**
 * Workspaces from a different organization than the chat are disabled
 * unless already selected, so stale bindings can still be cleared.
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
