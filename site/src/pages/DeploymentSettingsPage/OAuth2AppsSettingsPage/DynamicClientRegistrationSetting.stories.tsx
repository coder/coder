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
		// Stated in both states. An admin who has already disabled still needs to
		// know that clients registered earlier were not revoked.
		await expect(
			canvas.getByText(/keep working until you remove them/),
		).toBeVisible();
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
		await expect(
			canvas.getByText(/keep working until you remove them/),
		).toBeVisible();
	},
};

export const ReadOnly: Story = {
	args: {
		canEdit: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
		// A disabled button is skipped by Tab and fires no pointer events, so the
		// reason has to be readable on the page rather than attached to it.
		await expect(
			canvas.getByText(/permission to edit deployment configuration/),
		).toBeVisible();
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
		await expect(
			canvas.getByText(/permission to edit deployment configuration/),
		).toBeVisible();
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
		const title = "Enable Dynamic Client Registration?";
		await waitFor(() => expect(body.getByText(title)).toBeVisible());
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// Cancelling closing the dialog is the only thing this story protects.
		// `onChange` is unreachable from a cancel click, so asserting it was not
		// called would hold even against an `onClose` that does nothing.
		await waitFor(() =>
			expect(body.queryByText(title)).not.toBeInTheDocument(),
		);
		await expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const Updating: Story = {
	args: {
		isUpdating: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: "Enable" });

		// Inert but still focusable, unlike the read-only case: an in-flight
		// request must not blur the element the admin is standing on.
		await expect(button).toHaveAttribute("aria-disabled", "true");
		button.focus();
		await expect(button).toHaveFocus();
		await userEvent.keyboard("{Enter}");
		await expect(args.onChange).not.toHaveBeenCalled();

		// Disabled mid-request is self-evident and momentary. Only a permission
		// problem earns an explanation.
		await expect(
			canvas.queryByText(/permission to edit deployment configuration/),
		).not.toBeInTheDocument();
	},
};

export const UpdatingWhileEnabled: Story = {
	args: {
		enabled: true,
		isUpdating: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: "Disable" });

		await expect(button).toHaveAttribute("aria-disabled", "true");
		button.focus();
		await expect(button).toHaveFocus();
		await userEvent.keyboard("{Enter}");
		await expect(args.onChange).not.toHaveBeenCalled();
	},
};

/**
 * Flipping the setting must not cost a keyboard user their place. The button
 * goes inert while the request is in flight rather than disabled, so focus
 * stays on it through the transition and the label change.
 */
export const KeepsFocusWhileUpdating: Story = {
	args: {
		enabled: true,
	},
	render: function Harness(args) {
		const [enabled, setEnabled] = useState(true);
		const [isUpdating, setIsUpdating] = useState(false);

		return (
			<DynamicClientRegistrationSetting
				{...args}
				enabled={enabled}
				isUpdating={isUpdating}
				onChange={(next) => {
					setIsUpdating(true);
					// Stands in for the mutation round trip, which is the window where
					// a natively disabled button would blur.
					setTimeout(() => {
						setEnabled(next);
						setIsUpdating(false);
					}, 50);
				}}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: "Disable" });

		button.focus();
		await userEvent.keyboard("{Enter}");

		// Mid-request. A `disabled` attribute here would have blurred to <body>.
		await expect(button).toHaveAttribute("aria-disabled", "true");
		await expect(button).toHaveFocus();

		// The same element becomes the opposite action once the request lands, and
		// focus rides along rather than resetting to the top of the document.
		await waitFor(() =>
			expect(canvas.getByRole("button", { name: "Enable" })).toBeVisible(),
		);
		await expect(button).toHaveFocus();
	},
};

/**
 * The dialog's visibility follows only the admin's own intent, never the
 * server value. When the setting is enabled elsewhere while the dialog is
 * open, the dialog stays put and the admin closes it themselves. It must
 * never open, close, or reopen on its own as `enabled` changes underneath.
 *
 * The external change is armed on a timer rather than driven by a control
 * clicked mid-dialog. The dialog is modal, so any pointer interaction outside
 * it dismisses it, which would destroy the state under test.
 */
export const SurvivesExternalEnabledChanges: Story = {
	render: function Harness(args) {
		const [enabled, setEnabled] = useState(false);

		return (
			<div className="flex flex-col gap-6">
				<button
					type="button"
					onClick={() => {
						// Lands while the dialog is open, standing in for another admin
						// enabling the setting and this tab refetching.
						setTimeout(() => setEnabled(true), 150);
					}}
				>
					Arm external enable
				</button>

				<button type="button" onClick={() => setEnabled(false)}>
					Set externally disabled
				</button>

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

		// The dialog animates in and out, so it is present but transparent on the
		// way in and still opaque on the way out. Anything asserting that the
		// dialog did not close has to outlast that window, or a dialog already
		// animating out still reads as visible.
		const settleTransition = () =>
			new Promise((resolve) => setTimeout(resolve, 400));

		await userEvent.click(
			canvas.getByRole("button", { name: "Arm external enable" }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));
		await waitFor(() => expect(body.getByText(title)).toBeVisible());
		const dialog = body.getByTestId("dialog");

		// The armed change lands here. The dialog ignores it: the admin's intent
		// to confirm is theirs to resolve, not the server's.
		await settleTransition();
		await expect(body.getByText(title)).toBeVisible();
		// Still the same node, so it was never torn down and rebuilt.
		await expect(body.getByTestId("dialog")).toBe(dialog);

		// Cancelling is the admin's own action, so it closes.
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(body.queryByText(title)).not.toBeInTheDocument(),
		);

		// Going back to disabled is the transition that used to resurrect it.
		await userEvent.click(
			canvas.getByRole("button", { name: "Set externally disabled" }),
		);
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
