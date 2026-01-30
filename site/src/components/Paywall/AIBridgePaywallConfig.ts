import type { ComponentProps } from "react";
import type { Paywall } from "./Paywall";

// Centralized AI Bridge paywall configuration to avoid duplication.
export const aiBridgePaywallConfig: Omit<ComponentProps<typeof Paywall>, ""> = {
	message: "AI Bridge",
	description:
		"AI Bridge provides auditable visibility into user prompts and LLM tool calls from developer tools within Coder Workspaces. AI Bridge requires a Premium license with AI Governance add-on.",
	documentationLink: "https://coder.com/docs/ai-coder/ai-governance",
	documentationLinkText: "Learn about AI Governance",
	badgeText: "AI Governance",
	ctaText: "Contact Sales",
	ctaLink: "https://coder.com/contact",
	features: [
		{ text: "Auditable history of user prompts" },
		{ text: "Logged LLM tool invocations" },
		{ text: "Token usage and consumption visibility" },
		{ text: "Centrally-managed MCP servers" },
		{
			text: "Visit",
			link: {
				href: "https://coder.com/docs/ai-coder/ai-bridge",
				text: "AI Bridge Docs",
			},
		},
	],
};
