import type { MinimalUser } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeProviderIcon";
import { formatDateTime } from "#/utils/time";
import { TokenBadges } from "../TokenBadges";
import { getProviderDisplayName, getProviderIconName } from "../utils";

const Separator = () => (
	<div className="border-0 border-t border-solid border-border-content-secondary my-1" />
);

interface SessionSummaryTableProps {
	sessionId: string;
	startTime: Date;
	endTime?: Date;
	initiator: MinimalUser;
	client: string;
	providers: readonly string[];
	inputTokens: number;
	outputTokens: number;
	threadCount: number;
	toolCallCount: number;
	tokenUsageMetadata?: Record<string, unknown>;
}

export const SessionSummaryTable = ({
	sessionId,
	startTime,
	endTime,
	initiator,
	providers,
	client,
	inputTokens,
	outputTokens,
	threadCount,
	toolCallCount,
	tokenUsageMetadata,
}: SessionSummaryTableProps) => {
	const durationInMs =
		endTime != null
			? new Date(endTime).getTime() - new Date(startTime).getTime()
			: undefined;

	return (
		<div className="text-sm text-content-secondary flex flex-col gap-2">
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Session ID</span>
				<span
					className="text-content-primary font-mono truncate"
					title={sessionId}
				>
					{sessionId}
				</span>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Start time</span>
				<span
					className="text-content-primary font-mono truncate"
					title={formatDateTime(startTime)}
				>
					{formatDateTime(startTime)}
				</span>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">End time</span>
				<span className="text-content-primary font-mono truncate">
					{endTime ? formatDateTime(endTime) : "—"}
				</span>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Duration</span>
				<span
					className="text-content-primary font-mono truncate"
					title={durationInMs != null ? `${durationInMs} ms` : undefined}
				>
					{durationInMs != null ? `${Math.round(durationInMs / 1000)} s` : "—"}
				</span>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Initiator</span>
				<div className="flex items-center gap-2">
					<Avatar
						size="sm"
						src={initiator.avatar_url}
						fallback={initiator.name}
					/>
					<span className="truncate" title={initiator.name}>
						{initiator.name}
					</span>
				</div>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Client</span>
				<Badge className="gap-1.5 max-w-full">
					<div className="flex-shrink-0 flex items-center">
						<AIBridgeClientIcon client={client} className="size-icon-xs" />
					</div>
					<span className="truncate min-w-0" title={client ?? "Unknown"}>
						{client ?? "Unknown"}
					</span>
				</Badge>
			</div>
			<div className="flex items-start justify-between">
				<span className="pr-4 whitespace-nowrap">Provider</span>
				<div className="flex flex-col items-end gap-1">
					{providers.map((p) => (
						<Badge key={p} className="gap-1.5 max-w-full">
							<AIBridgeProviderIcon
								provider={getProviderIconName(p)}
								className="size-icon-xs"
							/>
							<span
								className="truncate min-w-0"
								title={getProviderDisplayName(p)}
							>
								{getProviderDisplayName(p)}
							</span>
						</Badge>
					))}
				</div>
			</div>
			<Separator />
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">In / out tokens</span>
				<TokenBadges
					inputTokens={inputTokens}
					outputTokens={outputTokens}
					tokenUsageMetadata={tokenUsageMetadata}
				/>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Threads</span>
				<Badge>{threadCount}</Badge>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4 whitespace-nowrap">Tool calls</span>
				<Badge>{toolCallCount}</Badge>
			</div>
		</div>
	);
};
