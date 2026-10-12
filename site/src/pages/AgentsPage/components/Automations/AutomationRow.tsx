import {
	EllipsisVerticalIcon,
	MessageSquareIcon,
	MessagesSquareIcon,
	PlayIcon,
} from "lucide-react";
import { memo, useRef } from "react";
import { Link as RouterLink } from "react-router";
import type { ChatAutomation } from "#/api/typesGenerated";
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
	AutomationEnabledSwitch,
	AutomationNextRun,
	AutomationTriggerSummary,
	ChatTitleLink,
} from "./AutomationStatus";

type AutomationRowProps = {
	automation: ChatAutomation;
	isOwner: boolean;
	/** Stacks the row into Name, Enabled and Actions cells for narrow screens. */
	compact: boolean;
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

/** True when an existing-chat automation has no chat it can run in. */
export const isTargetMissing = (automation: ChatAutomation) =>
	automation.target_mode === "existing_chat" &&
	(!automation.target_chat_id ||
		Boolean(automation.paused_reasons?.includes("target_unavailable")));

type AutomationChatsButtonProps = {
	automation: ChatAutomation;
	onViewChats: (automation: ChatAutomation) => void;
};

/**
 * An existing-chat automation always writes to one chat, so it links straight
 * to that chat. A new-chat automation creates many, so it opens their list.
 */
export const AutomationChatsButton: React.FC<AutomationChatsButtonProps> = ({
	automation,
	onViewChats,
}) => {
	if (automation.target_mode === "existing_chat") {
		const targetChat = automation.target_chat;
		if (!targetChat || isTargetMissing(automation)) {
			return null;
		}
		return (
			<Button
				asChild
				size="sm"
				variant="outline"
				aria-label={`Open chat of ${automation.name}`}
			>
				<RouterLink to={`/agents/${targetChat.id}`}>
					<MessageSquareIcon />
					Open chat
				</RouterLink>
			</Button>
		);
	}
	return (
		<Button
			size="sm"
			variant="outline"
			aria-label={`View chats of ${automation.name}`}
			onClick={() => onViewChats(automation)}
		>
			<MessagesSquareIcon />
			View chats
		</Button>
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
				{isOwner ? "Unavailable chat" : "Existing chat"}
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
	isOwner: boolean;
};

const NameDetails: React.FC<NameDetailsProps> = ({ automation, isOwner }) => {
	const owner = automation.owner;
	return (
		<>
			<span className="font-medium text-content-primary wrap-anywhere">
				{automation.name}
			</span>
			{!isOwner && (
				<span className="text-xs text-content-secondary">
					{owner
						? `Owned by ${owner.name || owner.username}`
						: "Owner unavailable"}
				</span>
			)}
			{automation.created_by_chat ? (
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

// memo() is allowed for list items under the React Compiler; it keeps
// other rows from re-rendering while one row's mutation is pending.
export const AutomationRow = memo<AutomationRowProps>(
	({
		automation,
		isOwner,
		compact,
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
					<Button
						size="sm"
						variant="outline"
						disabled={
							!automation.enabled ||
							Boolean(automation.paused_reasons?.length) ||
							isTargetMissing(automation) ||
							isAnyRunPending
						}
						aria-label={`Run now ${automation.name}`}
						onClick={() => onRunNow(automation)}
					>
						<Spinner loading={isRunning}>
							<PlayIcon />
						</Spinner>
						Run now
					</Button>
				)}
				{isOwner && (
					<AutomationChatsButton
						automation={automation}
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
						{isOwner && (
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
							<NameDetails automation={automation} isOwner={isOwner} />
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
						</div>
					</TableCell>
					<TableCell className="align-top">{enabledSwitch}</TableCell>
					<TableCell className="align-top">{actions}</TableCell>
				</TableRow>
			);
		}

		return (
			<TableRow>
				<TableCell>
					<div className="flex flex-col gap-0.5">
						<NameDetails automation={automation} isOwner={isOwner} />
					</div>
				</TableCell>
				<TableCell>
					<AutomationTriggerSummary automation={automation} />
				</TableCell>
				<TableCell>
					<TargetCell automation={automation} isOwner={isOwner} />
				</TableCell>
				<TableCell className="whitespace-nowrap">
					<AutomationNextRun automation={automation} />
				</TableCell>
				<TableCell>{enabledSwitch}</TableCell>
				<TableCell>{actions}</TableCell>
			</TableRow>
		);
	},
);
