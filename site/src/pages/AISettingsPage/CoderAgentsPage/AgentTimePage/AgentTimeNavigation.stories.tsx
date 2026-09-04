import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";
import { MockNoPermissions } from "#/testHelpers/entities";

const routePath = "/ai/settings/coder-agents/agent-time";

const meta = {
	title: "pages/AISettingsPage/CoderAgentsPage/AgentTimeNavigation",
	component: AISettingsSidebarView,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: routePath },
			routing: [
				{ path: "/ai/settings/coder-agents", element: <h1>Coder Agents</h1> },
				{ path: routePath, useStoryElement: true },
			],
		}),
	},
	args: {
		permissions: {
			...MockNoPermissions,
			viewDeploymentConfig: true,
		},
	},
} satisfies Meta<typeof AISettingsSidebarView>;

export default meta;
type Story = StoryObj<typeof AISettingsSidebarView>;

export const DeploymentConfigReader: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const link = canvas.getByRole("link", { name: "Agent time" });
		await expect(link).toBeVisible();
		await expect(link).toHaveAttribute("aria-current", "page");
		await expect(
			canvas.queryByRole("link", { name: "Coder Agents" }),
		).not.toBeInTheDocument();
	},
};

export const CoderAgentsParentKeepsSeparateActiveState: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			viewDeploymentConfig: true,
			editDeploymentConfig: true,
		},
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/coder-agents" },
			routing: [
				{ path: "/ai/settings/coder-agents", useStoryElement: true },
				{ path: routePath, element: <h1>Agent time route</h1> },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const coderAgents = canvas.getByRole("link", { name: "Coder Agents" });
		await expect(coderAgents).toHaveAttribute("aria-current", "page");
		await userEvent.click(canvas.getByRole("link", { name: "Agent time" }));
	},
};
