import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { PaywallSmall } from "./PaywallSmall";

const meta: Meta<typeof PaywallSmall> = {
	title: "components/Paywall/Small",
	component: PaywallSmall,
	args: {
		message: "Workspace Proxies",
		description:
			"Workspace proxies provide low-latency connections for geo-distributed teams.",
		canViewPremium: false,
	},
};

export default meta;
type Story = StoryObj<typeof PaywallSmall>;

export const CanViewLicenses: Story = {
	args: { canViewPremium: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toBeVisible();
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
		await expect(cta).not.toHaveAttribute("target", "_blank");
		await expect(
			canvas.queryByText(/contact your deployment administrator/i),
		).not.toBeInTheDocument();
	},
};

export const CannotViewLicenses: Story = {
	args: { canViewPremium: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("link", { name: "Learn more about premium" }),
		).toBeVisible();
		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const Compact: Story = {
	args: { canViewPremium: true, compact: true },
};
