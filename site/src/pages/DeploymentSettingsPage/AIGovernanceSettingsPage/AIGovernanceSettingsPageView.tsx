import type { SerpentOption } from "api/typesGenerated";
import { aiBridgePaywallConfig } from "components/Paywall/AIBridgePaywallConfig";
import { Paywall } from "components/Paywall/Paywall";
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
	featureAIBridgeEnabled: boolean;
};

export const AIGovernanceSettingsPageView: FC<
	AIGovernanceSettingsPageViewProps
> = ({ options, featureAIBridgeEnabled }) => {
	return (
		<Stack direction="column" spacing={6}>
			<SettingsHeader>
				<SettingsHeaderTitle>AI Governance</SettingsHeaderTitle>
			</SettingsHeader>

			{featureAIBridgeEnabled ? (
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

					<OptionsTable
						options={options
							.filter((o) => deploymentGroupHasParent(o.group, "AI Bridge"))
							.filter((o) => !o.annotations?.secret === true)}
					/>
				</div>
			) : (
				<Paywall {...aiBridgePaywallConfig} />
			)}
		</Stack>
	);
};
