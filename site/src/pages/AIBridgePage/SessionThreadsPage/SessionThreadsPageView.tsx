import type {
	AIBridgeSessionThreadsResponse,
	AIBridgeThread,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ArrowLeftIcon, InfoIcon } from "lucide-react";
import type { FC, PropsWithChildren } from "react";
import { Link as RouterLink } from "react-router";
import { docs } from "utils/docs";
import { SessionSummaryTable } from "./SessionSummaryTable";
import { SessionTimeline } from "./SessionTimeline/SessionTimeline";

const SessionSummaryTooltip: FC<PropsWithChildren> = ({ children }) => (
	<TooltipProvider>
		<Tooltip>
			<TooltipTrigger asChild>
				<div className="flex-shrink-0 flex items-center">{children}</div>
			</TooltipTrigger>
			<TooltipContent side="top" align="start" className="max-w-xs">
				<p className="text-sm">
					A session is a set of threads or interceptions logically grouped by a
					session key issued by the client.
				</p>
				<p>
					<Link href={docs("/ai-coder/ai-bridge")} target="_blank">
						View session terminology
					</Link>
				</p>
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

export interface SessionThreadsPageViewProps {
	session: AIBridgeSessionThreadsResponse | undefined;
	threads: readonly AIBridgeThread[];
	loading: boolean;
	hasNextPage: boolean;
	isFetchingNextPage: boolean;
	onFetchNextPage: () => void;
}

export const SessionThreadsPageView: FC<SessionThreadsPageViewProps> = ({
	session,
	threads,
	loading,
	hasNextPage,
	isFetchingNextPage,
	onFetchNextPage,
}) => {
	// Sum tool_calls within each agentic action, since each action can have
	// multiple tool calls.
	const threadToolCallCount = (thread: AIBridgeThread) =>
		thread.agentic_actions?.reduce(
			(acc, action) => acc + (action.tool_calls?.length ?? 0),
			0,
		) ?? 0;
	const toolCallCount = threads.reduce(
		(acc, thread) => acc + threadToolCallCount(thread),
		0,
	);

	return (
		<>
			<nav className="mb-6">
				<Button
					asChild
					variant="outline"
					size="lg"
					title="Back to AI Bridge sessions list"
				>
					<RouterLink to="/aibridge/sessions">
						<ArrowLeftIcon />
						Back
					</RouterLink>
				</Button>
			</nav>
			<div className="flex flex-col md:flex-row md:items-start gap-6">
				<aside className="md:w-72 md:shrink-0 p-4 border border-solid rounded-md">
					<h4 className="text-md flex items-center m-0 mb-4">
						Session summary
						<SessionSummaryTooltip>
							<InfoIcon className="ml-2 h-4 w-4 text-content-secondary" />
						</SessionSummaryTooltip>
					</h4>
					{loading && (
						// TODO actual loader
						<div className="text-content-secondary text-sm">Loading…</div>
					)}
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
						/>
					)}
				</aside>
				<main className="flex-1 min-w-0">
					{session && (
						<SessionTimeline
							initiator={session.initiator}
							threads={threads}
							hasNextPage={hasNextPage}
							isFetchingNextPage={isFetchingNextPage}
							onFetchNextPage={onFetchNextPage}
						/>
					)}
				</main>
			</div>
		</>
	);
};
