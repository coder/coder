import type { Interpolation, Theme } from "@emotion/react";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import KeyboardArrowUp from "@mui/icons-material/KeyboardArrowUp";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import Skeleton from "@mui/material/Skeleton";
import type {
	AgentConnectionTiming,
	AgentScriptTiming,
	ProvisionerTiming,
} from "api/typesGenerated";
import sortBy from "lodash/sortBy";
import uniqBy from "lodash/uniqBy";
import { type FC, useState } from "react";
import { type TimeRange, calcDuration, mergeTimeRanges } from "./Chart/utils";
import { ResourcesChart, isCoderResource } from "./ResourcesChart";
import { ScriptsChart } from "./ScriptsChart";
import {
	type Stage,
	StagesChart,
	agentStages,
	provisioningStages,
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
	// This is a workaround to deal with the BE returning multiple timings for a
	// single agent script when it should return only one. Reference:
	// https://github.com/coder/coder/issues/15413#issuecomment-2493663571
	const uniqScriptTimings = uniqBy(
		sortBy(agentScriptTimings, (t) => new Date(t.started_at).getTime() * -1),
		(t) => t.display_name,
	);
	const timings = [
		...provisionerTimings,
		...uniqScriptTimings,
		...agentConnectionTimings,
	].sort((a, b) => {
		return new Date(a.started_at).getTime() - new Date(b.started_at).getTime();
	});

	const [isOpen, setIsOpen] = useState(defaultIsOpen);

	// If any of the timings are empty, we are still loading the data. They can be
	// filled in different moments.
	const isLoading = [
		provisionerTimings,
		agentScriptTimings,
		agentConnectionTimings,
	].some((t) => t.length === 0);

	// Each agent connection timing is a stage in the timeline to make it easier
	// to users to see the timing for connection and the other scripts.
	const agentStageLabels = Array.from(
		new Set(
			agentConnectionTimings.map((t) => `agent (${t.workspace_agent_name})`),
		),
	);

	const stages = [
		...provisioningStages,
		...agentStageLabels.flatMap((a) => agentStages(a)),
	];

	const displayProvisioningTime = () => {
		const totalRange = mergeTimeRanges(timings.map(toTimeRange));
		const totalDuration = calcDuration(totalRange);
		return humanizeDuration(totalDuration);
	};

	return (
		<div css={styles.collapse}>
			<Button
				disabled={isLoading}
				variant="text"
				css={styles.collapseTrigger}
				onClick={() => setIsOpen((o) => !o)}
			>
				{isOpen ? (
					<KeyboardArrowUp css={{ fontSize: 16, marginRight: 16 }} />
				) : (
					<KeyboardArrowDown css={{ fontSize: 16, marginRight: 16 }} />
				)}
				<span>Build timeline</span>
				<span
					css={(theme) => ({
						marginLeft: "auto",
						color: theme.palette.text.secondary,
					})}
				>
					{isLoading ? (
						<Skeleton variant="text" width={40} height={16} />
					) : (
						displayProvisioningTime()
					)}
				</span>
			</Button>
			{!isLoading && (
				<Collapse in={isOpen}>
					<div css={styles.collapseBody}>
						{view.name === "default" && (
							<StagesChart
								timings={stages.map((s) => {
									const stageTimings = timings.filter(
										(t) => t.stage === s.name,
									);
									const stageRange =
										stageTimings.length === 0
											? undefined
											: mergeTimeRanges(stageTimings.map(toTimeRange));

									// Prevent users from inspecting internal coder resources in
									// provisioner timings because they were not useful to the
									// user and would add noise.
									const visibleResources = stageTimings.filter((t) => {
										const isProvisionerTiming = "resource" in t;
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
										timings={agentScriptTimings
											.filter((t) => t.stage === view.stage.name)
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

const humanizeDuration = (durationMs: number): string => {
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

const styles = {
	collapse: (theme) => ({
		borderRadius: 8,
		border: `1px solid ${theme.palette.divider}`,
		backgroundColor: theme.palette.background.default,
	}),
	collapseTrigger: {
		background: "none",
		border: 0,
		padding: 16,
		color: "inherit",
		width: "100%",
		display: "flex",
		alignItems: "center",
		height: 57,
		fontSize: 14,
		fontWeight: 500,
		cursor: "pointer",
	},
	collapseBody: (theme) => ({
		borderTop: `1px solid ${theme.palette.divider}`,
		display: "flex",
		flexDirection: "column",
		height: 420,
	}),
} satisfies Record<string, Interpolation<Theme>>;
