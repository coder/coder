import type { ProvisionerTiming } from "api/typesGenerated";
import {
	Chart,
	type Duration,
	type ChartProps,
	type Timing,
	duration,
} from "./Chart/Chart";
import { useState, type FC } from "react";
import type { Interpolation, Theme } from "@emotion/react";
import ChevronRight from "@mui/icons-material/ChevronRight";
import { YAxisSidePadding, YAxisWidth } from "./Chart/YAxis";
import { SearchField } from "components/SearchField/SearchField";

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
					const stageDuration = duration(durations);

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
		<div css={styles.panelBody}>
			{view.type === "advanced" && (
				<div css={styles.toolbar}>
					<ul css={styles.breadcrumbs}>
						<li>
							<button
								type="button"
								css={styles.breadcrumbButton}
								onClick={() => {
									setView({ type: "basic" });
								}}
							>
								{view.parentSection}
							</button>
						</li>
						<li role="presentation">
							<ChevronRight />
						</li>
						<li>{view.selectedStage}</li>
					</ul>

					<SearchField
						css={styles.searchField}
						placeholder="Filter results..."
						onChange={(q: string) => {}}
					/>
				</div>
			)}

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
		</div>
	);
};

const extractDuration = (t: ProvisionerTiming): Duration => {
	return {
		startedAt: new Date(t.started_at),
		endedAt: new Date(t.ended_at),
	};
};

const styles = {
	panelBody: {
		display: "flex",
		flexDirection: "column",
		height: "100%",
	},
	toolbar: (theme) => ({
		borderBottom: `1px solid ${theme.palette.divider}`,
		fontSize: 12,
		display: "flex",
	}),
	breadcrumbs: (theme) => ({
		listStyle: "none",
		margin: 0,
		width: YAxisWidth,
		padding: YAxisSidePadding,
		display: "flex",
		alignItems: "center",
		gap: 4,
		lineHeight: 1,

		"& li": {
			display: "block",

			"&[role=presentation]": {
				lineHeight: 0,
			},
		},

		"& li:first-child": {
			color: theme.palette.text.secondary,
		},

		"& li[role=presentation]": {
			color: theme.palette.text.secondary,

			"& svg": {
				width: 14,
				height: 14,
			},
		},
	}),
	breadcrumbButton: (theme) => ({
		background: "none",
		border: "none",
		fontSize: "inherit",
		color: "inherit",
		cursor: "pointer",

		"&:hover": {
			color: theme.palette.text.primary,
		},
	}),
	searchField: (theme) => ({
		"& fieldset": {
			border: 0,
			borderRadius: 0,
			borderLeft: `1px solid ${theme.palette.divider} !important`,
		},

		"& .MuiInputBase-root": {
			height: "100%",
			fontSize: 12,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
