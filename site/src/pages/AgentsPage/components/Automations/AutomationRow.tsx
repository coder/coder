import { PlayIcon, Trash2Icon } from "lucide-react";
import { memo } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import { chat } from "#/api/queries/chats";
import { organizationMember } from "#/api/queries/organizations";
import type { Chat, ChatAutomation } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link, type LinkProps } from "#/components/Link/Link";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { TableCell, TableRow } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useTime } from "#/hooks/useTime";
import { formatDate } from "#/utils/time";

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

const formatNextRun = (automation: ChatAutomation, now: number): string => {
	if (automation.kind !== "schedule") {
		return "Not scheduled";
	}
	// The server computed the list when it last loaded, so its first entry
	// can already be in the past.
	const date = automation.next_run_times
		.map((runTime) => new Date(runTime))
		.find((runTime) => runTime.getTime() > now);
	if (!date) {
		return "Not scheduled";
	}
	return formatDate(date, {
		locale: "en-US",
		timeZone: automation.schedule_time_zone || "UTC",
		timeZoneName: "short",
		month: "short",
		day: "numeric",
		year: undefined,
		hour: "numeric",
		minute: "2-digit",
		second: undefined,
	});
};

type NextRunProps = {
	automation: ChatAutomation;
};

const NextRun: React.FC<NextRunProps> = ({ automation }) => {
	// Re-checks every second, so a run that just started drops out at once
	// and an updated automation shows within a second. Re-renders happen
	// only when the label changes.
	return useTime(() => formatNextRun(automation, Date.now()), {
		interval: 1_000,
		disabled: automation.kind !== "schedule",
	});
};

type ChatTitleLinkProps = {
	chatId: string;
	title: string | undefined;
	size?: LinkProps["size"];
	className?: string;
};

/** Links to a chat by its title, falling back to "Untitled". */
export const ChatTitleLink: React.FC<ChatTitleLinkProps> = ({
	chatId,
	title,
	size = "lg",
	className,
}) => {
	return (
		<Link asChild showExternalIcon={false} size={size} className={className}>
			<RouterLink to={`/agents/${chatId}`}>{title || "Untitled"}</RouterLink>
		</Link>
	);
};

type ChatTitleProps = {
	chat: Chat | undefined;
	chatId: string;
	isLoading: boolean;
	error: unknown;
	size?: LinkProps["size"];
	className?: string;
};

const ChatTitle: React.FC<ChatTitleProps> = ({
	chat,
	chatId,
	isLoading,
	error,
	size,
	className,
}) => {
	if (isLoading) {
		return <Skeleton className="inline-block h-4 w-32 align-middle" />;
	}
	if (error) {
		return (
			<span className="text-content-secondary">
				{getErrorMessage(error, "Chat unavailable")}
			</span>
		);
	}
	return (
		<ChatTitleLink
			chatId={chatId}
			title={chat?.title}
			size={size}
			className={className}
		/>
	);
};

type CreatingChatProps = {
	chatId: string;
};

// Inline text, not flex, so a long title wraps as part of one sentence.
const CreatingChat: React.FC<CreatingChatProps> = ({ chatId }) => {
	const chatQuery = useQuery(chat(chatId));
	return (
		<span className="text-xs text-content-secondary">
			Created by the agent in{" "}
			{getErrorStatus(chatQuery.error) === 404 ? (
				"a deleted chat"
			) : (
				<ChatTitle
					chat={chatQuery.data}
					chatId={chatId}
					isLoading={chatQuery.isLoading}
					error={chatQuery.error}
					size="sm"
					className="inline"
				/>
			)}
		</span>
	);
};

type OwnerNameProps = {
	organizationId: string;
	userId: string;
};

const OwnerName: React.FC<OwnerNameProps> = ({ organizationId, userId }) => {
	const memberQuery = useQuery(organizationMember(organizationId, userId));
	if (memberQuery.isLoading) {
		return <Skeleton className="h-3 w-24" />;
	}
	if (!memberQuery.data) {
		return (
			<span className="text-xs text-content-secondary">Owner unavailable</span>
		);
	}
	return (
		<span className="text-xs text-content-secondary">
			Owned by {memberQuery.data.name || memberQuery.data.username}
		</span>
	);
};

type TriggerCellProps = {
	automation: ChatAutomation;
};

