import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockTemplateVersion, mockApiError } from "#/testHelpers/entities";
import { TemplateDataPageView } from "./TemplateDataPageView";

const meta: Meta<typeof TemplateDataPageView> = {
	title: "pages/TemplateSettingsPage/TemplateDataPageView",
	component: TemplateDataPageView,
	args: {
		activeVersion: MockTemplateVersion,
		canRefresh: true,
		isRefreshing: false,
		onRefresh: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof TemplateDataPageView>;

export const Example: Story = {};

export const Refreshing: Story = {
	args: {
		isRefreshing: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: /refresh template data/i }),
		).toBeDisabled();
	},
};

export const NoPermission: Story = {
	args: {
		canRefresh: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", { name: /refresh template data/i }),
		).not.toBeInTheDocument();
	},
};

export const RefreshFailed: Story = {
	args: {
		error: mockApiError({
			message: "Failed to import template version.",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Failed to import template version.");
	},
};

export const ConfirmsBeforeRefreshing: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(
			canvas.getByRole("button", { name: /refresh template data/i }),
		);

		const dialog = within(await within(document.body).findByRole("dialog"));
		await expect(args.onRefresh).not.toHaveBeenCalled();

		await user.click(dialog.getByRole("button", { name: "Refresh" }));
		await expect(args.onRefresh).toHaveBeenCalledTimes(1);
	},
};

export const CancelsRefresh: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(
			canvas.getByRole("button", { name: /refresh template data/i }),
		);

		const dialog = within(await within(document.body).findByRole("dialog"));
		await user.click(dialog.getByRole("button", { name: /cancel/i }));

		await expect(args.onRefresh).not.toHaveBeenCalled();
	},
};
