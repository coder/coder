import { PremiumBadge } from "#/components/Badges/Badges";
import { docs } from "#/utils/docs";
import {
	Paywall,
	PaywallContent,
	PaywallCTA,
	PaywallDescription,
	PaywallDocumentationLink,
	PaywallFeature,
	PaywallFeatures,
	PaywallHeading,
	PaywallSeparator,
	PaywallStack,
	PaywallTitle,
} from "./Paywall";

const PaywallAIGovernance = () => {
	return (
		<Paywall>
			<PaywallContent>
				<PaywallHeading>
					<PaywallTitle>AI Gateway</PaywallTitle>
					<PremiumBadge>AI Governance</PremiumBadge>
				</PaywallHeading>
				<PaywallDescription>
					AI Gateway provides auditable visibility into user prompts and LLM
					tool calls from developer tools within Coder Workspaces. AI Gateway
					requires a Premium license with AI Governance add-on.
				</PaywallDescription>
				<PaywallDocumentationLink href={docs("/ai-coder/ai-governance")}>
					Learn about AI Governance
				</PaywallDocumentationLink>
			</PaywallContent>
			<PaywallSeparator />
			<PaywallStack>
				<PaywallFeatures>
					<PaywallFeature>Auditable history of user prompts</PaywallFeature>
					<PaywallFeature>Logged LLM tool invocations</PaywallFeature>
					<PaywallFeature>
						Token usage and consumption visibility
					</PaywallFeature>
					<PaywallFeature>Centrally-managed MCP servers</PaywallFeature>
					<PaywallFeature>
						<span>
							Visit{" "}
							<a
								href={docs("/ai-coder/ai-gateway")}
								target="_blank"
								rel="noreferrer"
								className="text-content-link"
							>
								AI Gateway Docs
							</a>
						</span>
					</PaywallFeature>
				</PaywallFeatures>
				<PaywallCTA href="https://coder.com/contact/sales">
					Contact Sales
				</PaywallCTA>
			</PaywallStack>
		</Paywall>
	);
};

export { PaywallAIGovernance };
