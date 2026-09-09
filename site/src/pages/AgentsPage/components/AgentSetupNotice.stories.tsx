import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentSetupNotice } from "./AgentSetupNotice";

const meta: Meta<typeof AgentSetupNotice> = {
	title: "pages/AgentsPage/AgentSetupNotice",
	component: AgentSetupNotice,
};

export default meta;
type Story = StoryObj<typeof AgentSetupNotice>;

// Admin with nothing configured: prompt to set up a provider and a model.
export const AdminNoProvider: Story = {
	args: {
		isAdmin: true,
		providerCount: 0,
		modelCount: 0,
	},
};

// Admin with a provider but no model: prompt to add a model only.
export const AdminNoModel: Story = {
	args: {
		isAdmin: true,
		providerCount: 1,
		modelCount: 0,
	},
};

// Only a harness-unsupported provider (Copilot) is configured. The notice
// must explain why instead of implying nothing is configured.
export const AdminOnlyUnsupportedProvider: Story = {
	args: {
		isAdmin: true,
		providerCount: 0,
		modelCount: 0,
		unsupportedProviderNames: ["GitHub Copilot"],
	},
};

// Non-admin sees an account-agnostic explanation without admin links.
export const MemberOnlyUnsupportedProvider: Story = {
	args: {
		isAdmin: false,
		providerCount: 0,
		modelCount: 0,
		unsupportedProviderNames: ["GitHub Copilot"],
	},
};

// AI Gateway disabled takes precedence even when providers and models are
// configured and the viewer is an admin.
export const AIGatewayDisabled: Story = {
	args: {
		isAdmin: true,
		providerCount: 1,
		modelCount: 1,
		aiGatewayDisabled: true,
	},
};

// Both a provider and a model are configured: the notice renders nothing.
export const Configured: Story = {
	args: {
		isAdmin: true,
		providerCount: 1,
		modelCount: 1,
	},
};
