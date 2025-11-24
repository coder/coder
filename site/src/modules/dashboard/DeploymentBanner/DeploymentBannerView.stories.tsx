import {
	DeploymentHealthUnhealthy,
	MockDeploymentStats,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { DeploymentBannerView } from "./DeploymentBannerView";

const meta: Meta<typeof DeploymentBannerView> = {
	title: "modules/dashboard/DeploymentBannerView",
	component: DeploymentBannerView,
	args: {
		stats: MockDeploymentStats,
	},
};

export default meta;
type Story = StoryObj<typeof DeploymentBannerView>;

export const Example: Story = {};

export const WithHealthIssues: Story = {
	args: {
		health: DeploymentHealthUnhealthy,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByTestId("deployment-health-trigger");
		await userEvent.hover(trigger);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument(),
		);
	},
};

export const WithDismissedHealthIssues: Story = {
	args: {
		health: {
			...DeploymentHealthUnhealthy,
			workspace_proxy: {
				...DeploymentHealthUnhealthy.workspace_proxy,
				dismissed: true,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByTestId("deployment-health-trigger");
		await userEvent.hover(trigger);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toBeInTheDocument(),
		);
	},
};
