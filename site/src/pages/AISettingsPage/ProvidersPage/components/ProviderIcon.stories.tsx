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
