import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { PremiumPaywallSmall } from "./PremiumPaywallSmall";

const meta: Meta<typeof PremiumPaywallSmall> = {
	title: "modules/paywall/PremiumPaywallSmall",
	component: PremiumPaywallSmall,
	args: {
		source: "custom_roles",
		message: "Custom Roles",
		description: "You need a Premium license to use this feature.",
		canViewPremium: true,
	},
	beforeEach: () => {
		sessionStorage.clear();
		spyOn(API, "reportPremiumFunnelEvent").mockResolvedValue();
	},
};

export default meta;
type Story = StoryObj<typeof PremiumPaywallSmall>;

export const ReportsCTAClick: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("link", { name: "Start trial for free" }),
		);

		await waitFor(() =>
			expect(API.reportPremiumFunnelEvent).toHaveBeenCalledWith({
				id: expect.any(String),
				source: "custom_roles",
				variant: "small",
			}),
		);
	},
};
