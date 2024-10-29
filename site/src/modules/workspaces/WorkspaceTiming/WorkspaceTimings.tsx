import type { Interpolation, Theme } from "@emotion/react";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import KeyboardArrowUp from "@mui/icons-material/KeyboardArrowUp";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import Skeleton from "@mui/material/Skeleton";
import type { AgentScriptTiming, ProvisionerTiming } from "api/typesGenerated";
import { type FC, useState } from "react";
import { type TimeRange, calcDuration, mergeTimeRanges } from "./Chart/utils";
import { ResourcesChart, isCoderResource } from "./ResourcesChart";
import { ScriptsChart } from "./ScriptsChart";
import { type StageCategory, StagesChart, stages } from "./StagesChart";

type TimingView =
	| { name: "default" }
	| {
			name: "detailed";
			stage: string;
			category: StageCategory;
			filter: string;
	  };

type WorkspaceTimingsProps = {
	defaultIsOpen?: boolean;
	provisionerTimings: readonly ProvisionerTiming[] | undefined;
	agentScriptTimings: readonly AgentScriptTiming[] | undefined;
};

export const WorkspaceTimings: FC<WorkspaceTimingsProps> = ({
	provisionerTimings = [],
	agentScriptTimings = [],
	defaultIsOpen = false,
}) => {
	const [view, setView] = useState<TimingView>({ name: "default" });
	const timings = [...provisionerTimings, ...agentScriptTimings];
	const [isOpen, setIsOpen] = useState(defaultIsOpen);
	const isLoading = timings.length === 0;

	const displayProvisioningTime = () => {
		const totalRange = mergeTimeRanges(timings.map(extractRange));
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
											: mergeTimeRanges(stageTimings.map(extractRange));

									// Prevent users from inspecting internal coder resources in
									// provisioner timings.
									const visibleResources = stageTimings.filter((t) => {
										const isProvisionerTiming = "resource" in t;
										return isProvisionerTiming
											? !isCoderResource(t.resource)
											: true;
									});

									return {
										range: stageRange,
										name: s.name,
										categoryID: s.categoryID,
										visibleResources: visibleResources.length,
										error: stageTimings.some(
											(t) => "status" in t && t.status === "exit_failure",
										),
									};
								})}
								onSelectStage={(t, category) => {
									setView({
										name: "detailed",
										stage: t.name,
										category,
										filter: "",
									});
								}}
							/>
						)}

						{view.name === "detailed" &&
							view.category.id === "provisioning" && (
								<ResourcesChart
									timings={provisionerTimings
										.filter((t) => t.stage === view.stage)
										.map((t) => {
											return {
												range: extractRange(t),
												name: t.resource,
												source: t.source,
												action: t.action,
											};
										})}
									category={view.category}
									stage={view.stage}
									onBack={() => {
										setView({ name: "default" });
									}}
								/>
							)}

						{view.name === "detailed" &&
							view.category.id === "workspaceBoot" && (
								<ScriptsChart
									timings={agentScriptTimings
										.filter((t) => t.stage === view.stage)
										.map((t) => {
											return {
												range: extractRange(t),
												name: t.display_name,
												status: t.status,
												exitCode: t.exit_code,
											};
										})}
									category={view.category}
									stage={view.stage}
									onBack={() => {
										setView({ name: "default" });
									}}
								/>
							)}
					</div>
				</Collapse>
			)}
		</div>
	);
};

const extractRange = (
	timing: ProvisionerTiming | AgentScriptTiming,
): TimeRange => {
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
