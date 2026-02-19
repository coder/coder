import type {
	DAUsResponse,
	Experiment,
	SerpentOption,
} from "api/typesGenerated";
import { Link } from "components/Link/Link";
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
import { Alert, AlertTitle } from "../../../components/Alert/Alert";
import type { ConfigWarning } from "../ConfigAuditPage/configWarnings";
import OptionsTable from "../OptionsTable";
import { UserEngagementChart } from "./UserEngagementChart";

type OverviewPageViewProps = {
	deploymentOptions: SerpentOption[];
	dailyActiveUsers: DAUsResponse | undefined;
	readonly invalidExperiments: readonly string[];
	readonly safeExperiments: readonly Experiment[];
	readonly configWarnings: readonly ConfigWarning[];
};

export const OverviewPageView: FC<OverviewPageViewProps> = ({
	deploymentOptions,
	dailyActiveUsers,
	safeExperiments,
	invalidExperiments,
	configWarnings,
}) => {
	const errors = configWarnings.filter((w) => w.severity === "error");
	const warnings = configWarnings.filter((w) => w.severity === "warning");

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
				{(errors.length > 0 || warnings.length > 0) && (
					<div className="flex flex-col gap-2">
						{errors.map((w) => (
							<Alert key={w.option} severity="error" prominent>
								<code className="font-semibold">{w.option}</code>: {w.message}
							</Alert>
						))}
						{warnings.map((w) => (
							<Alert key={w.option} severity="warning" prominent>
								<code className="font-semibold">{w.option}</code>: {w.message}
							</Alert>
						))}
					</div>
				)}
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
							href="https://coder.com/docs/reference/cli/server#--experiments"
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
