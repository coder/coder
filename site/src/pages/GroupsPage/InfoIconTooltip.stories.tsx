import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { InfoIconTooltip } from "./InfoIconTooltip";

const meta: Meta<typeof InfoIconTooltip> = {
	title: "pages/OrganizationGroupsPage/InfoIconTooltip",
	component: InfoIconTooltip,
	args: { message: "Spend compared to the budget for the active period." },
};

export default meta;
type Story = StoryObj<typeof InfoIconTooltip>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "More info" }));
		await expect(
			await within(document.body).findByText(
				"Spend compared to the budget for the active period.",
			),
		).toBeInTheDocument();
	},
};

// Muted icon, used where it sits next to greyed content.
export const Muted: Story = {
	args: { className: "text-content-disabled" },
};
