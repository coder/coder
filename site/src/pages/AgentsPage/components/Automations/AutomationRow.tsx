import { memo } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import { chat } from "#/api/queries/chats";
import type { Chat, ChatAutomation } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link, type LinkProps } from "#/components/Link/Link";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { TableCell, TableRow } from "#/components/Table/Table";
import { formatDate } from "#/utils/time";

type AutomationRowProps = {
	automation: ChatAutomation;
	isOwner: boolean;
	isUpdating: boolean;
	isRunning: boolean;
	/** One run request at a time, so a pending run disables every row. */
	isAnyRunPending: boolean;
	onToggleEnabled: (automation: ChatAutomation, enabled: boolean) => void;
	onRunNow: (automation: ChatAutomation) => void;
	onViewChats: (automation: ChatAutomation) => void;
};

const formatNextRun = (automation: ChatAutomation): string => {
	const next = automation.next_run_times[0];
	if (automation.kind !== "schedule" || !next) {
		return "Not scheduled";
	}
	const date = new Date(next);
	if (!Number.isFinite(date.getTime())) {
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

type ChatTitleLinkProps = {
	chatId: string;
	title: string | undefined;
	size?: LinkProps["size"];
};

/** Links to a chat by its title, falling back to "Untitled". */
export const ChatTitleLink: React.FC<ChatTitleLinkProps> = ({
	chatId,
	title,
	size = "lg",
}) => {
	return (
		<Link asChild showExternalIcon={false} size={size}>
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
};

const ChatTitle: React.FC<ChatTitleProps> = ({
	chat,
	chatId,
	isLoading,
	error,
	size,
}) => {
	if (isLoading) {
		return <Skeleton className="h-4 w-32" />;
	}
	if (error) {
		return (
			<span className="text-content-secondary">
				{getErrorMessage(error, "Chat unavailable")}
			</span>
		);
	}
	return <ChatTitleLink chatId={chatId} title={chat?.title} size={size} />;
};

type CreatingChatProps = {
	chatId: string;
};

const CreatingChat: React.FC<CreatingChatProps> = ({ chatId }) => {
	const chatQuery = useQuery(chat(chatId));
	return (
		<span className="flex items-center gap-1 text-xs text-content-secondary">
			Created by agent in
			{getErrorStatus(chatQuery.error) === 404 ? (
				" a deleted chat"
			) : (
				<ChatTitle
					chat={chatQuery.data}
					chatId={chatId}
					isLoading={chatQuery.isLoading}
					error={chatQuery.error}
					size="sm"
				/>
			)}
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
		isUpdating,
		isRunning,
		isAnyRunPending,
		onToggleEnabled,
		onRunNow,
		onViewChats,
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

		return (
			<TableRow>
				<TableCell>
					<div className="flex flex-col gap-0.5">
						<span className="font-medium text-content-primary">
							{automation.name}
						</span>
						{automation.created_by_chat_id &&
							(isOwner ? (
								<CreatingChat chatId={automation.created_by_chat_id} />
							) : (
								<span className="text-xs text-content-secondary">
									Created by agent
								</span>
							))}
					</div>
				</TableCell>
				<TableCell>
					<TriggerCell automation={automation} />
				</TableCell>
				<TableCell>
					{automation.target_mode === "new_chat" ? (
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
						/>
					)}
				</TableCell>
				<TableCell className="whitespace-nowrap">
					{formatNextRun(automation)}
				</TableCell>
				<TableCell>
					<Switch
						// A used single-use webhook rejects every delivery, so
						// it shows as off and the switch cannot change that.
						checked={automation.enabled && !isConsumed}
						// Only the owner can re-enable an automation.
						disabled={
							isConsumed || isUpdating || (!isOwner && !automation.enabled)
						}
						aria-label={`Enable ${automation.name}`}
						onCheckedChange={(enabled) => onToggleEnabled(automation, enabled)}
					/>
				</TableCell>
				<TableCell>
					<div className="flex justify-end gap-2">
						{automation.kind === "schedule" && isOwner && (
							<Button
								size="sm"
								variant="outline"
								disabled={
									!automation.enabled || isTargetMissing || isAnyRunPending
								}
								aria-label={`Run now ${automation.name}`}
								onClick={() => onRunNow(automation)}
							>
								<Spinner loading={isRunning} />
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
					</div>
				</TableCell>
			</TableRow>
		);
	},
);
