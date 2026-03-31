import { ArrowLeftIcon, InfoIcon } from "lucide-react";
import type { FC, PropsWithChildren } from "react";
import { Link as RouterLink } from "react-router";
import type {
	AIBridgeSessionThreadsResponse,
	AIBridgeThread,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { SessionSummaryTable } from "./SessionSummaryTable";
import { SessionTimeline } from "./SessionTimeline/SessionTimeline";

const SessionSummaryTooltip: FC<PropsWithChildren> = ({ children }) => (
	<TooltipProvider>
		<Tooltip>
			<TooltipTrigger asChild>
				<div className="flex-shrink-0 flex items-center">{children}</div>
			</TooltipTrigger>
			<TooltipContent
				side="top"
				align="start"
				className="max-w-xs flex flex-col gap-1 text-xs p-3"
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
}

export const SessionThreadsPageView: FC<SessionThreadsPageViewProps> = ({
	session,
	threads,
	loading,
	hasNextPage,
	isFetchingNextPage,
	onFetchNextPage,
}) => {
	// calculate the total number of tool calls across all loaded threads
	const toolCallCount = threads.reduce(
		(acc, thread) => acc + (thread.agentic_actions?.length ?? 0),
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
				<aside className="md:w-64 md:shrink-0 px-3 py-2.5 border border-solid rounded-md flex flex-col gap-1">
					<h2 className="text-sm font-semibold flex items-center m-0">
						Session summary
						<SessionSummaryTooltip>
							<InfoIcon className="ml-2 text-content-secondary size-icon-sm" />
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
