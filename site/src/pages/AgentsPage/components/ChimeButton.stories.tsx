import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ChimeButton } from "./ChimeButton";

const meta: Meta<typeof ChimeButton> = {
	title: "pages/AgentsPage/ChimeButton",
	component: ChimeButton,
};
export default meta;
type Story = StoryObj<typeof ChimeButton>;

export const Default: Story = {};

export const ToggleState: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};
