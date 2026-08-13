import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { PaywallPremium } from "./PaywallPremium";

const meta: Meta<typeof PaywallPremium> = {
	title: "components/Paywall/Premium",
	component: PaywallPremium,
	args: {
		message: "Workspace Proxies",
		description:
			"Workspace proxies provide low-latency connections for geo-distributed teams. You need a Premium license to use this feature.",
		documentationLink:
			"https://coder.com/docs/admin/networking/workspace-proxies",
		canViewPremium: false,
	},
};

export default meta;
type Story = StoryObj<typeof PaywallPremium>;

export const CanViewLicenses: Story = {
	args: { canViewPremium: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Learn about Premium" });
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

		await expect(canvas.getByText("Premium")).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Read the documentation" }),
		).toBeVisible();
		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Learn about Premium" }),
		).not.toBeInTheDocument();
	},
};

export const Compact: Story = {
	args: { canViewPremium: true, compact: true },
};
