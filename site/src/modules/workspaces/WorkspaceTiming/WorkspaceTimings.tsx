import type { ProvisionerTiming } from "api/typesGenerated";
import {
	calcTotalDuration,
	Chart,
	type Duration,
	type ChartProps,
	type Timing,
} from "./Chart/Chart";
import { useState, type FC } from "react";

// We control the stages to be displayed in the chart so we can set the correct
// colors and labels.
const provisioningStages = [
	{ name: "init" },
	{ name: "plan" },
	{ name: "graph" },
	{ name: "apply" },
];

type WorkspaceTimingsProps = {
	provisionerTimings: readonly ProvisionerTiming[];
};

type TimingView =
	| { type: "basic" }
	// The advanced view enables users to filter results based on the XAxis label
	| { type: "advanced"; selectedStage: string; parentSection: string };

export const WorkspaceTimings: FC<WorkspaceTimingsProps> = ({
	provisionerTimings,
}) => {
	const [view, setView] = useState<TimingView>({ type: "basic" });
	let data: ChartProps["data"] = [];

	if (view.type === "basic") {
		data = [
			{
				name: "provisioning",
				timings: provisioningStages.map((stage) => {
					// Get all the timing durations for a stage
					const durations = provisionerTimings
						.filter((t) => t.stage === stage.name)
						.map(extractDuration);

					// Calc the total duration
					const stageDuration = calcTotalDuration(durations);

					// Mount the timing data that is required by the chart
					const stageTiming: Timing = {
						label: stage.name,
						count: durations.length,
						...stageDuration,
					};
					return stageTiming;
				}),
			},
		];
	}

	if (view.type === "advanced") {
		data = [
			{
				name: `${view.selectedStage} stage`,
				timings: provisionerTimings
					.filter((t) => t.stage === view.selectedStage)
					.map((t) => {
						console.log("-> RESOURCE", t);
						return {
							label: t.resource,
							count: 0, // Resource timings don't have inner timings
							...extractDuration(t),
						} as Timing;
					}),
			},
		];
	}

	return (
		<Chart
			data={data}
			onBarClick={(stage, section) => {
				setView({
					type: "advanced",
					selectedStage: stage,
					parentSection: section,
				});
			}}
		/>
	);
};

const extractDuration = (t: ProvisionerTiming): Duration => {
	return {
		startedAt: new Date(t.started_at),
		endedAt: new Date(t.ended_at),
	};
};
