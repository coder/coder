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
import type { MCPServerConfig } from "#/api/typesGenerated";
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

export type ToolBadgeData =
	| { kind: "workspace"; name: string }
	| ({ kind: "attached-workspace" } & AttachedWorkspaceInfo)
	| { kind: "mcp"; server: MCPServerConfig }
	| { kind: "mcp-group"; servers: readonly MCPServerConfig[] }
	| { kind: "planning" };

// flex-basis sets an 8ch floor, grow uses free row space, and max-w-max
// caps at the natural label width. Below the floor, +N overflow takes over.
export const composerPillSizingClasses =
	"grow shrink-0 basis-[calc(8ch_+_3.125rem)] max-w-max";

type BadgeActions = {
	onRemoveWorkspace?: () => void;
	onRemoveMcp: (serverId: string) => void;
	onRemovePlanning?: () => void;
	isDisabled: boolean;
};

/** Workspace badge data paired with the linked workspace's interactive pill. */
export type WorkspacePillBadge = {
	badge: Extract<ToolBadgeData, { kind: "workspace" | "attached-workspace" }>;
	props: Omit<
		React.ComponentProps<typeof WorkspacePill>,
		"onRemoveWorkspace" | "inOverflowPopover"
	>;
};

// Non-MCP badges can share a kind, so their keys are position-qualified.
const badgeKey = (badge: ToolBadgeData, index: number) => {
	if (badge.kind === "mcp") {
		return badge.server.id;
	}

	if (badge.kind === "mcp-group") {
		return badge.kind;
	}

	return `${badge.kind}-${index}`;
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

/** Measures the ordered badge row and renders trailing badges in its overflow menu. */
export const AgentComposerBadges = ({
	badges,
	workspacePill,
	...actions
}: BadgeActions & {
	badges: readonly ToolBadgeData[];
	workspacePill?: WorkspacePillBadge;
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const [open, setOpen] = useState(false);

	const overflowCount = useOverflowCount(containerRef, badges.length);
	const visibleCount = Math.max(0, badges.length - overflowCount);

	return (
		// Keep badges mounted for measurement; the invisible +N pill reserves
		// measurable width even when no badges overflow.
		<div
			ref={containerRef}
			className="flex min-w-0 items-center gap-1 overflow-hidden"
		>
			{badges.map((badge, index) => (
				<ComposerBadge
					key={
						badge === workspacePill?.badge
							? "workspace-pill"
							: badgeKey(badge, index)
					}
					badge={badge}
					workspacePill={workspacePill}
					{...actions}
					hidden={overflowCount > 0 && index >= visibleCount}
				/>
			))}
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
						tabIndex={overflowCount === 0 ? -1 : undefined}
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
							key={
								badge === workspacePill?.badge
									? "workspace-pill-overflow"
									: badgeKey(badge, visibleCount + index)
							}
							badge={badge}
							workspacePill={workspacePill}
							{...actions}
							inOverflowPopover
						/>
					))}
				</BadgePopoverContent>
			</Popover>
		</div>
	);
};

const ComposerBadge = ({
	badge,
	workspacePill,
	hidden,
	inOverflowPopover = false,
	...actions
}: BadgeActions & {
	badge: ToolBadgeData;
	workspacePill?: WorkspacePillBadge;
	hidden?: boolean;
	inOverflowPopover?: boolean;
}) => {
	if (workspacePill && badge === workspacePill.badge) {
		return (
			<span
				className={cn(
					"flex min-w-0 text-xs",
					!inOverflowPopover && composerPillSizingClasses,
					hidden && "hidden",
				)}
			>
				<WorkspacePill
					{...workspacePill.props}
					onRemoveWorkspace={actions.onRemoveWorkspace}
					inOverflowPopover={inOverflowPopover}
				/>
			</span>
		);
	}

	return (
		<ToolBadge
			badge={badge}
			{...actions}
			className={hidden ? "hidden" : undefined}
			disableTooltip={inOverflowPopover}
		/>
	);
};

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
	onRemove,
	isDisabled,
}: {
	onRemove: () => void;
	isDisabled: boolean;
}) => (
	<span
		data-testid="planning-badge"
		className="hidden shrink-0 items-center gap-1 rounded-full bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary sm:inline-flex"
	>
		<PencilIcon className="size-3" />
		Planning
		<BadgeDismissButton
			onClick={onRemove}
			ariaLabel="Disable plan mode"
			isDisabled={isDisabled}
		/>
	</span>
);

const MCPGroupBadge = ({
	servers,
	onRemoveMcp,
	isDisabled,
	className,
}: { servers: readonly MCPServerConfig[]; className: string } & Pick<
	BadgeActions,
	"onRemoveMcp" | "isDisabled"
>) => {
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

const ToolBadge = ({
	badge,
	onRemoveWorkspace,
	onRemoveMcp,
	onRemovePlanning,
	isDisabled,
	className,
	disableTooltip,
}: BadgeActions & {
	badge: ToolBadgeData;
	className?: string;
	// Overflow popovers auto-focus badges; suppress the tooltip there.
	disableTooltip?: boolean;
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
				{/* Touch focus would stick the tooltip open below md. */}
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
			{badge.server.availability === "force_on" ? (
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
