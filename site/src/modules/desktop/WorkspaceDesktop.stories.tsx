import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { withDesktopViewport } from "#/testHelpers/storybook";
import { WorkspaceDesktopView } from "./WorkspaceDesktop";

const meta = {
	title: "modules/desktop/WorkspaceDesktop",
	component: WorkspaceDesktopView,
	decorators: [withDesktopViewport],
	args: {
		onReconnect: fn(),
	},
} satisfies Meta<typeof WorkspaceDesktopView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connecting: Story = {
	args: { status: "connecting" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Connecting to desktop")).toBeInTheDocument();
	},
};

export const Connected: Story = {
	args: { status: "connected" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByTestId("desktop-canvas")).toBeInTheDocument();
		await expect(canvas.queryByText("Reconnect")).not.toBeInTheDocument();
	},
};

export const Disconnected: Story = {
	args: { status: "disconnected" },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByText("Reconnect"));
		await expect(args.onReconnect).toHaveBeenCalled();
	},
};
