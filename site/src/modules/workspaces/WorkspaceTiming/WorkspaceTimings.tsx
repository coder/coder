import Collapse from "@mui/material/Collapse";
import Skeleton from "@mui/material/Skeleton";
import type {
	AgentConnectionTiming,
	AgentScriptTiming,
	ProvisionerTiming,
} from "api/typesGenerated";
import sortBy from "lodash/sortBy";
import uniqBy from "lodash/uniqBy";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import { type FC, useState } from "react";
import {
	calcDuration,
	formatTime,
	mergeTimeRanges,
	type TimeRange,
} from "./Chart/utils";
import {
	isCoderResource,
	isStageBoundary,
	ResourcesChart,
} from "./ResourcesChart";
import { ScriptsChart } from "./ScriptsChart";
import {
	agentStages,
	provisioningStages,
	type Stage,
	StagesChart,
} from "./StagesChart";

type TimingView =
	| { name: "default" }
	| {
			name: "detailed";
			stage: Stage;
			filter: string;
	  };

type WorkspaceTimingsProps = {
	defaultIsOpen?: boolean;
	provisionerTimings: readonly ProvisionerTiming[] | undefined;
	agentScriptTimings: readonly AgentScriptTiming[] | undefined;
	agentConnectionTimings: readonly AgentConnectionTiming[] | undefined;
};

export const WorkspaceTimings: FC<WorkspaceTimingsProps> = ({
	provisionerTimings = [],
	agentScriptTimings = [],
	agentConnectionTimings = [],
	defaultIsOpen = false,
}) => {
	const [view, setView] = useState<TimingView>({ name: "default" });
	const [isOpen, setIsOpen] = useState(defaultIsOpen);

	// If any of the timings are empty, we are still loading the data. They can be
	// filled in different moments.
	const isLoading = [
		provisionerTimings,
		agentConnectionTimings,
		// agentScriptTimings might be an empty array if there are no scripts to run.
		// Only provisionerTimings and agentConnectionTimings are guaranteed to have
		// at least one entry.
	].some((t) => t.length === 0);

	// This is a workaround to deal with the BE returning multiple timings for a
	// single agent script when it should return only one. Reference:
	// https://github.com/coder/coder/issues/15413#issuecomment-2493663571
	const uniqScriptTimings = sortBy(
		uniqBy(
			sortBy(agentScriptTimings, (t) => t.started_at).reverse(),
			(t) => `${t.workspace_agent_id}:${t.display_name}`,
		),
		(t) => t.started_at,
	);

	// Combine agent timings for filtering by agent ID.
	const agentTimings = [...agentConnectionTimings, ...uniqScriptTimings];

	// Each agent connection timing is a stage in the timeline to make it easier
	// to users to see the timing for connection and the other scripts.
	const agents = uniqBy(agentConnectionTimings, (t) => t.workspace_agent_id);

	const stages = [
		...provisioningStages,
		...agents.flatMap((agent) =>
			agentStages(
				`agent (${agent.workspace_agent_name})`,
				agent.workspace_agent_id,
			),
		),
	];

	const displayProvisioningTime = () => {
		const allTimings = [...provisionerTimings, ...agentTimings];
		const totalRange = mergeTimeRanges(allTimings.map(toTimeRange));
		const totalDuration = calcDuration(totalRange);
		return formatTime(totalDuration);
	};

	return (
		<div className="rounded-lg border border-border bg-surface-primary">
			<button
				type="button"
				disabled={isLoading}
				className="flex items-center p-4 w-full bg-transparent border-0 text-content-secondary text-sm leading-none font-medium cursor-pointer"
				onClick={() => setIsOpen((o) => !o)}
			>
				{isOpen ? (
					<ChevronUpIcon className="size-4 mr-4" />
				) : (
					<ChevronDownIcon className="size-4 mr-4" />
				)}
				<span>Build timeline</span>
				<span className="ml-auto">
					{isLoading ? (
						<Skeleton variant="text" width={40} height={16} />
					) : (
						displayProvisioningTime()
					)}
				</span>
			</button>
			{!isLoading && (
				<Collapse in={isOpen}>
					<div className="border-t border-border flex flex-col h-[var(--collapse-body-height,420px)]">
						{view.name === "default" && (
							<StagesChart
								timings={stages.map((s) => {
									const stageTimings = s.agentId
										? agentTimings.filter(
												(t) =>
													t.stage === s.name &&
													t.workspace_agent_id === s.agentId,
											)
										: provisionerTimings.filter((t) => t.stage === s.name);
									const stageRange =
										stageTimings.length === 0
											? undefined
											: mergeTimeRanges(stageTimings.map(toTimeRange));

									// Prevent users from inspecting internal coder resources in
									// provisioner timings because they were not useful to the
									// user and would add noise.
									const visibleResources = stageTimings.filter((t) => {
										const isProvisionerTiming = "resource" in t;

										// StageBoundaries are being drawn on the total timeline.
										// Do not show them as discrete resources inside the stage view.
										if (isProvisionerTiming && isStageBoundary(t.resource)) {
											return false;
										}

										return isProvisionerTiming
											? !isCoderResource(t.resource)
											: true;
									});

									return {
										stage: s,
										range: stageRange,
										visibleResources: visibleResources.length,
										error: stageTimings.some(
											(t) => "status" in t && t.status === "exit_failure",
										),
									};
								})}
								onSelectStage={(stage) => {
									setView({
										stage,
										name: "detailed",
										filter: "",
									});
								}}
							/>
						)}

						{view.name === "detailed" && (
							<>
								{view.stage.section === "provisioning" && (
									<ResourcesChart
										timings={provisionerTimings
											.filter((t) => t.stage === view.stage.name)
											.map((t) => ({
												range: toTimeRange(t),
												name: t.resource,
												source: t.source,
												action: t.action,
											}))}
										stage={view.stage}
										onBack={() => {
											setView({ name: "default" });
										}}
									/>
								)}

								{view.stage.name === "start" && (
									<ScriptsChart
										timings={uniqScriptTimings
											.filter(
												(t) =>
													t.stage === view.stage.name &&
													t.workspace_agent_id === view.stage.agentId,
											)
											.map((t) => {
												return {
													range: toTimeRange(t),
													name: t.display_name,
													status: t.status,
													exitCode: t.exit_code,
												};
											})}
										stage={view.stage}
										onBack={() => {
											setView({ name: "default" });
										}}
									/>
								)}
							</>
						)}
					</div>
				</Collapse>
			)}
		</div>
	);
};

const toTimeRange = (timing: {
	started_at: string;
	ended_at: string;
}): TimeRange => {
	return {
		startedAt: new Date(timing.started_at),
		endedAt: new Date(timing.ended_at),
	};
};

const _humanizeDuration = (durationMs: number): string => {
	const seconds = Math.floor(durationMs / 1000);
	const minutes = Math.floor(seconds / 60);
	const hours = Math.floor(minutes / 60);

	if (hours > 0) {
		return `${hours.toLocaleString()}h ${(minutes % 60).toLocaleString()}m`;
	}

	if (minutes > 0) {
		return `${minutes.toLocaleString()}m ${(seconds % 60).toLocaleString()}s`;
	}

	return `${seconds.toLocaleString()}s`;
};
