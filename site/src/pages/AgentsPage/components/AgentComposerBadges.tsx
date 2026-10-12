import { cn } from "cn";
import {
	ChevronDownIcon,
	LockIcon,
	MonitorIcon,
	PencilIcon,
	ServerIcon,
	XIcon,
} from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import { Link } from "react-router";
import type {
	MCPServerConfig,
	Workspace,
	WorkspaceAgent,
} from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	type PopoverContentProps,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useOverflowCount } from "../hooks/useOverflowCount";
import { useAgentComposer } from "./AgentComposer/context";
import {
	enabledMcpServers,
	isMCPServerSelected,
	mcpServerNeedsAuth,
	setMCPServerSelected,
} from "./AgentComposerMCPMenu";
import {
	type AgentComposerOptionsData,
	useAgentComposerOptions,
} from "./AgentComposerOptionsContext";
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

type ToolBadgeData =
	| { kind: "planning" }
	| {
			kind: "linked-workspace";
			workspace: Workspace;
			agent: WorkspaceAgent;
			chatId: string;
			sshCommand?: string;
			folder?: string;
	  }
	| ({ kind: "attached-workspace" } & AttachedWorkspaceInfo)
	| { kind: "workspace"; name: string }
	| { kind: "mcp"; server: MCPServerConfig }
	| { kind: "mcp-group"; servers: readonly MCPServerConfig[] };

/** Where a badge renders: in the measured row, hidden there after overflowing, or in a popover. */
type BadgePlacement = "row" | "row-hidden" | "popover";

// flex-basis sets an 8ch floor, grow uses free row space, and max-w-max
// caps at the natural label width. Below the floor, +N overflow takes over.
export const composerPillSizingClasses =
	"grow shrink-0 basis-[calc(8ch_+_3.125rem)] max-w-max";

// Non-MCP badges can share a kind, so their keys are position-qualified.
// The linked workspace pill keeps one key per container.
const badgeKey = (
	badge: ToolBadgeData,
	index: number,
	placement: BadgePlacement,
) => {
	switch (badge.kind) {
		case "linked-workspace":
			return placement === "popover"
				? "workspace-pill-overflow"
				: "workspace-pill";
		case "mcp":
			return badge.server.id;
		case "mcp-group":
			return badge.kind;
		default:
			return `${badge.kind}-${index}`;
	}
};

const rowPlacement = (index: number, visibleCount: number): BadgePlacement => {
	if (index < visibleCount) {
		return "row";
	}

	return "row-hidden";
};

// Ordering controls which trailing badges move into the overflow menu.
const toolBadges = (
	{
		planning,
		linkedWorkspace,
		workspaceSelection,
		mcp,
	}: AgentComposerOptionsData,
	includePlanning: boolean,
): ToolBadgeData[] => {
	const { workspace, agent, chatId, sshCommand, folder, attachedWorkspace } =
		linkedWorkspace ?? {};
	const badges: ToolBadgeData[] = [];

	if (includePlanning && planning.enabled) {
		badges.push({ kind: "planning" });
	}

	if (workspace && agent && chatId) {
		badges.push({
			kind: "linked-workspace",
			workspace,
			agent,
			chatId,
			sshCommand,
			folder,
		});
	} else if (attachedWorkspace) {
		badges.push({ kind: "attached-workspace", ...attachedWorkspace });
	}

	const selectedWorkspace = workspaceSelection?.options?.find(
		(item) => item.id === workspaceSelection.selectedId,
	);
	const linkedWorkspaceId = workspace?.id ?? attachedWorkspace?.id;
	if (selectedWorkspace && selectedWorkspace.id !== linkedWorkspaceId) {
		badges.push({ kind: "workspace", name: selectedWorkspace.name });
	}

	if (!mcp) {
		return badges;
	}

	const activeMcpServers = enabledMcpServers(mcp).filter(
		(server) => isMCPServerSelected(mcp, server) && !mcpServerNeedsAuth(server),
	);
	if (activeMcpServers.length >= 3) {
		badges.push({ kind: "mcp-group", servers: activeMcpServers });
	} else {
		for (const server of activeMcpServers) {
			badges.push({ kind: "mcp", server });
		}
	}

	return badges;
};

