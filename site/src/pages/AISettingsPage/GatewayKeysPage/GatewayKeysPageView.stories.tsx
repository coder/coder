import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockAIGatewayKeys, mockApiError } from "#/testHelpers/entities";
import { GatewayKeysPageView } from "./GatewayKeysPageView";

const meta: Meta<typeof GatewayKeysPageView> = {
	title: "pages/AISettingsPage/GatewayKeysPageView",
	component: GatewayKeysPageView,
	args: {
		keys: MockAIGatewayKeys,
		isLoading: false,
		error: null,
		showPaywall: false,
		onCreateKey: fn(),
		onDeleteKey: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof GatewayKeysPageView>;

export const Default: Story = {};

export const Loading: Story = {
	args: {
		isLoading: true,
		keys: [],
	},
};

export const Empty: Story = {
	args: {
		keys: [],
	},
};

export const LoadError: Story = {
	args: {
		keys: [],
		error: mockApiError({ message: "Failed to load AI Gateway keys" }),
	},
};

export const Paywall: Story = {
	args: {
		showPaywall: true,
		keys: [],
	},
};

export const ClickCreate: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const button = await canvas.findByRole("button", { name: /create key/i });
		await userEvent.click(button);
		await expect(args.onCreateKey).toHaveBeenCalledTimes(1);
	},
};

export const ClickDelete: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const button = await canvas.findByRole("button", {
			name: `Delete ${MockAIGatewayKeys[0].name}`,
		});
		await userEvent.click(button);
		await expect(args.onDeleteKey).toHaveBeenCalledWith(MockAIGatewayKeys[0]);
	},
};
