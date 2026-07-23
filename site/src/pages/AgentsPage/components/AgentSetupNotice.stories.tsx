import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "provider" }),
		).toHaveAttribute("href", "/ai/settings/providers");
		await expect(
			canvas.getByRole("link", { name: "model" }),
		).toBeInTheDocument();
	},
};

// Admin with a provider but no model: prompt to add a model only.
export const AdminNoModel: Story = {
	args: {
		isAdmin: true,
		providerCount: 1,
		modelCount: 0,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "model" })).toHaveAttribute(
			"href",
			"/ai/settings/models",
		);
		await expect(
			canvas.queryByRole("link", { name: "provider" }),
		).not.toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/not supported by Coder Agents/),
		).toBeInTheDocument();
		await expect(canvas.getByText(/GitHub Copilot/)).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: "provider" }),
		).toHaveAttribute("href", "/ai/settings/providers");
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Members get the "learn more" docs link but no admin settings link.
		await expect(
			canvas.getByRole("link", { name: /not supported by Coder Agents/ }),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: "provider" }),
		).not.toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/AI Gateway is disabled/),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(/Enable it in your deployment config/),
		).toBeInTheDocument();
	},
};

// Both a provider and a model are configured: the notice renders nothing.
export const Configured: Story = {
	args: {
		isAdmin: true,
		providerCount: 1,
		modelCount: 1,
	},
	play: async ({ canvasElement }) => {
		await expect(canvasElement).toBeEmptyDOMElement();
	},
};
