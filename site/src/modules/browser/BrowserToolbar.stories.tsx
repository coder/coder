import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BrowserToolbar } from "./BrowserToolbar";

const meta: Meta<typeof BrowserToolbar> = {
	title: "modules/browser/BrowserToolbar",
	component: BrowserToolbar,
	args: {
		status: "connected",
		navState: {
			url: "http://localhost:3000/",
			title: "App",
			loading: false,
			canGoBack: true,
			canGoForward: false,
		},
		onNavigate: fn(),
		onBack: fn(),
		onForward: fn(),
		onReload: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof BrowserToolbar>;

export const Connected: Story = {};

export const Loading: Story = {
	args: {
		navState: {
			url: "http://localhost:3000/slow",
			title: "",
			loading: true,
			canGoBack: true,
			canGoForward: true,
		},
	},
};

export const Disconnected: Story = {
	args: { status: "disconnected" },
};
