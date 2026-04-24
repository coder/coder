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
import { Stack } from "#/components/Stack/Stack";
import { deploymentGroupHasParent } from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

// Fake options for the "Cost controls" section. These are not real
// deployment options; they exist only for the presentation mockup.
const COST_CONTROLS_OPTIONS: readonly SerpentOption[] = [
	{
		name: "Group AI budget options",
		description:
			"A user's budget is determined by their group membership. " +
			"If set to high, the user inherits the highest budget across all " +
			"group memberships. If set to low, the user inherits the lowest budget.",
		flag: "aigov-groupbudget-high",
		env: "CODER_AIGOV_GROUPBUDGET_HIGH",
		yaml: "groupbudget_high",
		value: "High",
		value_source: "default",
		group: { name: "Cost Controls", parent: { name: "AI Governance" } },
	},
];

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
			<SettingsHeader
				actions={
					<SettingsHeaderDocsLink href={docs("/ai-coder/ai-bridge")} />
				}
			>
				<SettingsHeaderTitle>AI Governance</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Monitor and manage AI requests across your deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{/* Cost controls section (mockup). */}
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						Cost controls
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Set budgets and spending limits per group to control AI
						costs across your organization.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<OptionsTable options={COST_CONTROLS_OPTIONS} />
			</div>

			{/* AI Gateway section (renamed from AI Bridge). */}
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle hierarchy="secondary" level="h2">
						AI Gateway
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Configure the AI gateway to route, authenticate, and
						audit LLM traffic for your deployment.
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
		</Stack>
	);
};
