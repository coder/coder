import { PaywallSmall } from "#/components/Paywall/PaywallSmall";

type PaywallAIGovernanceVariant = "governance" | "sessions";

const PAYWALL_AIGOVERNANCE_COPY: Record<
	PaywallAIGovernanceVariant,
	{ description: string; features: string[] }
> = {
	governance: {
		description:
			"Get a full audit trail of every prompt, tool call, and model response, so AI adoption stays visible, secure, and accountable.",
		features: [
			"Centralized auth, no scattered API keys",
			"Approve MCP servers & tools org-wide",
			"Per-user token spend & usage tracking",
		],
	},
	sessions: {
		description:
			"Trace every AI coding session step by step to see which prompt triggered which tool call, and who was behind it.",
		features: [
			"Full session & thread-level detail",
			"Attribute every action to a user",
			"Filter by user, project, or date",
		],
	},
};

type PaywallAIGovernanceProps = {
	variant?: PaywallAIGovernanceVariant;
};

const PaywallAIGovernance = ({
	variant = "governance",
}: PaywallAIGovernanceProps) => {
	const { description, features } = PAYWALL_AIGOVERNANCE_COPY[variant];

	return (
		<PaywallSmall
			message="AI Gateway"
			canViewPremium
			description={description}
			features={features}
		/>
	);
};

export { PaywallAIGovernance };
