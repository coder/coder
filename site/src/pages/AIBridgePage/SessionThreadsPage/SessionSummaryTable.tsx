import { BanIcon } from "lucide-react";
import type { ReactNode } from "react";
import type {
	AIBridgeSessionNetworkCallSummary,
	AIBridgeSessionNetworkDomain,
	MinimalUser,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Link } from "#/components/Link/Link";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/icons/AIBridgeProviderIcon";
import { formatDateTime } from "#/utils/time";
import {
	NetworkMonitoringDisabled,
	NetworkNoActivity,
} from "../NetworkRequestStates";
import { TokenBadges } from "../TokenBadges";
import { getProviderDisplayName } from "../utils";

const Separator = () => <div className="border-0 border-t border-solid my-1" />;

const linearIssueBaseURL = "https://linear.app/codercom/issue/";

// githubPRLabel shortens a pull request URL to owner/repo#number, falling back
// to the URL when it does not match that shape.
const githubPRLabel = (url: string): string => {
	const match = /^https:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)$/.exec(
		url,
	);
	if (!match) {
		return url;
	}
	return `${match[1]}/${match[2]}#${match[3]}`;
};

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
	// networkCalls is undefined when the session did not pass through Agent
	// Firewall, which renders as "Disabled".
	networkCalls?: AIBridgeSessionNetworkCallSummary;
	// networkDomains is undefined when the session contacted no destination
	// hosts. totalCount is the number of distinct domains contacted, which
	// renders as a "+N more" overflow beyond topDomain.
	networkDomains?: {
		readonly topDomain: AIBridgeSessionNetworkDomain;
		readonly totalCount: number;
	};
	// linearIssueIds are the Linear issues annotated on the session's
	// interceptions. githubPrUrls, repos and branches are the same for their
	// respective annotations. Empty when none were annotated.
	linearIssueIds?: readonly string[];
	githubPrUrls?: readonly string[];
	repos?: readonly string[];
	branches?: readonly string[];
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
	networkCalls,
	networkDomains,
	linearIssueIds,
	githubPrUrls,
	repos,
	branches,
}: SessionSummaryTableProps) => {
	const durationInMs =
		endTime !== undefined
			? new Date(endTime).getTime() - new Date(startTime).getTime()
			: undefined;

	let networkCallsValue: ReactNode;
	if (networkCalls === undefined) {
		networkCallsValue = <NetworkMonitoringDisabled />;
	} else if (networkCalls.total === 0) {
		networkCallsValue = <NetworkNoActivity />;
	} else {
		networkCallsValue = (
			<Badge>{networkCalls.total.toLocaleString("en-US")}</Badge>
		);
	}

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
							<AIBridgeProviderIcon provider={p} className="size-icon-xs" />
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

			<Separator />

			<div className="flex items-center justify-between">
				<dt className="shrink-0 font-normal whitespace-nowrap">
					Network requests
				</dt>
				<dd className="ml-4 min-w-0 truncate text-content-primary">
					{networkCallsValue}
				</dd>
			</div>

			{networkCalls !== undefined && networkCalls.total > 0 && (
				<div className="flex items-center justify-between">
					<dt className="shrink-0 font-normal whitespace-nowrap">
						Blocked network requests
					</dt>
					<dd className="ml-4 min-w-0 truncate text-content-primary">
						{networkCalls.blocked > 0 ? (
							<Badge svgSize="xs" className="gap-1 text-content-warning">
								<BanIcon className="flex-shrink-0" />
								{networkCalls.blocked.toLocaleString("en-US")}
							</Badge>
						) : (
							<Badge>{networkCalls.blocked.toLocaleString("en-US")}</Badge>
						)}
					</dd>
				</div>
			)}

			{networkDomains !== undefined && (
				<div className="flex items-start justify-between">
					<dt className="shrink-0 font-normal whitespace-nowrap mt-px">
						Top domains
					</dt>
					<dd className="ml-4 min-w-0 text-content-primary text-right">
						<div className="truncate" title={networkDomains.topDomain.domain}>
							{networkDomains.topDomain.domain}
						</div>
						{networkDomains.totalCount > 1 && (
							<div className="text-content-secondary text-xs">
								+{(networkDomains.totalCount - 1).toLocaleString("en-US")} more
							</div>
						)}
					</dd>
				</div>
			)}
			{linearIssueIds !== undefined && linearIssueIds.length > 0 && (
				<div className="flex items-start justify-between">
					<dt className="shrink-0 font-normal whitespace-nowrap mt-px">
						Linear issues
					</dt>
					<dd className="ml-4 min-w-0 text-content-primary text-right flex flex-col items-end">
						{linearIssueIds.map((issueId) => (
							<Link
								key={issueId}
								size="sm"
								href={`${linearIssueBaseURL}${encodeURIComponent(issueId)}`}
								target="_blank"
								rel="noreferrer"
								className="max-w-full min-w-0"
							>
								<span className="truncate">{issueId}</span>
							</Link>
						))}
					</dd>
				</div>
			)}

			{repos !== undefined && repos.length > 0 && (
				<div className="flex items-start justify-between">
					<dt className="shrink-0 font-normal whitespace-nowrap mt-px">
						Repository
					</dt>
					<dd className="ml-4 min-w-0 text-content-primary text-right">
						{repos.map((repo) => (
							<div key={repo} className="truncate" title={repo}>
								{repo}
							</div>
						))}
					</dd>
				</div>
			)}

			{branches !== undefined && branches.length > 0 && (
				<div className="flex items-start justify-between">
					<dt className="shrink-0 font-normal whitespace-nowrap mt-px">
						Branch
					</dt>
					<dd className="ml-4 min-w-0 text-content-primary text-right">
						{branches.map((branch) => (
							<div key={branch} className="truncate" title={branch}>
								{branch}
							</div>
						))}
					</dd>
				</div>
			)}

			{githubPrUrls !== undefined && githubPrUrls.length > 0 && (
				<div className="flex items-start justify-between">
					<dt className="shrink-0 font-normal whitespace-nowrap mt-px">
						Pull requests
					</dt>
					<dd className="ml-4 min-w-0 text-content-primary text-right flex flex-col items-end">
						{githubPrUrls.map((url) => (
							<Link
								key={url}
								size="sm"
								href={url}
								target="_blank"
								rel="noreferrer"
								className="max-w-full min-w-0"
							>
								<span className="truncate">{githubPRLabel(url)}</span>
							</Link>
						))}
					</dd>
				</div>
			)}
		</dl>
	);
};
