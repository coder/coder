import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockAIGatewayKeys, mockApiError } from "#/testHelpers/entities";
import { GatewayKeysPageView } from "./GatewayKeysPageView";

const meta: Meta<typeof GatewayKeysPageView> = {
	title: "pages/AISettingsPage/GatewayKeysPageView",
	component: GatewayKeysPageView,
	// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
	parameters: { pixel: { exclude: true } },
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
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const [, emptyCta] = await canvas.findAllByRole("button", {
			name: /create key/i,
		});
		await userEvent.click(emptyCta);
		await expect(args.onCreateKey).toHaveBeenCalledTimes(1);
	},
};

export const LoadError: Story = {
	args: {
		keys: [],
		error: mockApiError({ message: "Failed to load AI Gateway keys" }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText("Failed to load AI Gateway keys"),
		).resolves.toBeVisible();
		await expect(
			canvas.findByText("Failed to fetch AI Gateway keys"),
		).resolves.toBeVisible();
		await expect(
			canvas.queryByText("No AI Gateway keys"),
		).not.toBeInTheDocument();
		await expect(
			canvas.getAllByRole("button", { name: /create key/i }),
		).toHaveLength(1);
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
