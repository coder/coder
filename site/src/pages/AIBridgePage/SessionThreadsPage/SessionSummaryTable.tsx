import type { MinimalUser } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeProviderIcon";
import { cn } from "#/utils/cn";
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
		<dl
			className={cn(
				"text-sm text-content-secondary m-0 whitespace-nowrap",
				"grid grid-cols-[auto_1fr] gap-y-2 [&_dd]:ml-0 [&_dd]:text-content-primary",
				"[&_dd]:h-6 [&_dd]:flex [&_dd]:min-w-0 [&_dd]:items-center [&_dd]:justify-end",
				"[&_dt]:h-6 [&_dt]:inline-flex [&_dt]:items-center [&_dt]:font-normal",
			)}
		>
			<dt>Session ID</dt>
			<dd className="text-xs font-mono min-w-0" title={sessionId}>
				<span className="truncate w-full text-right">{sessionId}</span>
			</dd>

			<dt>Start time</dt>
			<dd className="text-xs font-mono" title={formatDateTime(startTime)}>
				{formatDateTime(startTime)}
			</dd>

			<dt>End time</dt>
			<dd className="text-xs font-mono">
				{endTime ? formatDateTime(endTime) : "—"}
			</dd>

			<dt>Duration</dt>
			<dd
				className="text-xs font-mono"
				title={durationInMs !== undefined ? `${durationInMs} ms` : undefined}
			>
				{durationInMs !== undefined
					? `${Math.round(durationInMs / 1000)} s`
					: "—"}
			</dd>

			<dt>Initiator</dt>
			<dd>
				<div className="flex w-full min-w-0 items-center justify-end gap-2">
					<Avatar
						size="sm"
						src={initiator.avatar_url}
						fallback={initiator.name}
					/>
					<span className="truncate min-w-0 text-right" title={initiator.name}>
						{initiator.name}
					</span>
				</div>
			</dd>

			<dt>Client</dt>
			<dd>
				<Badge className="gap-1.5 max-w-full min-w-0 overflow-hidden">
					<div className="flex-shrink-0 flex items-center">
						<AIBridgeClientIcon client={client} className="size-icon-xs" />
					</div>
					<span className="truncate min-w-0 flex-1" title={client ?? "Unknown"}>
						{client ?? "Unknown"}
					</span>
				</Badge>
			</dd>

			<dt className="self-start">Provider</dt>
			<dd>
				{providers.map((p) => (
					<Badge key={p} className="gap-1.5 max-w-full min-w-0 overflow-hidden">
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

			<div className="col-span-2">
				<Separator />
			</div>

			<dt>In / out tokens</dt>
			<dd>
				<TokenBadges
					inputTokens={inputTokens}
					outputTokens={outputTokens}
					tokenUsageMetadata={tokenUsageMetadata}
				/>
			</dd>

			<dt>Threads</dt>
			<dd>
				<Badge>{threadCount}</Badge>
			</dd>

			<dt>Tool calls</dt>
			<dd>
				<Badge>{toolCallCount}</Badge>
			</dd>
		</dl>
	);
};
