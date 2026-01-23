import type { SerpentOption } from "api/typesGenerated";
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
				<Paywall
					message="AI Bridge"
					description="AI Bridge provides auditable visibility into user prompts and LLM tool calls from developer tools within Coder Workspaces. AI Bridge requires a Premium license with AI Governance add-on."
					documentationLink="https://coder.com/docs/ai-coder/ai-governance"
					documentationLinkText="Learn about AI Governance"
					badgeText="AI Governance"
					ctaText="Contact Sales"
					ctaLink="https://coder.com/contact"
					features={[
						{ text: "Auditable history of user prompts" },
						{ text: "Logged LLM tool invocations" },
						{ text: "Token usage and consumption visibility" },
						{ text: "User-level AI request attribution" },
						{
							text: "Visit",
							link: {
								href: "https://coder.com/docs/ai-coder/ai-bridge",
								text: "AI Bridge Docs",
							},
						},
					]}
				/>
			)}
		</Stack>
	);
};
