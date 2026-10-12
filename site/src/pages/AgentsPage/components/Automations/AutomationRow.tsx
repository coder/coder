import {
	EllipsisVerticalIcon,
	MessageSquareIcon,
	MessagesSquareIcon,
	PlayIcon,
} from "lucide-react";
import { memo, useRef } from "react";
import { Link as RouterLink } from "react-router";
import type { ChatAutomation } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Spinner } from "#/components/Spinner/Spinner";
import { TableCell, TableRow } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AutomationLastRun } from "./AutomationLastRun";
import {
	AutomationEnabledSwitch,
	AutomationNextRun,
	AutomationTriggerSummary,
	ChatTitleLink,
	isTargetMissing,
	runNowBlockedReason,
} from "./AutomationStatus";
import { automationPath } from "./automationsFlag";

type AutomationRowProps = {
	automation: ChatAutomation;
	isOwner: boolean;
	/** The owner and organization admins can delete an automation. */
	canDelete: boolean;
	/** Stacks the row into Name, Enabled and Actions cells for narrow screens. */
	compact: boolean;
	/** Adds the Owner cell. Wide layout only. */
	showOwner?: boolean;
	isUpdating: boolean;
	isRunning: boolean;
	/** One run request at a time, so a pending run disables every row. */
	isAnyRunPending: boolean;
	onToggleEnabled: (automation: ChatAutomation, enabled: boolean) => void;
	onRunNow: (automation: ChatAutomation) => void;
	onViewChats: (automation: ChatAutomation) => void;
	onEdit: (automation: ChatAutomation) => void;
	onDelete: (automation: ChatAutomation) => void;
};

type AutomationChatsButtonProps = {
	automation: ChatAutomation;
	/** Shows only the icon, with the label in a tooltip. */
	iconOnly: boolean;
	onViewChats: (automation: ChatAutomation) => void;
};

/** Puts an icon-only button's label in a tooltip. */
const withLabelTooltip = (
	button: React.ReactElement,
	label: string,
	iconOnly: boolean,
) =>
	iconOnly ? (
		<Tooltip>
			<TooltipTrigger asChild>{button}</TooltipTrigger>
			<TooltipContent>{label}</TooltipContent>
		</Tooltip>
	) : (
		button
	);

/**
 * An existing-chat automation always writes to one chat, so it links straight
 * to that chat. A new-chat automation creates many, so it opens their list.
 */
const AutomationChatsButton: React.FC<AutomationChatsButtonProps> = ({
	automation,
	iconOnly,
	onViewChats,
}) => {
	const size = iconOnly ? "icon" : "sm";
	if (automation.target_mode === "existing_chat") {
		const targetChat = automation.target_chat;
		if (!targetChat || isTargetMissing(automation)) {
			return null;
		}
		return withLabelTooltip(
			<Button
				asChild
				size={size}
				variant="outline"
				aria-label={`Open chat of ${automation.name}`}
			>
				<RouterLink to={`/agents/${targetChat.id}`}>
					<MessageSquareIcon />
					{!iconOnly && "Open chat"}
				</RouterLink>
			</Button>,
			"Open chat",
			iconOnly,
		);
	}
	return withLabelTooltip(
		<Button
			size={size}
			variant="outline"
			aria-label={`View chats of ${automation.name}`}
			onClick={() => onViewChats(automation)}
		>
			<MessagesSquareIcon />
			{!iconOnly && "View chats"}
		</Button>,
		"View chats",
		iconOnly,
	);
};

type RunNowButtonProps = {
	automation: ChatAutomation;
	/** Shows only the icon, with the label in a tooltip. */
	iconOnly: boolean;
	isRunning: boolean;
	isAnyRunPending: boolean;
	onRunNow: (automation: ChatAutomation) => void;
};

/**
 * Stays focusable while blocked, so keyboard users can read the tooltip
 * that says why the server would refuse the run.
 */
