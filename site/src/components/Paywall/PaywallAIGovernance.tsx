import { PaywallSmall } from "#/components/Paywall/PaywallSmall";

const PAYWALL_AIGATEWAY_DESCRIPTION =
	"AI Gateway provides auditable visibility into user prompts and LLM tool calls from developer tools within Coder Workspaces. AI Gateway requires a Premium license with AI Governance add-on.".trim();

const PaywallAIGovernance = () => {
	return (
		<PaywallSmall
			message="AI Gateway"
			canViewPremium
			description={PAYWALL_AIGATEWAY_DESCRIPTION}
			features={[
				"Auditable history of user prompts",
				"Logged LLM tool invocations",
				"Token usage and consumption visibility",
				"Centrally-managed MCP servers",
			]}
		/>
	);
};

export { PaywallAIGovernance };
