import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DesktopToolbar } from "./DesktopToolbar";

const meta = {
	title: "pages/AgentsPage/DesktopToolbar",
	component: DesktopToolbar,
} satisfies Meta<typeof DesktopToolbar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ViewOnly: Story = {
	args: {
		scaleMode: "fit",
		onScaleModeChange: fn(),
		isControlling: false,
		onTakeControl: fn(),
		onReleaseControl: fn(),
		onPopOut: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const takeControl = canvas.getByText("Take control");
		await userEvent.click(takeControl);
		await expect(args.onTakeControl).toHaveBeenCalled();

		const zoom = canvas.getByText("Zoom to 100%");
		await userEvent.click(zoom);
		await expect(args.onScaleModeChange).toHaveBeenCalledWith("native");

		const detach = canvas.getByText("Detach");
		await userEvent.click(detach);
		await expect(args.onPopOut).toHaveBeenCalled();
	},
};

export const Controlling: Story = {
	args: {
		...ViewOnly.args,
		isControlling: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const release = canvas.getByText("Release control");
		await userEvent.click(release);
		await expect(args.onReleaseControl).toHaveBeenCalled();
	},
};

export const NativeZoom: Story = {
	args: {
		...ViewOnly.args,
		scaleMode: "native",
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const zoom = canvas.getByText("Zoom to fit");
		await userEvent.click(zoom);
		await expect(args.onScaleModeChange).toHaveBeenCalledWith("fit");
	},
};

export const PoppedOut: Story = {
	args: {
		...ViewOnly.args,
		isPoppedOut: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Detach button should not render when popped out.
		const detach = canvas.queryByText("Detach");
		await expect(detach).toBeNull();
	},
};
