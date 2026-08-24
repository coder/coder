import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { PremiumPaywall } from "./PremiumPaywall";
import { readPremiumFunnelAttribution } from "./premiumFunnelAttribution";

const meta: Meta<typeof PremiumPaywall> = {
	title: "modules/paywall/PremiumPaywall",
	component: PremiumPaywall,
	args: {
		source: "appearance",
		message: "Appearance",
		description: "You need a Premium license to use this feature.",
		canViewPremium: true,
	},
	beforeEach: () => {
		sessionStorage.clear();
		spyOn(API, "reportPremiumFunnelEvent").mockResolvedValue();
	},
};

export default meta;
type Story = StoryObj<typeof PremiumPaywall>;

export const ReportsCTAClickWithAttribution: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("link", { name: "Start trial for free" }),
		);

		// The stored token must match the reported click so a later trial signup
		// joins back to this event and not to a stale one.
		const attribution = readPremiumFunnelAttribution();
		await expect(attribution?.source).toBe("appearance");
		await waitFor(() =>
			expect(API.reportPremiumFunnelEvent).toHaveBeenCalledWith({
				id: attribution?.id,
				source: "appearance",
				variant: "premium",
			}),
		);
	},
};

// Users who cannot view licenses get guidance instead of a call to action, so
// they have nothing to click and nothing to report.
export const NoCTAWithoutPermission: Story = {
	args: { canViewPremium: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(API.reportPremiumFunnelEvent).not.toHaveBeenCalled();
	},
};
