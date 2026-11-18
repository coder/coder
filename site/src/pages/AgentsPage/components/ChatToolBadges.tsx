import { cn } from "cn";
import {
	ChevronDownIcon,
	MonitorIcon,
	PencilIcon,
	PlugIcon,
	XIcon,
} from "lucide-react";
import { useRef, useState } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useOverflowCount } from "../hooks/useOverflowCount";
import { MCPServerIconStack } from "./MCPServerIconStack";
import { WorkspacePill } from "./WorkspacePill";

/** Workspace status available before the full workspace and agent load. */
export type AttachedWorkspaceInfo = {
	id: string;
	name: string;
	route: string;
	statusIcon: React.ReactNode;
	statusLabel: string;
};

const pillFocusRingClasses =
	"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link";

type ToolBadgeData =
	| { kind: "workspace"; name: string }
	| ({ kind: "attached-workspace" } & AttachedWorkspaceInfo)
	| {
			kind: "workspace-menu";
			workspace: TypesGen.Workspace;
			agent: TypesGen.WorkspaceAgent;
			chatId: string;
			folder?: string;
	  }
	| { kind: "mcp-group"; servers: readonly TypesGen.MCPServerConfig[] }
	| { kind: "planning" };

type ChatToolBadgesProps = {
	planModeEnabled: boolean;
	collapsePlanningBadge: boolean;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	chatId?: string;
	folder?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	selectedWorkspace?: Pick<TypesGen.Workspace, "id" | "name">;
	// Undefined hides the badge; an empty array offers "Add MCPs".
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	mcpMenu: React.ReactNode;
	onRemoveWorkspace?: () => void;
	onRemovePlanning?: () => void;
	isDisabled: boolean;
};