const TriggerCell: React.FC<TriggerCellProps> = ({ automation }) => {
	if (automation.kind === "schedule") {
		return (
			<div className="flex flex-col gap-0.5">
				<code className="text-xs text-content-primary">
					{automation.schedule_cron}
				</code>
				<span className="text-xs text-content-secondary">
					{automation.schedule_time_zone}
				</span>
			</div>
		);
	}
	return (
		<div className="flex flex-col gap-0.5">
			<span>
				{automation.webhook_use === "single"
					? "Webhook, single-use"
					: "Webhook, multi-use"}
			</span>
			{automation.webhook_consumed_at && (
				<span className="text-xs text-content-secondary">Used</span>
			)}
		</div>
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
		const isConsumed = Boolean(automation.webhook_consumed_at);
		const targetChatId =
			automation.target_mode === "existing_chat"
				? automation.target_chat_id
				: undefined;
		// Chat titles are looked up only for the viewer's own automations:
		// the per-owner cap bounds the requests, and other owners' chats are
		// usually unreadable, which would look like a missing target.
		const targetQuery = useQuery({
			...chat(targetChatId ?? ""),
			enabled: isOwner && Boolean(targetChatId),
		});
		// A deleted target clears target_chat_id; an archived one refuses runs.
		const isTargetMissing =
			automation.target_mode === "existing_chat" &&
			(!targetChatId ||
				getErrorStatus(targetQuery.error) === 404 ||
				Boolean(targetQuery.data?.archived));

		const nameDetails = (
			<>
				<span className="font-medium text-content-primary wrap-anywhere">
					{automation.name}
				</span>
				{!isOwner && (
					<OwnerName
						organizationId={automation.organization_id}
						userId={automation.owner_id}
					/>
				)}
				{automation.created_by_chat_id &&
					(isOwner ? (
						<CreatingChat chatId={automation.created_by_chat_id} />
					) : (
						<span className="text-xs text-content-secondary">
							Created by the agent
						</span>
					))}
			</>
		);
		const target =
			automation.target_mode === "new_chat" ? (
				<span className="text-content-secondary">New chat each run</span>
			) : isTargetMissing || !targetChatId ? (
				<Badge variant="warning" size="sm">
					Missing target
				</Badge>
			) : !isOwner ? (
				<span className="text-content-secondary">Existing chat</span>
			) : (
				<ChatTitle
					chat={targetQuery.data}
					chatId={targetChatId}
					isLoading={targetQuery.isLoading}
					error={targetQuery.error}
					size="sm"
				/>
			);
		const enabledSwitch = (
			<Switch
				// A used single-use webhook rejects every delivery, so
				// it shows as off and the switch cannot change that.
				checked={automation.enabled && !isConsumed}
				// Only the owner can re-enable an automation.
				disabled={isConsumed || isUpdating || (!isOwner && !automation.enabled)}
				aria-label={`Enable ${automation.name}`}
				onCheckedChange={(enabled) => onToggleEnabled(automation, enabled)}
			/>
		);
		const actions = (
			<div
				className={
					compact ? "flex flex-col items-end gap-2" : "flex justify-end gap-2"
				}
			>
				{automation.kind === "schedule" && isOwner && (
					<Button
						size="sm"
						variant="outline"
						disabled={!automation.enabled || isTargetMissing || isAnyRunPending}
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
					<Button
						size="sm"
						variant="outline"
						aria-label={`View chats ${automation.name}`}
						onClick={() => onViewChats(automation)}
					>
						View chats
					</Button>
				)}
				<Button
					size="sm"
					variant="outline"
					aria-label={`Edit ${automation.name}`}
					onClick={() => onEdit(automation)}
				>
					Edit
				</Button>
				{isOwner && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								size="icon"
								variant="outline"
								aria-label={`Delete ${automation.name}`}
								onClick={() => onDelete(automation)}
							>
								<Trash2Icon />
							</Button>
						</TooltipTrigger>
						<TooltipContent>Delete</TooltipContent>
					</Tooltip>
				)}
			</div>
		);

		if (compact) {
			return (
				<TableRow>
					<TableCell className="align-top">
						<div className="flex flex-col gap-1 wrap-anywhere">
							{nameDetails}
							<TriggerCell automation={automation} />
							<div>{target}</div>
							{automation.kind === "schedule" && (
								<span>
									Next run: <NextRun automation={automation} />
								</span>
							)}
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
					<div className="flex flex-col gap-0.5">{nameDetails}</div>
				</TableCell>
				<TableCell>
					<TriggerCell automation={automation} />
				</TableCell>
				<TableCell>{target}</TableCell>
				<TableCell className="whitespace-nowrap">
					<NextRun automation={automation} />
				</TableCell>
				<TableCell>{enabledSwitch}</TableCell>
				<TableCell>{actions}</TableCell>
			</TableRow>
		);
	},
);
