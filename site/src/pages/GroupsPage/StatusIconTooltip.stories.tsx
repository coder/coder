import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { StatusIconTooltip } from "./StatusIconTooltip";

const meta: Meta<typeof StatusIconTooltip> = {
	title: "pages/OrganizationGroupsPage/StatusIconTooltip",
	component: StatusIconTooltip,
	args: { message: "Spend compared to the budget for the active period." },
};

export default meta;
type Story = StoryObj<typeof StatusIconTooltip>;

export const Info: Story = {
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

export const Warning: Story = {
	args: { kind: "warning" },
};
