import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockWorkspace } from "#/testHelpers/entities";
import { selectRadixOption } from "#/testHelpers/storybook";
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView";

const meta: Meta<typeof WorkspaceSettingsPageView> = {
	title: "pages/WorkspaceSettingsPage/WorkspaceSettingsPageView",
	component: WorkspaceSettingsPageView,
	args: {
		error: undefined,
		workspace: MockWorkspace,
		onCancel: action("onCancel"),
		onSubmit: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceSettingsPageView>;

export const Example: Story = {};

export const RenamesDisabled: Story = {
	args: {
		workspace: { ...MockWorkspace, allow_renames: false },
	},
};

export const UpdateAutomaticUpdatesPolicy: Story = {
	args: {
		workspace: { ...MockWorkspace, automatic_updates: "never" },
		onSubmit: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await selectRadixOption(canvas, /update policy/i, /always/i);

		await userEvent.click(canvas.getByRole("button", { name: /save/i }));

		await waitFor(() =>
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ automatic_updates: "always" }),
				expect.anything(),
			),
		);
	},
};