const RunNowButton: React.FC<RunNowButtonProps> = ({
	automation,
	iconOnly,
	isRunning,
	isAnyRunPending,
	onRunNow,
}) => {
	const blockedReason = runNowBlockedReason(automation);
	const button = (
		<Button
			size={iconOnly ? "icon" : "sm"}
			variant="outline"
			disabled={isAnyRunPending}
			aria-disabled={blockedReason !== undefined}
			aria-label={`Run now ${automation.name}`}
			className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
			onClick={() => {
				if (!blockedReason) {
					onRunNow(automation);
				}
			}}
		>
			<Spinner loading={isRunning}>
				<PlayIcon />
			</Spinner>
			{!iconOnly && "Run now"}
		</Button>
	);
	if (!blockedReason) {
		return withLabelTooltip(button, "Run now", iconOnly);
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>{button}</TooltipTrigger>
			<TooltipContent className="max-w-xs">{blockedReason}</TooltipContent>
		</Tooltip>
	);
};

type TargetCellProps = {
	automation: ChatAutomation;
	isOwner: boolean;
};

const TargetCell: React.FC<TargetCellProps> = ({ automation, isOwner }) => {
	if (automation.target_mode === "new_chat") {
		return <span className="text-content-secondary">New chat each run</span>;
	}
	if (isTargetMissing(automation)) {
		return (
			<Badge variant="warning" size="sm">
				Missing target
			</Badge>
		);
	}
	// The API includes the chat only when the viewer can open it.
	if (!automation.target_chat) {
		return (
			<span className="text-content-secondary">
				{isOwner ? "Unavailable chat" : "A chat you cannot open"}
			</span>
		);
	}
	return (
		<ChatTitleLink
			chatId={automation.target_chat.id}
			title={automation.target_chat.title}
			size="sm"
		/>
	);
};

type NameDetailsProps = {
	automation: ChatAutomation;
	/** Hides the "Owned by" line, for the owner or when an Owner column says it. */
	hideOwner: boolean;
};

const NameDetails: React.FC<NameDetailsProps> = ({ automation, hideOwner }) => {
	const owner = automation.owner;
	return (
		<>
			{/* A link, so the page opens with a middle-click or in a new tab. */}
			<RouterLink
				to={automationPath(automation.id)}
				className="font-medium text-content-primary no-underline wrap-anywhere hover:underline"
			>
				{automation.name}
			</RouterLink>
			{!hideOwner && (
				<span className="text-xs text-content-secondary">
					{owner
						? `Owned by ${owner.name || owner.username}`
						: "Owner unavailable"}
				</span>
			)}
			{automation.created_by_chat &&
			automation.created_by_chat.id === automation.target_chat_id ? (
				// The Runs in column links the same chat, so name it only once.
				<span className="text-xs text-content-secondary">
					Created by the agent in this chat
				</span>
			) : automation.created_by_chat ? (
				// Inline text, not flex, so a long title wraps as one sentence.
				<span className="text-xs text-content-secondary">
					Created by the agent in{" "}
					<ChatTitleLink
						chatId={automation.created_by_chat.id}
						title={automation.created_by_chat.title}
						size="sm"
						className="inline"
					/>
				</span>
			) : (
				automation.created_by_chat_id && (
					<span className="text-xs text-content-secondary">
						Created by the agent
					</span>
				)
			)}
		</>
	);
};

type OwnerCellProps = {
	owner: ChatAutomation["owner"];
};

const OwnerCell: React.FC<OwnerCellProps> = ({ owner }) => {
	if (!owner) {
		return (
			<span className="text-xs text-content-secondary">Owner unavailable</span>
		);
	}
	const ownerName = owner.name || owner.username;
	return (
		<span className="flex min-w-0 max-w-40 items-center gap-2">
			<Avatar size="sm" src={owner.avatar_url} fallback={ownerName} />
			<span className="truncate" title={ownerName}>
				{ownerName}
			</span>
		</span>
	);
};

