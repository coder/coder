import { ArrowLeftIcon, InfoIcon } from "lucide-react";
import { type FC, type PropsWithChildren, useState } from "react";
import type {
	AIBridgeSessionThreadsResponse,
	AIBridgeThread,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { PaywallAIGovernance } from "#/components/Paywall/PaywallAIGovernance";
import { SearchField } from "#/components/SearchField/SearchField";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AIBridgeSetupAlert } from "../AIBridgeSetupAlert";
import { SessionSummaryTable } from "./SessionSummaryTable";
import { SessionTimeline } from "./SessionTimeline/SessionTimeline";
import { SessionTimelineSkeleton } from "./SessionTimeline/SessionTimelineSkeleton";
import { countSessionSearchResults } from "./SessionTimeline/sessionSearch";

const SessionSummaryTooltip: FC<PropsWithChildren> = ({ children }) => (
	<TooltipProvider>
		<Tooltip>
			<TooltipTrigger asChild>
				<div className="flex-shrink-0 flex items-center">{children}</div>
			</TooltipTrigger>
			<TooltipContent
				side="top"
				align="start"
				className="max-w-xs flex flex-col gap-1 text-sm font-normal p-3"
			>
				<p className="m-0 leading-snug">
					A session is a set of threads or interceptions logically grouped by a
					session key issued by the client.
				</p>
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

interface SessionThreadsPageViewProps {
	session: AIBridgeSessionThreadsResponse | undefined;
	threads: readonly AIBridgeThread[];
	loading: boolean;
	hasNextPage: boolean;
	isFetchingNextPage: boolean;
	onFetchNextPage: () => void;
	isAISessionsEnabled: boolean;
	isAISessionsEntitled: boolean;
	onBackClicked: () => void;
}

export const SessionThreadsPageView: FC<SessionThreadsPageViewProps> = ({
	session,
	threads,
	loading,
	hasNextPage,
	isFetchingNextPage,
	onFetchNextPage,
	isAISessionsEnabled,
	isAISessionsEntitled,
	onBackClicked,
}) => {
	const [searchQuery, setSearchQuery] = useState("");

	if (!isAISessionsEntitled) {
		return <PaywallAIGovernance />;
	}

	if (!isAISessionsEnabled) {
		return <AIBridgeSetupAlert />;
	}

	// calculate the total number of tool calls across all loaded threads
	const toolCallCount = threads.reduce(
		(acc, thread) => acc + (thread.agentic_actions?.length ?? 0),
		0,
	);

	// The API returns only the single most contacted host, alongside the total
	// distinct domain count that drives the "+N more" overflow.
	const topDomain = session?.network_top_domains?.[0];

	const networkCalls = session?.network_call_logs ?? [];

	const isSearching = searchQuery.trim() !== "";

	const searchResults = countSessionSearchResults(
		threads,
		networkCalls,
		searchQuery,
	);

	return (
		<>
			<nav className="mb-6 flex flex-col md:flex-row md:items-start justify-between gap-4">
				<Button
					asChild
					variant="outline"
					size="lg"
					title="Back to AI Gateway sessions list"
					onClick={onBackClicked}
				>
					<span>
						<ArrowLeftIcon />
						Back
					</span>
				</Button>
				{session && (
					<div className="flex flex-col items-stretch md:items-end gap-1 md:w-96">
						<SearchField
							value={searchQuery}
							onChange={setSearchQuery}
							placeholder="Search prompt text, tool names, tool inputs, and network destinations"
							aria-label="Search session events"
						/>
						{isSearching && (
							<p
								className="m-0 text-sm font-normal text-content-secondary text-right"
								role="status"
							>
								<strong>{searchResults.toLocaleString("en-US")}</strong>{" "}
								{searchResults === 1 ? "result" : "results"}
							</p>
						)}
					</div>
				)}
			</nav>
			<div className="flex flex-col md:flex-row md:items-start gap-6">
				<aside className="md:w-80 md:shrink-0 px-3 py-2.5 border border-solid rounded-md flex flex-col gap-1">
					<h2 className="text-sm font-semibold flex items-center m-0">
						Session summary
						<SessionSummaryTooltip>
							<InfoIcon className="ml-2 text-content-secondary size-icon-xs" />
						</SessionSummaryTooltip>
					</h2>
					{loading && <Loader className="my-4" />}
					{session && (
						<SessionSummaryTable
							sessionId={session.id}
							startTime={new Date(session.started_at)}
							endTime={
								session.ended_at ? new Date(session.ended_at) : undefined
							}
							initiator={session.initiator}
							client={session.client ?? "Unknown client"}
							providers={session.providers}
							inputTokens={session.token_usage_summary.input_tokens}
							outputTokens={session.token_usage_summary.output_tokens}
							threadCount={threads.length}
							toolCallCount={toolCallCount}
							tokenUsageMetadata={session.token_usage_summary.metadata}
							networkCalls={session.network_calls}
							networkDomains={
								topDomain && {
									topDomain,
									totalCount: session.network_domain_count ?? 1,
								}
							}
						/>
					)}
				</aside>
				<main className="flex-1 min-w-0">
					{session ? (
						<SessionTimeline
							initiator={session.initiator}
							threads={threads}
							networkCallSummary={session.network_calls}
							networkCalls={networkCalls}
							searchQuery={searchQuery}
							hasNextPage={hasNextPage}
							isFetchingNextPage={isFetchingNextPage}
							onFetchNextPage={onFetchNextPage}
						/>
					) : (
						loading && <SessionTimelineSkeleton />
					)}
				</main>
			</div>
		</>
	);
};
