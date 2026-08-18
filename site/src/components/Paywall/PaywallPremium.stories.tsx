import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { PREMIUM_PRICING_LINK } from "#/components/Paywall/Paywall";
import { PaywallPremium } from "./PaywallPremium";

const meta: Meta<typeof PaywallPremium> = {
	title: "components/Paywall/Premium",
	component: PaywallPremium,
	args: {
		description: "You need a Premium license to use this feature.",
		canViewPremium: false,
	},
};

export default meta;
type Story = StoryObj<typeof PaywallPremium>;

export const CanViewLicenses: Story = {
	args: { canViewPremium: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("heading", { name: /Coder Premium/ }),
		).toBeVisible();
		await expect(canvas.getByText("Start a 30-day trial today.")).toBeVisible();

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toBeVisible();
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
		await expect(cta).not.toHaveAttribute("target", "_blank");

		const pricing = canvas.getByRole("link", {
			name: "Learn more about premium",
		});
		await expect(pricing).toBeVisible();
		await expect(pricing).toHaveAttribute("href", PREMIUM_PRICING_LINK);
		await expect(pricing).toHaveAttribute("target", "_blank");

		await expect(
			canvas.getByRole("heading", { name: /Workspace proxies provide/ }),
		).toBeVisible();
		await expect(canvas.getAllByRole("listitem")).toHaveLength(4);
		await expect(
			canvas.getByText("24x7 global support with SLA"),
		).toBeVisible();

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
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: "Learn more about premium" }),
		).toBeVisible();
	},
};

export const Light: Story = {
	args: { canViewPremium: true },
	parameters: { themes: { themeOverride: "light" } },
};
