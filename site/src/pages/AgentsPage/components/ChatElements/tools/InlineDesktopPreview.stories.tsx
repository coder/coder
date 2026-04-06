import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { InlineDesktopPreview } from "./InlineDesktopPreview";

const meta: Meta<typeof InlineDesktopPreview> = {
	title: "components/ai-elements/InlineDesktopPreview",
	component: InlineDesktopPreview,
	decorators: [
		(Story) => (
			<div className="max-w-md rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		chatId: "desktop-chat-1",
		onClick: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof InlineDesktopPreview>;

// ---------------------------------------------------------------------------
// Idle — hook has not started connecting yet.
// ---------------------------------------------------------------------------

export const Idle: Story = {
	args: {
		connectionOverride: {
			status: "idle",
			hasConnected: false,
			reconnect: fn(),
			attach: fn(),
			rfb: null,
			remoteClipboardText: null,
		},
	},
	play: async ({ canvasElement }) => {
		// The idle state shows a loading spinner.
		const canvas = within(canvasElement);
		expect(canvas.getByTitle("Loading spinner")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Connecting — WebSocket handshake in progress.
// ---------------------------------------------------------------------------

export const Connecting: Story = {
	args: {
		connectionOverride: {
			status: "connecting",
			hasConnected: false,
			reconnect: fn(),
			attach: fn(),
			rfb: null,
			remoteClipboardText: null,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTitle("Loading spinner")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Connected — VNC canvas attached.
// ---------------------------------------------------------------------------

export const Connected: Story = {
	args: {
		connectionOverride: {
			status: "connected",
			hasConnected: true,
			reconnect: fn(),
			attach: fn(),
			rfb: null,
			remoteClipboardText: null,
		},
	},
	play: async ({ canvasElement }) => {
		// The connected state renders the VNC container with
		// pointer-events-none to act as a read-only preview.
		expect(canvasElement.querySelector(".pointer-events-none")).not.toBeNull();
	},
};

// ---------------------------------------------------------------------------
// Disconnected — connection dropped, auto-reconnecting.
// ---------------------------------------------------------------------------

export const Disconnected: Story = {
	args: {
		connectionOverride: {
			status: "disconnected",
			hasConnected: true,
			reconnect: fn(),
			attach: fn(),
			rfb: null,
			remoteClipboardText: null,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Desktop disconnected/)).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Error — connection failed permanently.
// ---------------------------------------------------------------------------

export const ErrorState: Story = {
	args: {
		connectionOverride: {
			status: "error",
			hasConnected: false,
			reconnect: fn(),
			attach: fn(),
			rfb: null,
			remoteClipboardText: null,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/Could not connect to desktop/),
		).toBeInTheDocument();
	},
};
