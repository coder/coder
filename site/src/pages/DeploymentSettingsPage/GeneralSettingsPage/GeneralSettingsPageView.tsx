import AlertTitle from "@mui/material/AlertTitle";
import type {
	DAUsResponse,
	Experiments,
	GetUserStatusChangesResponse,
	SerpentOption,
} from "api/typesGenerated";
import {
	ActiveUserChart,
	ActiveUsersTitle,
	type DataSeries,
	UserStatusTitle,
} from "components/ActiveUserChart/ActiveUserChart";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";
import { Alert } from "../../../components/Alert/Alert";
import OptionsTable from "../OptionsTable";
import { ChartSection } from "./ChartSection";

export type GeneralSettingsPageViewProps = {
	deploymentOptions: SerpentOption[];
	deploymentDAUs?: DAUsResponse;
	deploymentDAUsError: unknown;
	userStatusCountsOverTime?: GetUserStatusChangesResponse;
	activeUserLimit?: number;
	readonly invalidExperiments: Experiments | string[];
	readonly safeExperiments: Experiments | string[];
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
	deploymentOptions,
	deploymentDAUs,
	deploymentDAUsError,
	userStatusCountsOverTime,
	activeUserLimit,
	safeExperiments,
	invalidExperiments,
}) => {
	const colors: Record<string, string> = {
		active: "green",
		dormant: "grey",
		deleted: "red",
	};
	let series: DataSeries[] = [];
	if (userStatusCountsOverTime?.status_counts) {
		series = Object.entries(userStatusCountsOverTime.status_counts).map(
			([status, counts]) => ({
				label: status,
				data: counts.map((count) => ({
					date: count.date.toString(),
					amount: count.count,
				})),
				color: colors[status],
			}),
		);
	}
	return (
		<>
			<SettingsHeader
				title="General"
				description="Information about your Coder deployment."
				docsHref={docs("/admin/setup")}
			/>
			<Stack spacing={4}>
				{Boolean(deploymentDAUsError) && (
					<ErrorAlert error={deploymentDAUsError} />
				)}
				{series.length && (
					<ChartSection title={<UserStatusTitle interval="day" />}>
						<ActiveUserChart
							series={series}
							userLimit={activeUserLimit}
							interval="day"
						/>
					</ChartSection>
				)}
				{deploymentDAUs && (
					<ChartSection title={<ActiveUsersTitle interval="day" />}>
						<ActiveUserChart
							series={[{ label: "Daily", data: deploymentDAUs.entries }]}
							interval="day"
						/>
					</ChartSection>
				)}
				{invalidExperiments.length > 0 && (
					<Alert severity="warning">
						<AlertTitle>Invalid experiments in use:</AlertTitle>
						<ul>
							{invalidExperiments.map((it) => (
								<li key={it}>
									<pre>{it}</pre>
								</li>
							))}
						</ul>
						It is recommended that you remove these experiments from your
						configuration as they have no effect. See{" "}
						<a
							href="https://coder.com/docs/cli/server#--experiments"
							target="_blank"
							rel="noreferrer"
						>
							the documentation
						</a>{" "}
						for more details.
					</Alert>
				)}
				<OptionsTable
					options={useDeploymentOptions(
						deploymentOptions,
						"Access URL",
						"Wildcard Access URL",
						"Experiments",
					)}
					additionalValues={safeExperiments}
				/>
			</Stack>
		</>
	);
};
