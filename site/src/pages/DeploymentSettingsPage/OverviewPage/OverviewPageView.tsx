import type {
	DAUsResponse,
	Experiment,
	SerpentOption,
} from "api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";
import OptionsTable from "../OptionsTable";
import { UserEngagementChart } from "./UserEngagementChart";

type OverviewPageViewProps = {
	deploymentOptions: SerpentOption[];
	dailyActiveUsers: DAUsResponse | undefined;
	readonly safeExperiments: readonly Experiment[];
};

export const OverviewPageView: FC<OverviewPageViewProps> = ({
	deploymentOptions,
	dailyActiveUsers,
	safeExperiments,
}) => {
	return (
		<>
			<SettingsHeader
				actions={<SettingsHeaderDocsLink href={docs("/admin/setup")} />}
			>
				<SettingsHeaderTitle>General</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Information about your Coder deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<Stack spacing={4}>
				<UserEngagementChart
					data={dailyActiveUsers?.entries.map((i) => ({
						date: i.date,
						users: i.amount,
					}))}
				/>
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
