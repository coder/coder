import type { Interpolation, Theme } from "@emotion/react";
import type { ProvisionerTiming } from "api/typesGenerated";
import { type FC, useState } from "react";
import { type BaseTiming, combineTimings } from "./Chart/utils";
import { ResourcesChart } from "./ResourcesChart";
import { StagesChart, stages } from "./StagesChart";

type TimingView =
	| { name: "stages" }
	| {
			name: "resources";
			stage: string;
			category: string;
			filter: string;
	  };

type WorkspaceTimingsProps = {
	provisionerTimings: readonly ProvisionerTiming[];
};

export const WorkspaceTimings: FC<WorkspaceTimingsProps> = ({
	provisionerTimings,
}) => {
	const [view, setView] = useState<TimingView>({ name: "stages" });

	return (
		<div css={styles.panelBody}>
			{view.name === "stages" && (
				<StagesChart
					timings={stages.map((s) => {
						const stageTimings = provisionerTimings.filter(
							(t) => t.stage === s.name,
						);
						const combinedStageTiming = combineTimings(
							stageTimings.map(provisionerToBaseTiming),
						);
						return {
							...combinedStageTiming,
							name: s.name,
							category: s.category,
							resources: stageTimings.length,
						};
					})}
					onSelectStage={(t, category) => {
						setView({ name: "resources", stage: t.name, category, filter: "" });
					}}
				/>
			)}

			{view.name === "resources" && (
				<ResourcesChart
					timings={provisionerTimings
						.filter((t) => t.stage === view.stage)
						.map((t) => {
							return {
								...provisionerToBaseTiming(t),
								name: t.resource,
								source: t.source,
								action: t.action,
							};
						})}
					category={view.category}
					stage={view.stage}
					onBack={() => {
						setView({ name: "stages" });
					}}
				/>
			)}
		</div>
	);
};

const provisionerToBaseTiming = (
	provisioner: ProvisionerTiming,
): BaseTiming => {
	return {
		startedAt: new Date(provisioner.started_at),
		endedAt: new Date(provisioner.ended_at),
	};
};

const styles = {
	panelBody: {
		display: "flex",
		flexDirection: "column",
		height: "100%",
	},
} satisfies Record<string, Interpolation<Theme>>;
