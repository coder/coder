import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { InlineDesktopPreview } from "./InlineDesktopPreview";

const meta: Meta<typeof InlineDesktopPreview> = {
	title: "components/ai-elements/InlineDesktopPreview",
	component: InlineDesktopPreview,
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
};

// ---------------------------------------------------------------------------
// Connecting — WebSocket handshake in progress.
// ---------------------------------------------------------------------------

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
};
