import { memo } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { getErrorStatus } from "#/api/errors";
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
	chat: Chat | undefined;
	chatId: string;
	isLoading: boolean;
	size?: LinkProps["size"];
};

const ChatTitleLink: React.FC<ChatTitleLinkProps> = ({
	chat,
	chatId,
	isLoading,
	size = "lg",
}) => {
	if (isLoading) {
		return <Skeleton className="h-4 w-32" />;
	}
	return (
		<Link asChild showExternalIcon={false} size={size}>
			<RouterLink to={`/agents/${chatId}`}>
				{chat?.title || "Untitled"}
			</RouterLink>
		</Link>
	);
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
				<ChatTitleLink
					chat={chatQuery.data}
					chatId={chatId}
					isLoading={chatQuery.isLoading}
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
				Webhook,{" "}
				{automation.webhook_use === "single" ? "single use" : "multi use"}
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
		onToggleEnabled,
		onRunNow,
		onViewChats,
	}) => {
		const targetChatId =
			automation.target_mode === "existing_chat"
				? automation.target_chat_id
				: undefined;
		const targetQuery = useQuery({
			...chat(targetChatId ?? ""),
			enabled: Boolean(targetChatId),
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
						{automation.created_by_chat_id && (
							<CreatingChat chatId={automation.created_by_chat_id} />
						)}
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
					) : (
						<ChatTitleLink
							chat={targetQuery.data}
							chatId={targetChatId}
							isLoading={targetQuery.isLoading}
						/>
					)}
				</TableCell>
				<TableCell className="whitespace-nowrap">
					{formatNextRun(automation)}
				</TableCell>
				<TableCell>
					<Switch
						checked={automation.enabled}
						disabled={isUpdating}
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
								disabled={!automation.enabled || isTargetMissing || isRunning}
								onClick={() => onRunNow(automation)}
							>
								<Spinner loading={isRunning} />
								Run now
							</Button>
						)}
						<Button
							size="sm"
							variant="outline"
							onClick={() => onViewChats(automation)}
						>
							View chats
						</Button>
					</div>
				</TableCell>
			</TableRow>
		);
	},
);
