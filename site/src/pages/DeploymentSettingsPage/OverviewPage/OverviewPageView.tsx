import AlertTitle from "@mui/material/AlertTitle";
import type {
	DAUsResponse,
	Experiments,
	SerpentOption,
} from "api/typesGenerated";
import { Link } from "components/Link/Link";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";
import { Alert } from "../../../components/Alert/Alert";
import OptionsTable from "../OptionsTable";
import { UserEngagementChart } from "./UserEngagementChart";

export type GeneralSettingsPageViewProps = {
	deploymentOptions: SerpentOption[];
	dailyActiveUsers: DAUsResponse | undefined;
	readonly invalidExperiments: Experiments | string[];
	readonly safeExperiments: Experiments | string[];
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
	deploymentOptions,
	dailyActiveUsers,
	safeExperiments,
	invalidExperiments,
}) => {
	return (
		<>
			<SettingsHeader
				title="General"
				description="Information about your Coder deployment."
				docsHref={docs("/admin/setup")}
			/>
			<Stack spacing={4}>
				<UserEngagementChart
					data={dailyActiveUsers?.entries.map((i) => ({
						date: i.date,
						users: i.amount,
					}))}
				/>
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
						<Link
							href="https://coder.com/docs/cli/server#--experiments"
							target="_blank"
							rel="noreferrer"
						>
							the documentation
						</Link>{" "}
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
