import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	Paywall,
	PaywallContent,
	PaywallCTA,
	PaywallDescription,
	PaywallFeature,
	PaywallFeatures,
	PaywallHeading,
	PaywallStack,
	PaywallSupergraphic,
	PaywallTitle,
} from "./Paywall";

const CTA_HREF = "https://coder.com/pricing#compare-plans";

const meta: Meta<typeof Paywall> = {
	title: "components/Paywall",
	component: Paywall,
};

export default meta;
type Story = StoryObj<typeof Paywall>;

const paywallChildren = (
	<>
		<PaywallContent>
			<PaywallHeading>
				<PaywallTitle>Black Lotus</PaywallTitle>
			</PaywallHeading>
			<PaywallDescription>
				Adds 3 mana of any single color of your choice to your mana pool, then
				is discarded. Tapping this artifact can be played as an interrupt.
			</PaywallDescription>
		</PaywallContent>
		<PaywallStack>
			<PaywallFeatures>
				<PaywallFeature>High availability & workspace proxies</PaywallFeature>
				<PaywallFeature>Multi-org & role-based access control</PaywallFeature>
				<PaywallFeature>24x7 global support with SLA</PaywallFeature>
				<PaywallFeature>
					Unlimited Git & external auth integrations
				</PaywallFeature>
			</PaywallFeatures>
			<PaywallCTA href={CTA_HREF}>Start trial for free</PaywallCTA>
		</PaywallStack>
	</>
);

const withSupergraphicChildren = (
	<>
		<PaywallSupergraphic />
		{paywallChildren}
	</>
);

const expectPaywallContent = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);

	await expect(
		canvas.getByRole("heading", { name: "Black Lotus" }),
	).toBeVisible();
	await expect(
		canvas.getByText(/Adds 3 mana of any single color/),
	).toBeVisible();

	await expect(canvas.getAllByRole("listitem")).toHaveLength(4);
	await expect(canvas.getByText("24x7 global support with SLA")).toBeVisible();

	const cta = canvas.getByRole("link", { name: "Start trial for free" });
	await expect(cta).toBeVisible();
	await expect(cta).toHaveAttribute("href", CTA_HREF);
	await expect(cta).toHaveAttribute("target", "_blank");
};

export const Default: Story = {
	args: {
		children: paywallChildren,
	},
	play: async ({ canvasElement }) => {
		await expectPaywallContent(canvasElement);
	},
};

export const WithSupergraphic: Story = {
	args: {
		children: withSupergraphicChildren,
	},
	play: async ({ canvasElement }) => {
		await expectPaywallContent(canvasElement);
	},
};

export const WithSupergraphicLight: Story = {
	args: {
		children: withSupergraphicChildren,
	},
	parameters: { themes: { themeOverride: "light" } },
	play: async ({ canvasElement }) => {
		await expectPaywallContent(canvasElement);
	},
};
