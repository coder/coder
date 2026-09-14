import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { PremiumPaywallAIGovernance } from "./PremiumPaywallAIGovernance";

const meta: Meta<typeof PremiumPaywallAIGovernance> = {
	title: "modules/paywall/PremiumPaywallAIGovernance",
	component: PremiumPaywallAIGovernance,
	args: {
		source: "ai_governance",
	},
	beforeEach: () => {
		sessionStorage.clear();
		spyOn(API, "reportPremiumFunnelEvent").mockResolvedValue();
	},
};

export default meta;
type Story = StoryObj<typeof PremiumPaywallAIGovernance>;

export const ReportsCTAClick: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("link", { name: "Start trial for free" }),
		);

		await waitFor(() =>
			expect(API.reportPremiumFunnelEvent).toHaveBeenCalledWith({
				id: expect.any(String),
				source: "ai_governance",
				variant: "ai_governance",
			}),
		);
	},
};

// The three AI surfaces share one component, so the source must distinguish
// them.
export const AIBridgeSessions: Story = {
	args: { source: "aibridge_sessions", variant: "sessions" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("link", { name: "Start trial for free" }),
		);

		await waitFor(() =>
			expect(API.reportPremiumFunnelEvent).toHaveBeenCalledWith(
				expect.objectContaining({ source: "aibridge_sessions" }),
			),
		);
	},
};
