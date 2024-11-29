import AlertTitle from "@mui/material/AlertTitle";
import LinearProgress from "@mui/material/LinearProgress";
import type {
	DAUsResponse,
	Entitlements,
	Experiments,
	SerpentOption,
} from "api/typesGenerated";
import {
	ActiveUserChart,
	ActiveUsersTitle,
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
	entitlements: Entitlements | undefined;
	readonly invalidExperiments: Experiments | string[];
	readonly safeExperiments: Experiments | string[];
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
	deploymentOptions,
	deploymentDAUs,
	deploymentDAUsError,
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
				{Boolean(deploymentDAUsError) && (
					<ErrorAlert error={deploymentDAUsError} />
				)}
				{deploymentDAUs && (
					<div css={{ marginBottom: 24, height: 200 }}>
						<ChartSection title={<ActiveUsersTitle interval="day" />}>
							<ActiveUserChart data={deploymentDAUs.entries} interval="day" />
						</ChartSection>
					</div>
				)}
				{licenseUtilizationPercentage && (
					<div css={{ marginBottom: 24, height: 200 }}>
						<ChartSection title="License Utilization">
							<LinearProgress
								variant="determinate"
								value={licenseUtilizationPercentage * 100}
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
					</div>
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
