import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { DynamicClientRegistrationSetting } from "./DynamicClientRegistrationSetting";

const meta: Meta<typeof DynamicClientRegistrationSetting> = {
	title: "pages/DeploymentSettingsPage/DynamicClientRegistrationSetting",
	component: DynamicClientRegistrationSetting,
	args: {
		enabled: false,
		canEdit: true,
		isUpdating: false,
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof DynamicClientRegistrationSetting>;

export const Disabled: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeEnabled();
		await expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
	},
};

export const Enabled: Story = {
	args: {
		enabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByText("Enabled")).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Disable" })).toBeVisible();
	},
};

export const ReadOnly: Story = {
	args: {
		canEdit: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
	},
};

export const EnabledReadOnly: Story = {
	args: {
		enabled: true,
		canEdit: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: "Disable" }),
		).toBeDisabled();
	},
};

export const EnableShowsConfirmationDialog: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		await body.findByText("Enable Dynamic Client Registration?");
		await expect(args.onChange).not.toHaveBeenCalled();

		// The dialog's confirm button shares its accessible name with the trigger
		// button behind it, so scope the query to the dialog.
		const dialog = within(body.getByTestId("dialog"));

		// The dialog covers the section description, so it is the last thing the
		// admin reads before enabling. It has to name both what enabling exposes
		// and what disabling does not undo, or the confirm click decides nothing.
		await expect(
			dialog.getByText(/no Coder account and no administrator approval/),
		).toBeInTheDocument();
		await expect(
			dialog.getByText(/does not revoke clients that already registered/),
		).toBeInTheDocument();

		await userEvent.click(dialog.getByRole("button", { name: "Enable" }));
		await expect(args.onChange).toHaveBeenCalledWith(true);
	},
};

export const CancelEnable: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const Updating: Story = {
	args: {
		isUpdating: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
	},
};

export const UpdatingWhileEnabled: Story = {
	args: {
		enabled: true,
		isUpdating: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: "Disable" }),
		).toBeDisabled();
	},
};

/**
 * The dialog's visibility follows only the admin's own intent, never the
 * server value. When the setting is enabled elsewhere while the dialog is
 * open, the dialog stays put and the admin closes it themselves. It must
 * never open, close, or reopen on its own as `enabled` changes underneath.
 *
 * The external-change buttons stack above the dialog's backdrop so they stay
 * clickable while it is open.
 */
export const SurvivesExternalEnabledChanges: Story = {
	render: function Harness(args) {
		const [enabled, setEnabled] = useState(false);

		return (
			<div className="flex flex-col gap-6">
				<div className="relative z-[1400] flex flex-row items-center gap-4">
					<button type="button" onClick={() => setEnabled(true)}>
						Simulate external enable
					</button>
					<button type="button" onClick={() => setEnabled(false)}>
						Simulate external disable
					</button>
				</div>

				<DynamicClientRegistrationSetting
					{...args}
					enabled={enabled}
					onChange={(next) => {
						setEnabled(next);
						args.onChange(next);
					}}
				/>
			</div>
		);
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const title = "Enable Dynamic Client Registration?";

		// The dialog animates in and out over ~225ms, so it is present but
		// transparent on the way in and still opaque on the way out. Anything
		// asserting that the dialog did not close has to outlast that window, or
		// a dialog already fading out still reads as visible.
		const settleTransition = () =>
			new Promise((resolve) => setTimeout(resolve, 400));

		// Grab these before opening the dialog. MUI's modal marks everything
		// outside itself aria-hidden, and role queries skip aria-hidden nodes,
		// so a lookup by role after the dialog opens will not find them.
		const externalEnable = canvas.getByRole("button", {
			name: "Simulate external enable",
		});
		const externalDisable = canvas.getByRole("button", {
			name: "Simulate external disable",
		});

		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));
		await waitFor(() => expect(body.getByText(title)).toBeVisible());
		const dialog = body.getByTestId("dialog");

		// Enabled elsewhere. The dialog ignores it: the admin's intent to
		// confirm is theirs to resolve, not the server's.
		await userEvent.click(externalEnable);
		await expect(canvas.getByText("Enabled")).toBeVisible();
		await settleTransition();
		await expect(body.getByText(title)).toBeVisible();
		// Still the same node, so it was never torn down and rebuilt.
		await expect(body.getByTestId("dialog")).toBe(dialog);

		// And disabled again, the transition that used to resurrect it.
		await userEvent.click(externalDisable);
		await settleTransition();
		await expect(body.getByTestId("dialog")).toBe(dialog);

		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(body.queryByText(title)).not.toBeInTheDocument(),
		);

		// Once the admin has closed it, no amount of external churn brings it
		// back.
		await userEvent.click(externalEnable);
		await userEvent.click(externalDisable);
		await settleTransition();
		await expect(body.queryByText(title)).not.toBeInTheDocument();
		await expect(args.onChange).not.toHaveBeenCalled();
	},
};

// Disabling skips the confirmation dialog, unlike enabling.
export const Disable: Story = {
	args: {
		enabled: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Disable" }));

		await expect(args.onChange).toHaveBeenCalledWith(false);
	},
};
