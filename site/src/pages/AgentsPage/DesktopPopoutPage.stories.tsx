import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { DesktopPopoutPageView } from "./DesktopPopoutPage";

const meta = {
	title: "pages/AgentsPage/DesktopPopoutPage",
	component: DesktopPopoutPageView,
	parameters: {
		layout: "fullscreen",
	},
} satisfies Meta<typeof DesktopPopoutPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connecting: Story = {
	args: {
		status: "connecting",
		reconnect: fn(),
		attach: fn(),
		scaleMode: "fit",
		onScaleModeChange: fn(),
		isControlling: false,
		onTakeControl: fn(),
		onReleaseControl: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Connecting to desktop..."),
		).toBeInTheDocument();
	},
};

export const Connected: Story = {
	args: {
		...Connecting.args,
		status: "connected",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Take control")).toBeInTheDocument();
		await expect(canvas.getByText("Zoom to 100%")).toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	args: {
		...Connecting.args,
		status: "error",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Reconnect")).toBeInTheDocument();
	},
};

export const Disconnected: Story = {
	args: {
		...Connecting.args,
		status: "disconnected",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Desktop disconnected. Reconnecting..."),
		).toBeInTheDocument();
	},
};
