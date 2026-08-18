import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { TemplateSettingsPageView } from "./TemplateSettingsPageView";

const meta: Meta<typeof TemplateSettingsPageView> = {
	title: "pages/TemplateSettingsPage",
	component: TemplateSettingsPageView,
	args: {
		template: MockTemplate,
		accessControlEnabled: true,
		advancedSchedulingEnabled: true,
		onCancel: action("onCancel"),
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof TemplateSettingsPageView>;

export const Example: Story = {};

export const AgentsNotAllowed: Story = {
	args: {
		template: {
			...MockTemplate,
			agents_allowed: false,
		},
	},
};

export const SaveTemplateSettingsError: Story = {
	args: {
		submitError: mockApiError({
			message: 'Template "test" already exists.',
			validations: [
				{
					field: "name",
					detail: "This value is already in use and should be unique.",
				},
			],
		}),
		initialTouched: {
			allow_user_cancel_workspace_jobs: true,
		},
	},
};

export const NoEntitlements: Story = {
	args: {
		accessControlEnabled: false,
		advancedSchedulingEnabled: false,
	},
};

export const NoEntitlementsExpiredSettings: Story = {
	args: {
		template: {
			...MockTemplate,
			deprecated: true,
			deprecation_message: "This template tastes bad",
			require_active_version: true,
		},
		accessControlEnabled: false,
		advancedSchedulingEnabled: false,
	},
};

export const AllowWorkspaceRenamesEnabled: Story = {
	args: {
		template: { ...MockTemplate, allow_workspace_renames: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const checkbox = await canvas.findByRole("checkbox", {
			name: /allow users to rename their workspaces/i,
		});
		await expect(checkbox).toBeChecked();
	},
};

export const ToggleAllowWorkspaceRenames: Story = {
	args: {
		onSubmit: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const checkbox = await canvas.findByRole("checkbox", {
			name: /allow users to rename their workspaces/i,
		});
		await expect(checkbox).not.toBeChecked();

		await user.click(checkbox);
		await user.click(canvas.getByRole("button", { name: /save/i }));

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ allow_workspace_renames: true }),
				// Formik passes its helpers as a second argument.
				expect.anything(),
			);
		});
	},
};
