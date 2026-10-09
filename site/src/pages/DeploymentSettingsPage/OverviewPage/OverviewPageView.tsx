import { Fragment } from "react";
import type {
	DAUsResponse,
	Experiment,
	SerpentOption,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useDeploymentOptions } from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";
import { UserEngagementChart } from "./UserEngagementChart";

type OverviewPageViewProps = {
	deploymentOptions: SerpentOption[];
	dailyActiveUsers: DAUsResponse | undefined;
	readonly invalidExperiments: readonly string[];
	readonly safeExperiments: readonly Experiment[];
};

export const OverviewPageView: React.FC<OverviewPageViewProps> = ({
	deploymentOptions,
	dailyActiveUsers,
	safeExperiments,
	invalidExperiments,
}) => {
	return (
		<>
			<SettingsHeader>
				<SettingsHeaderTitle>General</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Information about your Coder deployment.{" "}
					<SettingsHeaderDocsLink href={docs("/admin/setup")} />
				</SettingsHeaderDescription>
			</SettingsHeader>

			<div className="flex flex-col gap-8">
				<UserEngagementChart
					data={dailyActiveUsers?.entries.map((i) => ({
						date: i.date,
						users: i.amount,
					}))}
				/>
				{invalidExperiments.length > 0 && (
					<Alert
						severity="warning"
						actions={
							<Link
								href={docs("/reference/cli/server#--experiments")}
								target="_blank"
								rel="noreferrer"
							>
								View experiments docs
								<span className="sr-only"> (opens in new tab)</span>
							</Link>
						}
					>
						<AlertTitle>Some experiments aren't recognized</AlertTitle>
						<AlertDescription>
							These experiments have no effect:{" "}
							{invalidExperiments.map((it, index) => (
								<Fragment key={it}>
									{index > 0 && ", "}
									<code>{it}</code>
								</Fragment>
							))}
							. Remove them from your server configuration.
						</AlertDescription>
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
				<p className="m-0 text-sm text-content-secondary">
					Experiments lists the startup defaults. Runtime experiment rules can
					override them per user without a restart. See{" "}
					<Link
						href={docs(
							"/reference/feature-stages#target-experiments-at-runtime",
						)}
						target="_blank"
						rel="noreferrer"
					>
						the documentation
					</Link>
					.
				</p>
			</div>
		</>
	);
};
