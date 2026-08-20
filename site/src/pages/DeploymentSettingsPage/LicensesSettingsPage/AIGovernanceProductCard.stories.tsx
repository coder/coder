import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { AIGovernanceProductCard } from "./AIGovernanceProductCard";

const meta: Meta<typeof AIGovernanceProductCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/AIGovernanceProductCard",
	component: AIGovernanceProductCard,
};

export default meta;
type Story = StoryObj<typeof AIGovernanceProductCard>;

const getMetricValue = (canvas: ReturnType<typeof within>, label: string) =>
	canvas.getByText(label).parentElement?.nextElementSibling;

export const GatewayEnabledFirewallInUse: Story = {
	args: {
		aibridgeFeature: {
			enabled: true,
			entitlement: "entitled",
		},
		boundaryFeature: {
			enabled: true,
			entitlement: "entitled",
		},
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "AI Gateway")).toHaveTextContent(
			"Enabled",
		);
		await expect(getMetricValue(canvas, "Agent Firewall")).toHaveTextContent(
			"In use",
		);
		const aiSettings = canvas.getByRole("link", { name: "AI settings" });
		await expect(aiSettings).toHaveAttribute("href", "/ai/settings");

		await step("open the AI Gateway tooltip on hover", async () => {
			await userEvent.hover(
				canvas.getByRole("button", { name: "AI Gateway information" }),
			);
			await waitFor(async () => {
				await expect(screen.getByRole("tooltip")).toHaveTextContent(
					"Routes your deployment's LLM traffic through Coder for auditing and cost controls.",
				);
			});
		});
	},
};

export const GatewayAndFirewallOff: Story = {
	args: {
		aibridgeFeature: {
			enabled: false,
			entitlement: "not_entitled",
		},
		boundaryFeature: {
			enabled: false,
			entitlement: "not_entitled",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "AI Gateway")).toHaveTextContent(
			"Not enabled",
		);
		await expect(getMetricValue(canvas, "Agent Firewall")).toHaveTextContent(
			"Not in use",
		);
	},
};

// Entitlements can be briefly absent while the query loads; the card
// falls back to the disabled statuses instead of erroring.
export const MissingFeatures: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(getMetricValue(canvas, "AI Gateway")).toHaveTextContent(
			"Not enabled",
		);
		await expect(getMetricValue(canvas, "Agent Firewall")).toHaveTextContent(
			"Not in use",
		);
	},
};
