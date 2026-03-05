import type { SerpentOption } from "api/typesGenerated";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { PaywallAIGovernance } from "components/Paywall/PaywallAIGovernance";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { docs } from "utils/docs";
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
		<Stack direction="column" spacing={6}>
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
								<AlertDetail>
									You have access to AI Governance, but it still needs to be
									setup. Check out the{" "}
									<Link href={docs("/ai-coder/ai-bridge")} target="_blank">
										AI Bridge
									</Link>{" "}
									documentation to get started.
								</AlertDetail>
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
		</Stack>
	);
};
