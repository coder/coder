import type { ProvisionerTiming } from "api/typesGenerated";
import { Chart, type Duration, type Timing, duration } from "./Chart/Chart";
import { useState, type FC } from "react";
import type { Interpolation, Theme } from "@emotion/react";
import ChevronRight from "@mui/icons-material/ChevronRight";
import { YAxisSidePadding, YAxisWidth } from "./Chart/YAxis";
import { SearchField } from "components/SearchField/SearchField";

// TODO: Export provisioning stages from the BE to the generated types.
const provisioningStages = ["init", "plan", "graph", "apply"];

// TODO: Export actions from the BE to the generated types.
const colorsByActions: Record<string, Timing["color"]> = {
	create: {
		fill: "#022C22",
		border: "#BBF7D0",
	},
	delete: {
		fill: "#422006",
		border: "#FDBA74",
	},
	read: {
		fill: "#082F49",
		border: "#38BDF8",
	},
};

// The advanced view is an expanded view of the stage, allowing the user to see
// which resources within a stage are taking the most time. It supports resource
// filtering and displays bars with different colors representing various states
// such as created, deleted, etc.
type TimingView =
	| { name: "basic" }
	| {
			name: "advanced";
			selectedStage: string;
			parentSection: string;
			filter: string;
	  };

type WorkspaceTimingsProps = {
	provisionerTimings: readonly ProvisionerTiming[];
};

export const WorkspaceTimings: FC<WorkspaceTimingsProps> = ({
	provisionerTimings,
}) => {
	const [view, setView] = useState<TimingView>({ name: "basic" });

	return (
		<div css={styles.panelBody}>
			{view.name === "advanced" && (
				<div css={styles.toolbar}>
					<ul css={styles.breadcrumbs}>
						<li>
							<button
								type="button"
								css={styles.breadcrumbButton}
								onClick={() => {
									setView({ name: "basic" });
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
						value={view.filter}
						placeholder="Filter results..."
						onChange={(q: string) => {
							setView((v) => ({
								...v,
								filter: q,
							}));
						}}
					/>
				</div>
			)}

			<Chart
				data={selectChartData(view, provisionerTimings)}
				onBarClick={(stage, section) => {
					setView({
						name: "advanced",
						selectedStage: stage,
						parentSection: section,
						filter: "",
					});
				}}
			/>
		</div>
	);
};

export const selectChartData = (
	view: TimingView,
	timings: readonly ProvisionerTiming[],
) => {
	switch (view.name) {
		case "basic": {
			const groupedTimingsByStage = provisioningStages.map((stage) => {
				const durations = timings
					.filter((t) => t.stage === stage)
					.map(extractDuration);
				const stageDuration = duration(durations);
				const stageTiming: Timing = {
					label: stage,
					childrenCount: durations.length,
					...stageDuration,
				};
				return stageTiming;
			});

			return [
				{
					name: "provisioning",
					timings: groupedTimingsByStage,
				},
			];
		}

		case "advanced": {
			const selectedStageTimings = timings
				.filter(
					(t) =>
						t.stage === view.selectedStage && t.resource.includes(view.filter),
				)
				.map((t) => {
					return {
						label: `${t.resource}.${t.action}`,
						color: colorsByActions[t.action],
						childrenCount: 0, // Resource timings don't have inner timings
						...extractDuration(t),
					} as Timing;
				});

			return [
				{
					name: `${view.selectedStage} stage`,
					timings: selectedStageTimings,
				},
			];
		}
	}
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
		flexShrink: 0,

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
		width: "100%",

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
