import { memo } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { getErrorStatus } from "#/api/errors";
import { chat } from "#/api/queries/chats";
import type { ChatAutomation } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { TableCell, TableRow } from "#/components/Table/Table";
import { formatDate } from "#/utils/time";

type AutomationRowProps = {
	automation: ChatAutomation;
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

const MissingTarget: React.FC = () => (
	<Badge variant="warning" size="sm">
		Missing target
	</Badge>
);

/** Links to a chat by title. A missing or archived chat renders `missing`. */
const ChatTitleLink: React.FC<{
	chatId: string;
	missing?: React.ReactNode;
	size?: "sm" | "lg";
}> = ({ chatId, missing, size = "lg" }) => {
	const chatQuery = useQuery(chat(chatId));
	if (chatQuery.isLoading) {
		return <Skeleton className="h-4 w-32" />;
	}
	const isGone =
		getErrorStatus(chatQuery.error) === 404 || chatQuery.data?.archived;
	if (isGone && missing) {
		return missing;
	}
	return (
		<Link asChild showExternalIcon={false} size={size}>
			<RouterLink to={`/agents/${chatId}`}>
				{chatQuery.data?.title || "Untitled"}
			</RouterLink>
		</Link>
	);
};

const TriggerCell: React.FC<{ automation: ChatAutomation }> = ({
	automation,
}) => {
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
	const use = automation.webhook_use === "single" ? "single use" : "multi use";
	return (
		<div className="flex flex-col gap-0.5">
			<span>Webhook, {use}</span>
			{automation.webhook_consumed_at && (
				<span className="text-xs text-content-secondary">Used</span>
			)}
		</div>
	);
};

const TargetCell: React.FC<{ automation: ChatAutomation }> = ({
	automation,
}) => {
	if (automation.target_mode === "new_chat") {
		return <span className="text-content-secondary">New chat each run</span>;
	}
	if (!automation.target_chat_id) {
		return <MissingTarget />;
	}
	return (
		<ChatTitleLink
			chatId={automation.target_chat_id}
			missing={<MissingTarget />}
		/>
	);
};

// memo() keeps unrelated rows from re-rendering while one row's mutation
// is pending.
export const AutomationRow = memo<AutomationRowProps>(
	({
		automation,
		isUpdating,
		isRunning,
		onToggleEnabled,
		onRunNow,
		onViewChats,
	}) => {
		return (
			<TableRow>
				<TableCell>
					<div className="flex flex-col gap-0.5">
						<span className="font-medium text-content-primary">
							{automation.name}
						</span>
						{automation.created_by_chat_id && (
							<span className="flex items-center gap-1 text-xs text-content-secondary">
								Created by agent in
								<ChatTitleLink
									chatId={automation.created_by_chat_id}
									size="sm"
								/>
							</span>
						)}
					</div>
				</TableCell>
				<TableCell>
					<TriggerCell automation={automation} />
				</TableCell>
				<TableCell>
					<TargetCell automation={automation} />
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
						{automation.kind === "schedule" && (
							<Button
								size="sm"
								variant="outline"
								disabled={!automation.enabled || isRunning}
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