/** Chat metadata pills with optional editing actions and responsive overflow. */
export const ChatToolBadges: React.FC<ChatToolBadgesProps> = ({
	planModeEnabled,
	collapsePlanningBadge,
	workspace,
	workspaceAgent,
	chatId,
	folder,
	attachedWorkspace,
	selectedWorkspace,
	mcpServers,
	mcpMenu,
	onRemoveWorkspace,
	onRemovePlanning,
	isDisabled,
}) => {
	const badgeContainerRef = useRef<HTMLDivElement>(null);
	const [overflowPopoverOpen, setOverflowPopoverOpen] = useState(false);
	const linkedWorkspaceId = workspace?.id ?? attachedWorkspace?.id;
	const allBadges: ToolBadgeData[] = [];
	if (planModeEnabled && collapsePlanningBadge) {
		allBadges.push({ kind: "planning" });
	}
	if (workspace && workspaceAgent && chatId) {
		allBadges.push({
			kind: "workspace-menu",
			workspace,
			agent: workspaceAgent,
			chatId,
			folder,
		});
	} else if (attachedWorkspace) {
		allBadges.push({ kind: "attached-workspace", ...attachedWorkspace });
	}
	if (selectedWorkspace && selectedWorkspace.id !== linkedWorkspaceId) {
		allBadges.push({ kind: "workspace", name: selectedWorkspace.name });
	}
	if (mcpServers !== undefined) {
		allBadges.push({ kind: "mcp-group", servers: mcpServers });
	}

	const overflowCount = useOverflowCount(badgeContainerRef, allBadges.length);
	const visibleCount = Math.max(0, allBadges.length - overflowCount);
	const overflowBadges = allBadges.slice(visibleCount);

	return (
		<>
			{planModeEnabled && !collapsePlanningBadge && (
				<ToolBadge
					badge={{ kind: "planning" }}
					onRemovePlanning={onRemovePlanning}
					mcpMenu={mcpMenu}
					isDisabled={isDisabled}
					className="hidden sm:inline-flex"
				/>
			)}
			{/* Overflowed badges stay mounted but display:none; the +N pill
			 * stays measurable even when invisible. */}
			<div
				ref={badgeContainerRef}
				className="flex min-w-0 items-center gap-1 overflow-hidden"
			>
				{allBadges.map((badge, i) => (
					<ToolBadge
						key={badge.kind}
						badge={badge}
						onRemoveWorkspace={onRemoveWorkspace}
						onRemovePlanning={onRemovePlanning}
						mcpMenu={mcpMenu}
						isDisabled={isDisabled}
						className={
							overflowCount > 0 && i >= visibleCount ? "hidden" : undefined
						}
					/>
				))}
				<Popover
					open={overflowPopoverOpen && overflowCount > 0}
					onOpenChange={setOverflowPopoverOpen}
				>
					<PopoverTrigger asChild>
						<button
							type="button"
							className={cn(
								"inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary",
								pillFocusRingClasses,
								overflowCount === 0 && "invisible",
							)}
							aria-label={`+${overflowCount} more item${overflowCount !== 1 ? "s" : ""}`}
							aria-hidden={overflowCount === 0}
						>
							+{overflowCount}
						</button>
					</PopoverTrigger>
					<PopoverContent
						side="top"
						align="start"
						aria-label="More chat tools"
						// Clamp pills so long names cannot push dismiss buttons out of view.
						className="flex w-auto max-w-64 flex-wrap gap-1 p-2 *:max-w-full"
						onInteractOutside={(event) => {
							// Workspace menus are portaled outside this popover. Keep
							// it mounted while focus or pointer presses are inside them.
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
						{overflowBadges.map((badge) => (
							<ToolBadge
								key={badge.kind}
								badge={badge}
								onRemoveWorkspace={onRemoveWorkspace}
								onRemovePlanning={onRemovePlanning}
								mcpMenu={mcpMenu}
								isDisabled={isDisabled}
								inOverflowPopover
							/>
						))}
					</PopoverContent>
				</Popover>
			</div>
		</>
	);
};

const BadgeDismissButton: React.FC<{
	onClick: () => void;
	ariaLabel: string;
	isDisabled: boolean;
}> = ({ onClick, ariaLabel, isDisabled }) => (
	<button
		type="button"
		onClick={onClick}
		disabled={isDisabled}
		className="group -mx-1 -my-1 inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-full bg-transparent p-0 text-content-secondary focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50"
		aria-label={ariaLabel}
	>
		<span className="inline-flex size-3.5 items-center justify-center rounded-full transition-colors group-hover:bg-surface-tertiary group-hover:text-content-primary group-focus-visible:bg-surface-tertiary group-focus-visible:text-content-primary">
			<XIcon className="size-2.5!" />
		</span>
	</button>
);

type MCPGroupBadgeProps = {
	servers: readonly TypesGen.MCPServerConfig[];
	menu: React.ReactNode;
	className: string;
};

const MCPGroupBadge: React.FC<MCPGroupBadgeProps> = ({
	servers,
	menu,
	className,
}) => {
	const [open, setOpen] = useState(false);
	let label = `${servers.length} MCPs`;
	if (servers.length === 0) {
		label = "Add MCPs";
	} else if (servers.length === 1) {
		label = "1 MCP";
	}

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label={label}
					className={cn(
						className,
						"cursor-pointer transition-colors hover:bg-surface-tertiary hover:text-content-primary",
						pillFocusRingClasses,
					)}
				>
					{servers.length === 0 ? (
						<PlugIcon className="size-3 shrink-0" />
					) : (
						<MCPServerIconStack servers={servers} />
					)}
					{label}
					<ChevronDownIcon
						className={cn("size-3 transition-transform", open && "rotate-180")}
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				align="start"
				aria-label="MCP servers"
				className="w-72 p-1"
			>
				{menu}
			</PopoverContent>
		</Popover>
	);
};

const ToolBadge: React.FC<{
	badge: ToolBadgeData;
	onRemoveWorkspace?: () => void;
	onRemovePlanning?: () => void;
	mcpMenu: React.ReactNode;
	isDisabled: boolean;
	className?: string;
	// Overflow auto-focuses badges, so tooltips are suppressed there.
	inOverflowPopover?: boolean;
}> = ({
	badge,
	onRemoveWorkspace,
	onRemovePlanning,
	mcpMenu,
	isDisabled,
	className,
	inOverflowPopover,
}) => {
	const badgeCls = cn(
		"inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary",
		"has-focus-visible:ring-2 has-focus-visible:ring-inset has-focus-visible:ring-content-link",
		className,
	);

	if (badge.kind === "workspace-menu") {
		return (
			<span
				className={cn(
					"flex min-w-0 text-xs",
					!inOverflowPopover &&
						"grow shrink-0 basis-[calc(8ch_+_3.125rem)] max-w-max",
					className,
				)}
			>
				<WorkspacePill
					workspace={badge.workspace}
					agent={badge.agent}
					chatId={badge.chatId}
					folder={badge.folder}
					onRemoveWorkspace={onRemoveWorkspace}
					inOverflowPopover={inOverflowPopover}
				/>
			</span>
		);
	}

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
							className="inline-flex min-w-0 items-center gap-1 text-inherit no-underline focus-visible:outline-hidden"
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
				{!inOverflowPopover && (
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

	return (
		<MCPGroupBadge
			servers={badge.servers}
			menu={mcpMenu}
			className={badgeCls}
		/>
	);
};
