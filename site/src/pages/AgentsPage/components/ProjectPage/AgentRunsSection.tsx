import { cn } from "cn";
import { useQuery, useQueryClient } from "react-query";
import { Link } from "react-router";
import {
	chatProjectRuns,
	chatProjectTriggers,
} from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectRun,
	ChatProjectTrigger,
} from "#/api/typesGenerated";
import { relativeTime } from "#/utils/time";
import { buildAgentChatPath } from "../../utils/navigation";
import { getChatStatusDisplay } from "../ChatsSidebar/tree/statusConfig";
import {
	SectionError,
	SectionLoadState,
	SettingsCard,
	SettingsCardEmptyRow,
	SettingsCardRow,
	SettingsCardRowIcon,
	SettingsCardRowText,
	SettingsSection,
} from "./ProjectSection";

/** What started a run. */
type RunSource = "automation" | "delegation";

const runSourceCopy: Record<
	RunSource,
	{ readonly title: string; readonly empty: string }
> = {
	automation: {
		title: "Automated runs",
		empty: "No automated runs yet. Chats started by a trigger show up here.",
	},
	delegation: {
		title: "Delegated runs",
		empty:
			"No delegated runs yet. Chats that other agents hand to this agent show up here.",
	},
};

const isRunFromSource = (run: ChatProjectRun, source: RunSource): boolean =>
	source === "automation"
		? Boolean(run.trigger_id)
		: Boolean(run.parent_chat_id) && !run.trigger_id;

type AgentRunsSectionProps = {
	readonly project: ChatProject;
	readonly source: RunSource;
};

/** Loads and shows the chats of one run source in an agent, newest first. */
export const AgentRunsSection: React.FC<AgentRunsSectionProps> = ({
	project,
	source,
}) => {
	const queryClient = useQueryClient();
	const runsQuery = useQuery(chatProjectRuns(project));
	const triggersQuery = useQuery(chatProjectTriggers(queryClient, project));
	return (
		<AgentRunsSectionView
			source={source}
			runs={runsQuery.data?.filter((run) => isRunFromSource(run, source))}
			triggers={triggersQuery.data ?? []}
			error={runsQuery.error}
			onRetry={() => void runsQuery.refetch()}
		/>
	);
};

type RunOutcome = "succeeded" | "failed" | "inProgress";

const runOutcome = (run: ChatProjectRun): RunOutcome => {
	switch (run.status) {
		case "waiting":
			return "succeeded";
		case "error":
			return "failed";
		default:
			return "inProgress";
	}
};

const runOriginLabel = (
	run: ChatProjectRun,
	triggerName: string | undefined,
): string => {
	if (run.trigger_id) {
		return triggerName
			? `Trigger: ${triggerName}`
			: "Started by a webhook trigger";
	}
	return "Delegated by another agent";
};

type AgentRunsSectionViewProps = {
	readonly source: RunSource;
	/** `undefined` until the first load finishes. */
	readonly runs: readonly ChatProjectRun[] | undefined;
	readonly triggers: readonly ChatProjectTrigger[];
	/** Shown above the runs when a refetch fails, or in place of them on first load. */
	readonly error: unknown;
	readonly onRetry: () => void;
};

/** Summarizes run outcomes and lists the runs. */
export const AgentRunsSectionView: React.FC<AgentRunsSectionViewProps> = ({
	source,
	runs,
	triggers,
	error,
	onRetry,
}) => {
	const counts = { succeeded: 0, failed: 0, inProgress: 0 };
	for (const run of runs ?? []) {
		counts[runOutcome(run)]++;
	}

	return (
		<SettingsSection
			headingLevel={2}
			title={runSourceCopy[source].title}
			actions={
				runs &&
				runs.length > 0 && (
					<p className="m-0 text-xs text-content-secondary">
						{counts.succeeded} succeeded, {counts.failed} failed,{" "}
						{counts.inProgress} in progress
					</p>
				)
			}
		>
			{runs === undefined ? (
				<SectionLoadState error={error} onRetry={onRetry} />
			) : (
				<>
					{Boolean(error) && <SectionError error={error} onRetry={onRetry} />}
					<SettingsCard>
						{runs.length === 0 && (
							<SettingsCardEmptyRow>
								{runSourceCopy[source].empty}
							</SettingsCardEmptyRow>
						)}
						{runs.map((run) => (
							<RunRow
								key={run.chat_id}
								run={run}
								triggerName={
									triggers.find((t) => t.id === run.trigger_id)?.name
								}
							/>
						))}
					</SettingsCard>
				</>
			)}
		</SettingsSection>
	);
};

type RunRowProps = {
	readonly run: ChatProjectRun;
	readonly triggerName: string | undefined;
};

const RunRow: React.FC<RunRowProps> = ({ run, triggerName }) => {
	const outcome = runOutcome(run);
	const status = getChatStatusDisplay(run.status);
	const StatusIcon = status.icon;
	const outcomeLabels: Record<RunOutcome, string> = {
		succeeded: "Succeeded",
		failed: "Failed",
		inProgress: status.label,
	};

	return (
		<SettingsCardRow>
			<SettingsCardRowIcon>
				<StatusIcon aria-hidden className={cn("shrink-0", status.className)} />
			</SettingsCardRowIcon>
			<SettingsCardRowText
				title={
					<Link
						to={buildAgentChatPath({ chatId: run.chat_id })}
						className="text-content-primary no-underline hover:underline"
					>
						{run.title || "Untitled"}
					</Link>
				}
				description={`${outcomeLabels[outcome]}, ${runOriginLabel(run, triggerName)}${run.last_error ? `: ${run.last_error}` : ""}`}
			/>
			<time
				dateTime={run.created_at}
				className="shrink-0 text-xs text-content-secondary"
			>
				{relativeTime(run.created_at)}
			</time>
		</SettingsCardRow>
	);
};
