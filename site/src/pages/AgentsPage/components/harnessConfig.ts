import type { Harness } from "./HarnessPicker";

interface HarnessConfigOption {
	id: string;
	name: string;
	description: string;
	currentValue: string;
	options: readonly { value: string; name: string; description?: string }[];
}

/** Mock session settings for exploring the harness UI without backend integration. */
export const harnessConfig: Record<Harness, readonly HarnessConfigOption[]> = {
	"Coder Agents": [],
	Codex: [
		{
			id: "mode",
			name: "Mode",
			description: "Approval and sandboxing preset for the session",
			currentValue: "agent",
			options: [
				{
					value: "read-only",
					name: "Ask for approval",
					description: "Always ask to edit external files and use the internet",
				},
				{
					value: "agent",
					name: "Approve for me",
					description: "Only ask for actions detected as potentially unsafe",
				},
				{
					value: "agent-full-access",
					name: "Full access",
					description:
						"Unrestricted access to the internet and any file on your computer",
				},
			],
		},
		{
			id: "collaboration_mode",
			name: "Collaboration mode",
			description: "How Codex collaborates for subsequent turns",
			currentValue: "default",
			options: [
				{ value: "default", name: "Default" },
				{
					value: "plan",
					name: "Plan",
					description: "Plan before making changes",
				},
			],
		},
		{
			id: "model",
			name: "Model",
			description: "Model Codex uses for the session",
			currentValue: "gpt-6-astra",
			options: [
				{
					value: "gpt-6-astra",
					name: "6 Astra",
					description: "Our most capable model for complex, demanding work.",
				},
				{
					value: "gpt-5.6-sol",
					name: "5.6 Sol",
					description: "Reliable agentic workhorse for everyday tasks.",
				},
				{
					value: "gpt-5.6-terra",
					name: "5.6 Terra",
					description: "Balanced agentic coding model for everyday work.",
				},
				{
					value: "gpt-5.6-luna",
					name: "5.6 Luna",
					description: "Fast and affordable agentic coding model.",
				},
				{
					value: "gpt-5.5",
					name: "5.5",
					description:
						"Proven previous-generation model for coding and general work.",
				},
			],
		},
		{
			id: "reasoning_effort",
			name: "Reasoning effort",
			description: "How much reasoning effort the model should use",
			currentValue: "low",
			options: [
				{
					value: "low",
					name: "Low",
					description: "Fast responses with lighter reasoning",
				},
				{
					value: "medium",
					name: "Medium",
					description: "Balances speed and reasoning depth for everyday tasks",
				},
				{
					value: "high",
					name: "High",
					description: "Greater reasoning depth for complex problems",
				},
				{
					value: "xhigh",
					name: "Xhigh",
					description: "Extra high reasoning depth for complex problems",
				},
				{
					value: "max",
					name: "Max",
					description: "Maximum reasoning depth for the hardest problems",
				},
				{
					value: "ultra",
					name: "Ultra",
					description: "Maximum reasoning with automatic task delegation",
				},
			],
		},
		{
			id: "fast-mode",
			name: "Fast mode",
			description: "1.5x speed, increased usage",
			currentValue: "off",
			options: [
				{
					value: "off",
					name: "Off",
					description: "Default speed, normal usage",
				},
				{ value: "on", name: "On", description: "1.5x speed, increased usage" },
			],
		},
	],
	"Claude Code": [
		{
			id: "permission_mode",
			name: "Permission mode",
			description: "How Claude Code asks for permission",
			currentValue: "default",
			options: [
				{
					value: "default",
					name: "Default",
					description: "Ask before running tools that need approval",
				},
				{
					value: "accept-edits",
					name: "Accept edits",
					description: "Allow file edits and ask before running commands",
				},
				{
					value: "plan",
					name: "Plan",
					description: "Explore and plan without changing files",
				},
			],
		},
		{
			id: "model",
			name: "Model",
			description: "Model Claude Code uses for the session",
			currentValue: "sonnet",
			options: [
				{
					value: "sonnet",
					name: "Sonnet",
					description: "Balanced speed and capability",
				},
				{
					value: "opus",
					name: "Opus",
					description: "Deeper reasoning for demanding work",
				},
				{
					value: "haiku",
					name: "Haiku",
					description: "Fast responses for smaller tasks",
				},
			],
		},
		{
			id: "thinking",
			name: "Extended thinking",
			description: "Allow more time to reason before responding",
			currentValue: "on",
			options: [
				{
					value: "off",
					name: "Off",
					description: "Respond without extended thinking",
				},
				{
					value: "on",
					name: "On",
					description: "Think through complex requests before acting",
				},
			],
		},
	],
	Pi: [
		{
			id: "model",
			name: "Model",
			description: "Model Pi uses for the session",
			currentValue: "sonnet",
			options: [
				{
					value: "sonnet",
					name: "Sonnet",
					description: "Balanced coding and reasoning",
				},
				{
					value: "gpt-5.5",
					name: "5.5",
					description: "General-purpose coding model",
				},
			],
		},
		{
			id: "thinking_level",
			name: "Thinking level",
			description: "How much time Pi spends reasoning",
			currentValue: "medium",
			options: [
				{ value: "off", name: "Off" },
				{ value: "low", name: "Low" },
				{ value: "medium", name: "Medium" },
				{ value: "high", name: "High" },
			],
		},
		{
			id: "tool_preset",
			name: "Tool preset",
			description: "Tools available during the session",
			currentValue: "coding",
			options: [
				{
					value: "coding",
					name: "Coding",
					description: "Read, edit, and run commands",
				},
				{
					value: "read-only",
					name: "Read only",
					description: "Explore files without making changes",
				},
			],
		},
	],
};
