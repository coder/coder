import type { MinimalUser } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeProviderIcon";
import { formatDateTime } from "#/utils/time";
import { TokenBadges } from "../TokenBadges";
import { getProviderDisplayName, getProviderIconName } from "../utils";

const Separator = () => <div className="border-0 border-t border-solid my-1" />;

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
		endTime !== undefined
			? new Date(endTime).getTime() - new Date(startTime).getTime()
			: undefined;

	return (
		<dl className="text-sm text-content-secondary m-0 flex flex-col gap-y-2">
			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Session ID</dt>
				<dd
					className="ml-4 min-w-0 truncate text-content-primary text-xs font-mono"
					title={sessionId}
				>
					{sessionId}
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Start time</dt>
				<dd
					className="ml-4 min-w-0 truncate text-content-primary text-xs font-mono"
					title={formatDateTime(startTime)}
				>
					{formatDateTime(startTime)}
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">End time</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary text-xs font-mono">
					{endTime ? formatDateTime(endTime) : "—"}
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Duration</dt>
				<dd
					className="ml-4 min-w-0 truncate text-content-primary text-xs font-mono"
					title={durationInMs !== undefined ? `${durationInMs} ms` : undefined}
				>
					{durationInMs !== undefined
						? `${Math.round(durationInMs / 1000)} s`
						: "—"}
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Initiator</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary flex items-center gap-2">
					<Avatar
						size="sm"
						src={initiator.avatar_url}
						fallback={initiator.name}
					/>
					<span className="truncate min-w-0" title={initiator.name}>
						{initiator.name}
					</span>
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Client</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary">
					<Badge className="gap-1.5 max-w-full min-w-0 overflow-hidden">
						<div className="flex-shrink-0 flex items-center">
							<AIBridgeClientIcon client={client} className="size-icon-xs" />
						</div>
						<span
							className="truncate min-w-0 flex-1"
							title={client ?? "Unknown"}
						>
							{client ?? "Unknown"}
						</span>
					</Badge>
				</dd>
			</div>

			<div className="flex items-start justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap mt-1">
					Provider
				</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary flex flex-wrap gap-1">
					{providers.map((p) => (
						<Badge
							key={p}
							className="gap-1.5 max-w-full min-w-0 overflow-hidden"
						>
							<AIBridgeProviderIcon
								provider={getProviderIconName(p)}
								className="size-icon-xs"
							/>
							<span
								className="truncate min-w-0 flex-1"
								title={getProviderDisplayName(p)}
							>
								{getProviderDisplayName(p)}
							</span>
						</Badge>
					))}
				</dd>
			</div>

			<Separator />

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">
					In / out tokens
				</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary">
					<TokenBadges
						inputTokens={inputTokens}
						outputTokens={outputTokens}
						tokenUsageMetadata={tokenUsageMetadata}
					/>
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Threads</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary">
					<Badge>{threadCount}</Badge>
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">Tool calls</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary">
					<Badge>{toolCallCount}</Badge>
				</dd>
			</div>
		</dl>
	);
};
