import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MockTemplateVersion, mockApiError } from "#/testHelpers/entities";
import { TemplateParametersPageView } from "./TemplateParametersPageView";

const meta: Meta<typeof TemplateParametersPageView> = {
	title: "pages/TemplateSettingsPage/TemplateParametersPageView",
	component: TemplateParametersPageView,
	args: {
		activeVersion: MockTemplateVersion,
		useClassicParameterFlow: false,
		canUpdate: true,
		isSaving: false,
		isRefreshing: false,
		onChangeClassicParameterFlow: fn(),
		onRefresh: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof TemplateParametersPageView>;

export const Example: Story = {};

export const Saving: Story = {
	args: {
		isSaving: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("checkbox", {
				name: /use parameter compatibility mode for workspace builds/i,
			}),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("status", {
				name: /saving parameter compatibility mode/i,
			}),
		).toBeVisible();
	},
};

export const Refreshing: Story = {
	args: {
		isRefreshing: true,
	},
};

export const NoPermission: Story = {
	args: {
		canUpdate: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: /refresh template data/i }),
		).toBeDisabled();
		await expect(
			canvas.getByRole("checkbox", {
				name: /use parameter compatibility mode for workspace builds/i,
			}),
		).toBeDisabled();
	},
};

export const Failed: Story = {
	args: {
		error: mockApiError({
			message: "Failed to import template version.",
		}),
	},
};

export const OptsIntoClassicParameters: Story = {
	parameters: { pixel: { exclude: true } },
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const checkbox = canvas.getByRole("checkbox", {
			name: /use parameter compatibility mode for workspace builds/i,
		});
		await expect(checkbox).not.toBeChecked();

		await user.click(checkbox);
		await expect(args.onChangeClassicParameterFlow).toHaveBeenCalledWith(true);
	},
};

export const ConfirmsBeforeRefreshing: Story = {
	parameters: { pixel: { exclude: true } },
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
