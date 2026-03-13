import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
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
		const button = canvas.getByRole("button");
		const initialText = button.querySelector("svg")?.classList.toString();
		await userEvent.click(button);
		const updatedText = button.querySelector("svg")?.classList.toString();
		// The icon class should change after clicking.
		expect(initialText).not.toBe(updatedText);
	},
};
