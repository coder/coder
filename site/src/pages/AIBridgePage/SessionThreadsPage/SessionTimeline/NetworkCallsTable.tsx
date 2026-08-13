import { BanIcon, CheckIcon, ChevronRightIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type {
	AgentFirewallLog,
	AIBridgeSessionNetworkCallSummary,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { formatDateTime } from "#/utils/time";

interface NetworkCallsTableProps {
	/**
	 * Reports the whole session's network call totals. The total can exceed the
	 * number of rows in `calls`, which is capped server-side.
	 */
	summary: AIBridgeSessionNetworkCallSummary;
	calls: readonly AgentFirewallLog[];
	/**
	 * When set, the table renders search results. `calls` holds only the
	 * matching rows, and `loaded` is the number of network calls the client
	 * received (at most the server-side cap). When `loaded` is below
	 * `summary.total`, the search only covered a truncated prefix.
	 */
	search?: { loaded: number };
}

export const NetworkCallsTable: FC<NetworkCallsTableProps> = ({
	summary,
	calls,
	search,
}) => {
	const matches = calls.length;
	const blocked = search
		? calls.filter((call) => !call.allowed).length
		: summary.blocked;

	return (
		<Collapsible defaultOpen className="border border-solid rounded-md">
			<div className="flex items-center justify-between gap-2 px-2 py-1">
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className="group flex items-center gap-4 p-1 bg-transparent border-none cursor-pointer text-sm font-normal text-content-secondary"
					>
						<ChevronRightIcon className="size-3.5 transition-transform group-data-[state=open]:rotate-90" />
						{search ? (
							<span>
								{matches.toLocaleString("en-US")}{" "}
								{matches === 1 ? "match" : "matches"}
							</span>
						) : (
							<span>
								Network calls ({summary.total.toLocaleString("en-US")})
							</span>
						)}
					</button>
				</CollapsibleTrigger>
				{blocked > 0 && (
					<Badge svgSize="xs" className="gap-1 text-content-warning">
						<BanIcon className="flex-shrink-0" />
						<span className="sr-only">
							{search ? "Blocked matches: " : "Blocked network calls: "}
						</span>
						{blocked.toLocaleString("en-US")}
					</Badge>
				)}
			</div>

			<CollapsibleContent className="border-0 border-t border-solid">
				<NetworkCallsList
					calls={calls}
					search={search}
					sessionTotal={summary.total}
				/>
			</CollapsibleContent>
		</Collapsible>
	);
};

const NetworkCallsList: FC<{
	calls: readonly AgentFirewallLog[];
	search?: { loaded: number };
	sessionTotal: number;
}> = ({ calls, search, sessionTotal }) => {
	const matches = calls.length;
	const hiddenCount = sessionTotal - matches;
	const isTruncated = search ? search.loaded < sessionTotal : hiddenCount > 0;

	if (matches === 0) {
		return (
			<>
				{search ? (
					<p className="m-0 px-4 py-3 text-sm font-normal text-content-secondary">
						No network calls match your search.
					</p>
				) : (
					<p className="m-0 px-4 py-3 text-sm font-normal text-content-secondary">
						No network calls were recorded for this session.
					</p>
				)}
				{search && isTruncated && (
					<p className="m-0 px-4 py-2 text-xs font-normal text-content-secondary border-0 border-t border-solid">
						{`0 matches within the first ${search.loaded.toLocaleString(
							"en-US",
						)} of ${sessionTotal.toLocaleString("en-US")} network calls.`}
					</p>
				)}
			</>
		);
	}

	return (
		<>
			<ul className="m-0 p-0 list-none">
				{calls.map((call) => (
					<NetworkCallRow key={call.id} call={call} />
				))}
			</ul>
			{isTruncated && (
				<p className="m-0 px-4 py-2 text-xs font-normal text-content-secondary border-0 border-t border-solid">
					{search
						? `${matches.toLocaleString("en-US")} ${
								matches === 1 ? "match" : "matches"
							} within the first ${search.loaded.toLocaleString("en-US")} of ${sessionTotal.toLocaleString(
								"en-US",
							)} network calls.`
						: `Showing the first ${matches.toLocaleString("en-US")} of ${sessionTotal.toLocaleString(
								"en-US",
							)} network calls.`}
				</p>
			)}
		</>
	);
};

interface NetworkCallRowProps {
	call: AgentFirewallLog;
}

const NetworkCallRow: FC<NetworkCallRowProps> = ({ call }) => {
	const timestamp = formatDateTime(new Date(call.created_at));

	return (
		<li className="border-0 border-t border-solid first:border-t-0">
			<Collapsible>
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className="group flex items-center gap-3 w-full px-2 py-2 text-left bg-transparent border-none cursor-pointer hover:bg-surface-secondary"
					>
						<ChevronRightIcon className="size-3.5 flex-shrink-0 text-content-secondary transition-transform group-data-[state=open]:rotate-90" />
						{call.method && (
							<Badge size="sm" className="flex-shrink-0 font-mono">
								{call.method}
							</Badge>
						)}
						<NetworkCallStatusBadge allowed={call.allowed} />
						<span
							className="flex-1 min-w-0 truncate font-mono text-xs text-content-primary"
							title={call.detail}
						>
							{call.detail || "N/A"}
						</span>
						<span className="hidden md:flex items-center gap-2 flex-shrink-0 text-sm font-normal text-content-secondary">
							Timestamp
							<span className="font-mono text-xs text-content-primary">
								{timestamp}
							</span>
						</span>
					</button>
				</CollapsibleTrigger>

				<CollapsibleContent>
					<dl className="flex flex-col gap-2 m-0 px-9 pb-3 text-sm font-normal text-content-secondary">
						<NetworkCallDetailRow label="URL">
							<span
								className="min-w-0 truncate font-mono text-xs text-content-primary"
								title={call.detail}
							>
								{call.detail || "N/A"}
							</span>
							{call.detail && (
								<CopyButton text={call.detail} label="Copy network call URL" />
							)}
						</NetworkCallDetailRow>
						<NetworkCallDetailRow label="Protocol">
							<span className="font-mono text-xs text-content-primary">
								{call.proto || "N/A"}
							</span>
						</NetworkCallDetailRow>
						<NetworkCallDetailRow label="Matched rule">
							<span
								className="min-w-0 truncate font-mono text-xs text-content-primary"
								title={call.matched_rule ?? undefined}
							>
								{call.matched_rule ?? "None"}
							</span>
						</NetworkCallDetailRow>
						<NetworkCallDetailRow label="Timestamp">
							<span className="font-mono text-xs text-content-primary">
								{timestamp}
							</span>
						</NetworkCallDetailRow>
					</dl>
				</CollapsibleContent>
			</Collapsible>
		</li>
	);
};

const NetworkCallStatusBadge: FC<{ allowed: boolean }> = ({ allowed }) =>
	allowed ? (
		<Badge size="sm" svgSize="xs" className="flex-shrink-0 gap-1">
			<CheckIcon className="flex-shrink-0" />
			Allowed
		</Badge>
	) : (
		<Badge
			size="sm"
			svgSize="xs"
			className="flex-shrink-0 gap-1 text-content-warning"
		>
			<BanIcon className="flex-shrink-0" />
			Blocked
		</Badge>
	);

interface NetworkCallDetailRowProps {
	label: string;
	children: ReactNode;
}

const NetworkCallDetailRow: FC<NetworkCallDetailRowProps> = ({
	label,
	children,
}) => (
	<div className="flex items-center justify-between gap-4">
		<dt className="shrink-0 whitespace-nowrap">{label}</dt>
		<dd className="flex items-center gap-2 m-0 min-w-0">{children}</dd>
	</div>
);
