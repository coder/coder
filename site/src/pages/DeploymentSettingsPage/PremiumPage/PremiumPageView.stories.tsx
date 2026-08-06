import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { PremiumPageView } from "./PremiumPageView";

const meta: Meta<typeof PremiumPageView> = {
	title: "pages/DeploymentSettingsPage/PremiumPageView",
	component: PremiumPageView,
};

export default meta;

type Story = StoryObj<typeof PremiumPageView>;

const expectHeaderBadge = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);

	await expect(
		canvas.getByRole("heading", { name: "Premium", level: 1 }),
	).toBeVisible();
	// The title and the badge beside it are the only exact "Premium" matches.
	await expect(canvas.getAllByText("Premium")).toHaveLength(2);
};

export const Enterprise: Story = {
	args: {
		isEnterprise: true,
	},
	play: async ({ canvasElement }) => {
		await expectHeaderBadge(canvasElement);
	},
};

export const OSS: Story = {
	args: {
		isEnterprise: false,
	},
	play: async ({ canvasElement }) => {
		await expectHeaderBadge(canvasElement);
	},
};
