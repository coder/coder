import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { Latency } from "./Latency";

const meta: Meta<typeof Latency> = {
	title: "components/Latency",
	component: Latency,
};

export default meta;
type Story = StoryObj<typeof Latency>;

export const Low: Story = {
	args: {
		latency: 10,
	},
};

export const Medium: Story = {
	args: {
		latency: 150,
	},
};

export const High: Story = {
	args: {
		latency: 300,
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

export const NoLatency: Story = {
	args: {
		latency: undefined,
	},

	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const tooltipTrigger = canvas.getByLabelText(/Latency not available/i);
		await userEvent.hover(tooltipTrigger);

		// Need to await getting the tooltip because the tooltip doesn't open
		// immediately on hover
		await screen.findByRole("tooltip", { name: /Latency not available/i });
	},
};
