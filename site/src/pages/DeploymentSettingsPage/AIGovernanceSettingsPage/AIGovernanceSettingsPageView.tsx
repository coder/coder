import type { SerpentOption } from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
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
						{!featureAIBridgeEnabled && <AIBridgeSetupAlert />}
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
