import type { FC } from "react";
import type { SerpentOption } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { PaywallAIGovernance } from "#/components/Paywall/PaywallAIGovernance";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { deploymentGroupHasParent } from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

type AIGovernanceSettingsPageViewProps = {
	options: SerpentOption[];
	featureAIBridgeEntitled: boolean;
	featureAIBridgeEnabled: boolean;
};

export const AIGovernanceSettingsPageView: FC<
	AIGovernanceSettingsPageViewProps
> = ({ options, featureAIBridgeEntitled, featureAIBridgeEnabled }) => {
	return (
		<div className="flex flex-col gap-12">
			<SettingsHeader>
				<SettingsHeaderTitle>AI Governance</SettingsHeaderTitle>
			</SettingsHeader>

			<div>
				<SettingsHeader
					actions={
						<SettingsHeaderDocsLink href={docs("/ai-coder/ai-bridge")} />
					}
				>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						AI Bridge
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Monitor and manage AI requests across your deployment.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{featureAIBridgeEntitled ? (
					<>
						{!featureAIBridgeEnabled && (
							<Alert className="mb-12" severity="warning" prominent>
								<AlertTitle>
									AI Bridge is included in your license, but not set up yet.
								</AlertTitle>
								<AlertDescription>
									You have access to AI Governance, but it still needs to be
									setup. Check out the{" "}
									<Link href={docs("/ai-coder/ai-bridge")} target="_blank">
										AI Bridge
									</Link>{" "}
									documentation to get started.
								</AlertDescription>
							</Alert>
						)}
						<OptionsTable
							options={options
								.filter((o) => deploymentGroupHasParent(o.group, "AI Bridge"))
								.filter((o) => !o.annotations?.secret === true)}
						/>
					</>
				) : (
					<PaywallAIGovernance />
				)}
			</div>
		</div>
	);
};
