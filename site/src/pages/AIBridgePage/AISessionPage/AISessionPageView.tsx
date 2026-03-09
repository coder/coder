import type { AIBridgeSessionResponse } from "api/typesGenerated";
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
import { roundTokenDisplay } from "../utils";
import { AISessionTable, type AISessionTableProps } from "./AISessionTable";

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
					<Link href="TODO docs page" target="_blank">
						View session terminology
					</Link>
				</p>
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

export interface AISessionPageViewProps {
	sessionId: string;
	session: AIBridgeSessionResponse | undefined;
	loading: boolean;
}

export const AISessionPageView: FC<AISessionPageViewProps> = ({
	session,
	loading,
}) => {
	const toolCallCount =
		session?.threads.reduce(
			(acc, thread) => acc + thread.agentic_actions.length,
			0,
		) ?? 0;

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
			<main className="flex flex-col md:flex-row gap-6">
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
						<AISessionTable
							sessionId={session.id}
							startTime={new Date(session.started_at)}
							endTime={
								session.ended_at ? new Date(session.ended_at) : undefined
							}
							initiator={session.initiator}
							client={session.client}
							provider={session.provider}
							inputTokens={session.token_usage_summary.input_tokens}
							outputTokens={session.token_usage_summary.output_tokens}
							threadCount={session.threads.length}
							toolCallCount={toolCallCount}
						/>
					)}
				</aside>
				<div className="">
					Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do
					eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad
					minim veniam, quis nostrud exercitation ullamco laboris nisi ut
					aliquip ex ea commodo consequat. Duis aute irure dolor in
					reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla
					pariatur. Excepteur sint occaecat cupidatat non proident, sunt in
					culpa qui officia deserunt mollit anim id est laborum.
				</div>
			</main>
		</>
	);
};
