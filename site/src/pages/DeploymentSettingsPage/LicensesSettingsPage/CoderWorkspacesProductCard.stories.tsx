import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, waitFor, within } from "storybook/test";
import { CoderWorkspacesProductCard } from "./CoderWorkspacesProductCard";

const meta: Meta<typeof CoderWorkspacesProductCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/CoderWorkspacesProductCard",
	component: CoderWorkspacesProductCard,
	args: {
		userLimitActual: 4,
		userLimitLimit: 10,
	},
};

export default meta;
type Story = StoryObj<typeof CoderWorkspacesProductCard>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Coder Workspaces")).toBeInTheDocument();
		const usageLabel = canvas.getByText("Active seat usage");
		const usageValue = usageLabel.parentElement?.nextElementSibling;
		await expect(usageValue).toHaveTextContent("4 / 10");
	},
};

export const TooltipInteraction: Story = {
	play: async ({ canvasElement, userEvent }) => {
		const canvas = within(canvasElement);
		await userEvent.tab();
		await expect(
			canvas.getByRole("button", { name: "Active seat usage information" }),
		).toHaveFocus();
		await waitFor(async () => {
			await expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Only Active user accounts consume license seats.",
			);
		});
	},
};

export const UnlimitedSeats: Story = {
	args: {
		userLimitLimit: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const usageLabel = canvas.getByText("Active seat usage");
		const usageValue = usageLabel.parentElement?.nextElementSibling;
		await expect(usageValue).toHaveTextContent("4 / Unlimited");
	},
};

export const NoUsageData: Story = {
	args: {
		userLimitActual: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const usageLabel = canvas.getByText("Active seat usage");
		const usageValue = usageLabel.parentElement?.nextElementSibling;
		await expect(usageValue).toHaveTextContent("\u2014 / 10");
	},
};

export const LargeCounts: Story = {
	args: {
		userLimitActual: 1923,
		userLimitLimit: 2500,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const usageLabel = canvas.getByText("Active seat usage");
		const usageValue = usageLabel.parentElement?.nextElementSibling;
		await expect(usageValue).toHaveTextContent("1,923 / 2,500");
	},
};
