import type { SerpentOption } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { deploymentGroupHasParent } from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

type AIGovernanceSettingsPageViewProps = {
	options: SerpentOption[];
	featureAIBridgeEntitled: boolean;
	featureAIBridgeEnabled: boolean;
};

export const AIGovernanceSettingsPageView: React.FC<
	AIGovernanceSettingsPageViewProps
> = ({ options, featureAIBridgeEntitled, featureAIBridgeEnabled }) => {
	return (
		<div className="flex flex-col gap-12">
			<SettingsHeader>
				<SettingsHeaderTitle>AI Governance</SettingsHeaderTitle>
			</SettingsHeader>

			<div>
				<SettingsHeader>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						AI Gateway
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Monitor and manage AI requests across your deployment.{" "}
						<SettingsHeaderDocsLink href={docs("/ai-coder/ai-governance")} />
					</SettingsHeaderDescription>
				</SettingsHeader>

				{featureAIBridgeEntitled ? (
					<>
						{!featureAIBridgeEnabled && (
							<Alert
								className="mb-12"
								severity="info"
								actions={
									<Link
										href={docs("/ai-coder/ai-gateway")}
										target="_blank"
										rel="noreferrer"
									>
										View setup guide
										<span className="sr-only"> (opens in new tab)</span>
									</Link>
								}
							>
								<AlertTitle>AI Gateway isn't set up yet</AlertTitle>
								<AlertDescription>
									Your license includes it, but it isn't turned on for this
									deployment. Follow the setup guide to start monitoring AI
									requests.
								</AlertDescription>
							</Alert>
						)}
						<OptionsTable
							options={options
								.filter((o) => deploymentGroupHasParent(o.group, "AI Gateway"))
								.filter((o) => !o.annotations?.secret === true)}
						/>
					</>
				) : (
					<PremiumPaywallAIGovernance source="ai_governance" />
				)}
			</div>
		</div>
	);
};