// memo() is allowed for list items under the React Compiler; it keeps
// other rows from re-rendering while one row's mutation is pending.
export const AutomationRow = memo<AutomationRowProps>(
	({
		automation,
		isOwner,
		canDelete,
		compact,
		showOwner = false,
		isUpdating,
		isRunning,
		isAnyRunPending,
		onToggleEnabled,
		onRunNow,
		onViewChats,
		onEdit,
		onDelete,
	}) => {
		const moreButtonRef = useRef<HTMLButtonElement>(null);
		// Runs once the menu closed and focused its trigger, so the dialog
		// the action opens returns focus there instead of to a removed item.
		const pendingActionRef = useRef<(() => void) | null>(null);
		const enabledSwitch = (
			<AutomationEnabledSwitch
				automation={automation}
				isOwner={isOwner}
				isUpdating={isUpdating}
				onToggle={(enabled) => onToggleEnabled(automation, enabled)}
			/>
		);
		const actions = (
			<div
				className={
					compact
						? "flex flex-col items-end gap-2"
						: "flex items-center justify-end gap-2"
				}
			>
				{automation.kind === "schedule" && isOwner && (
					<RunNowButton
						automation={automation}
						iconOnly={showOwner}
						isRunning={isRunning}
						isAnyRunPending={isAnyRunPending}
						onRunNow={onRunNow}
					/>
				)}
				{isOwner && (
					<AutomationChatsButton
						automation={automation}
						iconOnly={showOwner}
						onViewChats={onViewChats}
					/>
				)}
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							ref={moreButtonRef}
							size="icon"
							variant="subtle"
							aria-label={`More actions for ${automation.name}`}
						>
							<EllipsisVerticalIcon aria-hidden />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="end"
						onCloseAutoFocus={(event) => {
							const action = pendingActionRef.current;
							pendingActionRef.current = null;
							if (action) {
								event.preventDefault();
								moreButtonRef.current?.focus();
								action();
							}
						}}
					>
						<DropdownMenuItem
							onSelect={() => {
								pendingActionRef.current = () => onEdit(automation);
							}}
						>
							{isOwner ? "Edit" : "View"}
						</DropdownMenuItem>
						{canDelete && (
							<>
								<DropdownMenuSeparator />
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onSelect={() => {
										pendingActionRef.current = () => onDelete(automation);
									}}
								>
									Delete&hellip;
								</DropdownMenuItem>
							</>
						)}
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
		);

		if (compact) {
			return (
				<TableRow>
					<TableCell className="align-top">
						<div className="flex flex-col gap-1 wrap-anywhere">
							<NameDetails automation={automation} hideOwner={isOwner} />
							{/* The Trigger and Runs in headers are hidden in this layout. */}
							<div>
								<span className="sr-only">Trigger: </span>
								<AutomationTriggerSummary automation={automation} />
							</div>
							<div>
								<span className="sr-only">Runs in: </span>
								<TargetCell automation={automation} isOwner={isOwner} />
							</div>
							<span>
								Next: <AutomationNextRun automation={automation} />
							</span>
							<AutomationLastRun lastRun={automation.last_run} />
						</div>
					</TableCell>
					<TableCell className="align-top">{enabledSwitch}</TableCell>
					<TableCell className="align-top">{actions}</TableCell>
				</TableRow>
			);
		}

		return (
			<TableRow>
				{/* A minimum width, so a narrow table does not break the name
				    every few characters. */}
				<TableCell className="min-w-32">
					<div className="flex flex-col gap-0.5">
						<NameDetails
							automation={automation}
							hideOwner={isOwner || showOwner}
						/>
					</div>
				</TableCell>
				{showOwner && (
					<TableCell>
						<OwnerCell owner={automation.owner} />
					</TableCell>
				)}
				<TableCell>
					<AutomationTriggerSummary automation={automation} />
				</TableCell>
				<TableCell>
					<TargetCell automation={automation} isOwner={isOwner} />
				</TableCell>
				<TableCell>
					{/* Paused reasons come first: while paused, the last run is
					    an older result. */}
					<div className="flex flex-col gap-1">
						<span className="whitespace-nowrap">
							<AutomationNextRun automation={automation} />
						</span>
						<AutomationLastRun lastRun={automation.last_run} />
					</div>
				</TableCell>
				<TableCell>{enabledSwitch}</TableCell>
				<TableCell>{actions}</TableCell>
			</TableRow>
		);
	},
);
