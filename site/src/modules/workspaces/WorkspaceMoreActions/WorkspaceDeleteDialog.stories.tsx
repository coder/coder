import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockFailedWorkspace, MockWorkspace } from "#/testHelpers/entities";
import { daysAgo } from "#/utils/time";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

const meta: Meta<typeof WorkspaceDeleteDialog> = {
	title: "modules/workspaces/WorkspaceDeleteDialog",
	component: WorkspaceDeleteDialog,
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				created_at: daysAgo(2),
			},
		},
		canDeleteFailedWorkspace: false,
		isOpen: true,
		onCancel: fn(),
		onConfirm: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeleteDialog>;

export const Example: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const confirm = body.getByTestId("delete-dialog-name-confirmation");
		const deleteButton = body.getByRole("button", { name: "Delete" });

		await expect(deleteButton).toBeDisabled();
		await userEvent.type(confirm, MockWorkspace.name);
		await expect(deleteButton).toBeEnabled();
		await userEvent.click(deleteButton);
		await expect(args.onConfirm).toHaveBeenCalledWith(false);
	},
};

// Should look the same as `Example`
export const Unhealthy: Story = {
	args: {
		workspace: MockFailedWorkspace,
	},
};

// Should look the same as `Example`
export const AdminView: Story = {
	args: {
		canDeleteFailedWorkspace: true,
	},
};

// Should show the `--orphan` option
export const UnhealthyAdminView: Story = {
	args: {
		workspace: MockFailedWorkspace,
		canDeleteFailedWorkspace: true,
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const orphan = body.getByTestId("orphan-checkbox");
		const confirm = body.getByTestId("delete-dialog-name-confirmation");

		await userEvent.click(orphan);
		await userEvent.type(confirm, MockFailedWorkspace.name);
		await userEvent.click(body.getByRole("button", { name: "Delete" }));
		await expect(args.onConfirm).toHaveBeenCalledWith(true);
	},
};

export const FilledWrong: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const confirm = body.getByTestId("delete-dialog-name-confirmation");

		await userEvent.type(confirm, "wrong-name");
		await userEvent.tab();
		// The validation error renders asynchronously after blur, so wait for
		// visibility instead of asserting it once.
		await waitFor(
			() =>
				expect(
					body.getByText(
						"wrong-name does not match the name of this workspace",
					),
				).toBeVisible(),
			{ timeout: 5_000 },
		);
		await expect(body.getByRole("button", { name: "Delete" })).toBeDisabled();
	},
};
