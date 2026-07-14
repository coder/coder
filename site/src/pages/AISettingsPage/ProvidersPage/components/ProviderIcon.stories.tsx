import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProviderIcon } from "./ProviderIcon";

const meta: Meta<typeof ProviderIcon> = {
	title: "pages/AISettingsPage/ProviderIcon",
	component: ProviderIcon,
};

export default meta;
type Story = StoryObj<typeof ProviderIcon>;

export const OpenAI: Story = {
	args: {
		provider: "openai",
	},
};

export const Anthropic: Story = {
	args: {
		provider: "anthropic",
	},
};

export const Bedrock: Story = {
	args: {
		provider: "bedrock",
	},
};

export const Copilot: Story = {
	args: {
		provider: "copilot",
	},
};

export const Azure: Story = {
	args: {
		provider: "azure",
	},
};

export const Google: Story = {
	args: {
		provider: "google",
	},
};

export const Vercel: Story = {
	args: {
		provider: "vercel",
	},
};

// Provider types without a bundled icon (openai-compat, openrouter, or
// anything we don't recognize) render the generic Building2 glyph.
export const Fallback: Story = {
	args: {
		provider: "openai-compat",
	},
};
