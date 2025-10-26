import {
	DeploymentHealthUnhealthy,
	MockDeploymentStats,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
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
		const healthLink = canvas
			.getAllByRole("link")
			.find((el) => el.getAttribute("href") === "/health");
		if (!healthLink) {
			throw new Error("Health link not found");
		}
		await userEvent.hover(healthLink);
		await waitFor(() => expect(canvas.getByRole("dialog")).toBeInTheDocument());
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
		const healthLink = canvas
			.getAllByRole("link")
			.find((el) => el.getAttribute("href") === "/health");
		if (!healthLink) {
			throw new Error("Health link not found");
		}
		await userEvent.hover(healthLink);
		await waitFor(() => expect(canvas.getByRole("dialog")).toBeInTheDocument());
	},
};
