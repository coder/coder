import type { Interpolation, Theme } from "@emotion/react";
import type { AgentScriptTiming, ProvisionerTiming } from "api/typesGenerated";
import { type FC, useState } from "react";
import { type TimeRange, mergeTimeRanges } from "./Chart/utils";
import { ResourcesChart } from "./ResourcesChart";
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
	provisionerTimings: readonly ProvisionerTiming[];
	agentScriptTimings: readonly AgentScriptTiming[];
};

export const WorkspaceTimings: FC<WorkspaceTimingsProps> = ({
	provisionerTimings,
	agentScriptTimings,
}) => {
	const [view, setView] = useState<TimingView>({ name: "default" });
	const timings = [...provisionerTimings, ...agentScriptTimings];

	return (
		<div css={styles.panelBody}>
			{view.name === "default" && (
				<StagesChart
					timings={stages.map((s) => {
						const stageTimings = timings.filter((t) => t.stage === s.name);
						const stageRange =
							stageTimings.length === 0
								? undefined
								: mergeTimeRanges(stageTimings.map(extractRange));
						return {
							range: stageRange,
							name: s.name,
							categoryID: s.categoryID,
							resources: stageTimings.length,
							error: stageTimings.some(
								(t) => "status" in t && t.status === "exit_failure",
							),
						};
					})}
					onSelectStage={(t, category) => {
						setView({ name: "detailed", stage: t.name, category, filter: "" });
					}}
				/>
			)}

			{view.name === "detailed" && view.category.id === "provisioning" && (
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

			{view.name === "detailed" && view.category.id === "workspaceBoot" && (
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

const styles = {
	panelBody: {
		display: "flex",
		flexDirection: "column",
		height: "100%",
	},
} satisfies Record<string, Interpolation<Theme>>;