// Clamp pills to the popover width so a long name cannot push its X out of view.
const BadgePopoverContent = ({ className, ...props }: PopoverContentProps) => (
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

/**
 * Measures the ordered badge row and renders trailing badges in its overflow
 * menu. With `includePlanning`, the planning badge leads the measured row and
 * can overflow; otherwise compose AgentComposerPlanningBadge beside the row.
 */
export const AgentComposerBadges = ({
	includePlanning = false,
}: {
	includePlanning?: boolean;
}) => {
	const options = useAgentComposerOptions();
	const badges = toolBadges(options, includePlanning);
	const containerRef = useRef<HTMLDivElement>(null);
	const [open, setOpen] = useState(false);

	const overflowCount = useOverflowCount(containerRef, badges.length);
	const visibleCount = Math.max(0, badges.length - overflowCount);

	return (
		// Keep badges mounted for measurement: overflowed badges are
		// display:none, and the +N pill is merely invisible so its width
		// stays readable.
		<div
			ref={containerRef}
			className="flex min-w-0 items-center gap-1 overflow-hidden"
		>
			{badges.map((badge, index) => {
				const placement = rowPlacement(index, visibleCount);

				return (
					<ComposerBadge
						key={badgeKey(badge, index, placement)}
						badge={badge}
						placement={placement}
					/>
				);
			})}
			<Popover open={open && overflowCount > 0} onOpenChange={setOpen}>
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
						// Workspace menus portal outside this popover. Dismissing on
						// focus shifts or presses in that menu would unmount it.
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
					{badges.slice(visibleCount).map((badge, index) => (
						<ComposerBadge
							key={badgeKey(badge, visibleCount + index, "popover")}
							badge={badge}
							placement="popover"
						/>
					))}
				</BadgePopoverContent>
			</Popover>
		</div>
	);
};

// Small `X` button inside pill-style badges that dismisses or disables the
// badge without opening the `+` menu.
const BadgeDismissButton = ({
	onClick,
	ariaLabel,
	isDisabled = false,
}: {
	onClick: () => void;
	ariaLabel: string;
	isDisabled?: boolean;
}) => (
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

/** Planning badge composed separately from the measured row. */
export const AgentComposerPlanningBadge = ({
	className = "hidden sm:inline-flex",
}: {
	className?: string;
}) => {
	const { planning, isViewOnly } = useAgentComposerOptions();
	const { state } = useAgentComposer();

	if (!planning.enabled) {
		return null;
	}

	return (
		<span
			data-testid="planning-badge"
			className={cn(
				"shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
				className,
			)}
		>
			<PencilIcon className="size-3" />
			Planning
			{!isViewOnly && (
				<BadgeDismissButton
					onClick={() => planning.onChange(false)}
					ariaLabel="Disable plan mode"
					isDisabled={state.isDisabled}
				/>
			)}
		</span>
	);
};

const MCPGroupBadge = ({
	servers,
	className,
}: {
	servers: readonly MCPServerConfig[];
	className: string;
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
					<ComposerBadge
						key={server.id}
						badge={{ kind: "mcp", server }}
						placement="popover"
					/>
				))}
			</BadgePopoverContent>
		</Popover>
	);
};

const ComposerBadge = ({
	badge,
	placement,
}: {
	badge: ToolBadgeData;
	placement: BadgePlacement;
}) => {
	const { workspaceSelection, mcp, isViewOnly, changeWorkspace } =
		useAgentComposerOptions();
	const { state } = useAgentComposer();
	const isDisabled = state.isDisabled;
	const inPopover = placement === "popover";
	const isRowHidden = placement === "row-hidden";

	// Badges keep the workspace X while a change is offered and disable it
	// only while workspace selection loads, not while the composer is
	// disabled.
	const isWorkspaceRemovalOffered =
		workspaceSelection?.onChange !== undefined && !isViewOnly;
	const canChangeWorkspace = changeWorkspace !== undefined;
	const removeWorkspace = () => changeWorkspace?.(null);
	let removeLinkedWorkspace: (() => void) | undefined;

	if (isWorkspaceRemovalOffered && canChangeWorkspace) {
		removeLinkedWorkspace = removeWorkspace;
	}

	if (badge.kind === "linked-workspace") {
		return (
			<span
				className={cn(
					"flex min-w-0 text-xs",
					!inPopover && composerPillSizingClasses,
					isRowHidden && "hidden",
				)}
			>
				<WorkspacePill
					workspace={badge.workspace}
					agent={badge.agent}
					chatId={badge.chatId}
					sshCommand={badge.sshCommand}
					folder={badge.folder}
					onRemoveWorkspace={removeLinkedWorkspace}
					inOverflowPopover={inPopover}
				/>
			</span>
		);
	}

	const badgeCls = cn(
		"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
		isRowHidden && "hidden",
	);

	if (badge.kind === "planning") {
		return <AgentComposerPlanningBadge className={badgeCls} />;
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
						{isWorkspaceRemovalOffered && (
							<BadgeDismissButton
								onClick={removeWorkspace}
								ariaLabel={`Remove workspace ${badge.name}`}
								isDisabled={!canChangeWorkspace}
							/>
						)}
					</span>
				</TooltipTrigger>
				{/* Touch focus would stick the tooltip open below md. The
				overflow popover auto-focuses badges, so it omits the tooltip. */}
				{!inPopover && (
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
				{isWorkspaceRemovalOffered && (
					<BadgeDismissButton
						onClick={removeWorkspace}
						ariaLabel={`Remove workspace ${badge.name}`}
						isDisabled={!canChangeWorkspace}
					/>
				)}
			</span>
		);
	}

	if (badge.kind === "mcp-group") {
		return <MCPGroupBadge servers={badge.servers} className={badgeCls} />;
	}

	const isForceOn = badge.server.availability === "force_on";
	const canRemoveServer = !isForceOn && !isViewOnly;
	const removeServer = () => {
		if (mcp) {
			setMCPServerSelected(mcp, badge.server.id, false);
		}
	};

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
			{isForceOn && (
				<>
					<LockIcon className="size-3 shrink-0" />
					<span className="sr-only">Always on</span>
				</>
			)}
			{canRemoveServer && (
				<BadgeDismissButton
					onClick={removeServer}
					ariaLabel={`Remove ${badge.server.display_name}`}
					isDisabled={isDisabled}
				/>
			)}
		</span>
	);
};
