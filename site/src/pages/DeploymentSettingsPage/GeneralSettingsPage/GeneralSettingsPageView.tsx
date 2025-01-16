import AlertTitle from "@mui/material/AlertTitle";
import LinearProgress from "@mui/material/LinearProgress";
import type {
	DAUsResponse,
	Entitlements,
	Experiments,
	SerpentOption,
} from "api/typesGenerated";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";
import { Alert } from "../../../components/Alert/Alert";
import OptionsTable from "../OptionsTable";
import { ChartSection } from "./ChartSection";
import { UserEngagementChart } from "./UserEngagementChart";

export type GeneralSettingsPageViewProps = {
	deploymentOptions: SerpentOption[];
	dailyActiveUsers: DAUsResponse | undefined;
	entitlements: Entitlements | undefined;
	readonly invalidExperiments: Experiments | string[];
	readonly safeExperiments: Experiments | string[];
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
	deploymentOptions,
	dailyActiveUsers,
	entitlements,
	safeExperiments,
	invalidExperiments,
}) => {
	const licenseUtilizationPercentage =
		entitlements?.features?.user_limit?.actual &&
		entitlements?.features?.user_limit?.limit
			? entitlements.features.user_limit.actual /
				entitlements.features.user_limit.limit
			: undefined;
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
				{licenseUtilizationPercentage && (
					<ChartSection title="License Utilization">
						<LinearProgress
							variant="determinate"
							value={Math.min(licenseUtilizationPercentage * 100, 100)}
							color={
								licenseUtilizationPercentage < 0.9
									? "primary"
									: licenseUtilizationPercentage < 1
										? "warning"
										: "error"
							}
							css={{
								height: 24,
								borderRadius: 4,
								marginBottom: 8,
							}}
						/>
						<span
							css={{
								fontSize: "0.75rem",
								display: "block",
								textAlign: "right",
							}}
						>
							{Math.round(licenseUtilizationPercentage * 100)}% used (
							{entitlements!.features.user_limit.actual}/
							{entitlements!.features.user_limit.limit} users)
						</span>
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
