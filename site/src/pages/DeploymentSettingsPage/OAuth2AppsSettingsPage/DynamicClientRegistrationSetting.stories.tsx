import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
	expect,
	fireEvent,
	fn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { DynamicClientRegistrationSetting } from "./DynamicClientRegistrationSetting";

const meta: Meta<typeof DynamicClientRegistrationSetting> = {
	title:
		"pages/DeploymentSettingsPage/OAuth2AppsSettingsPage/DynamicClientRegistrationSetting",
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
		// The endpoint is what an admin has to hand a client after enabling.
		await expect(canvas.getByText("/oauth2/register")).toBeVisible();
		// Stated in both states. An admin who has already disabled still needs to
		// know that clients registered earlier were not revoked.
		await expect(
			canvas.getByText(/keep working until an administrator deletes them/),
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
			canvas.getByText(/keep working until an administrator deletes them/),
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

		// The dialog covers the section description, so it is the last thing the
		// admin reads before enabling. It has to name both what enabling exposes
		// and what disabling does not undo, or the confirm click decides nothing.
		await expect(
			body.getByText(/no Coder account and no administrator approval/),
		).toBeInTheDocument();
		await expect(
			body.getByText(/does not revoke clients that already registered/),
		).toBeInTheDocument();

		await userEvent.click(body.getByTestId("confirm-button"));
		await expect(args.onChange).toHaveBeenCalledWith(true);
		// Closing is asserted twice on the cancel path and was asserted nowhere on
		// this one, which is the direction that opens the endpoint. A modal left
		// standing would cover the badge that reports the save worked.
		await waitFor(() =>
			expect(
				body.queryByText("Enable Dynamic Client Registration?"),
			).not.toBeInTheDocument(),
		);
	},
};

export const CancelEnable: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		const title = "Enable Dynamic Client Registration?";
		await waitFor(() => expect(body.getByText(title)).toBeVisible());
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// Cancelling closes the dialog, which is the only thing this story can
		// prove. `onChange` is unreachable from a cancel click, so asserting it
		// was not called would hold even against an `onClose` that does nothing.
		await waitFor(() =>
			expect(body.queryByText(title)).not.toBeInTheDocument(),
		);
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
		// `aria-disabled` does not stop a keyboard Enter; the guard in `onClick`
		// does. In this direction Enter would open the dialog, not call `onChange`,
		// so the call assertion above cannot see the guard go missing.
		await expect(
			within(canvasElement.ownerDocument.body).queryByText(
				"Enable Dynamic Client Registration?",
			),
		).not.toBeInTheDocument();

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
		const [pending, setPending] = useState<boolean | undefined>(undefined);

		return (
			<div className="flex flex-col gap-6">
				{/*
				 * The request ends when the story says so, not when a timer says so.
				 * A timer would race the assertions that require the in-flight state,
				 * and this suite is documented to stall under CPU contention.
				 */}
				<button
					type="button"
					onClick={() => {
						if (pending !== undefined) {
							setEnabled(pending);
							setPending(undefined);
							setIsUpdating(false);
						}
					}}
				>
					Finish request
				</button>

				<DynamicClientRegistrationSetting
					{...args}
					enabled={enabled}
					isUpdating={isUpdating}
					onChange={(next) => {
						setIsUpdating(true);
						setPending(next);
					}}
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: "Disable" });
		const finish = canvas.getByRole("button", { name: "Finish request" });

		button.focus();
		await userEvent.keyboard("{Enter}");

		// Mid-request, and it stays mid-request until the click below. A `disabled`
		// attribute here would have blurred to <body>.
		await expect(button).toHaveAttribute("aria-disabled", "true");
		await expect(button).toHaveFocus();

		// `fireEvent`, not `userEvent`: clicking with a pointer would move focus to
		// the harness button and destroy the state under test.
		fireEvent.click(finish);

		// The same element becomes the opposite action once the request lands, and
		// focus rides along rather than resetting to the top of the document.
		await waitFor(() => {
			expect(canvas.getByRole("button", { name: "Enable" })).toBeVisible();
		});
		await expect(button).toHaveFocus();
	},
};

/**
 * The enable path opens a dialog, and closing one returns focus to whatever
 * opened it. `ConfirmDialog` renders no Radix trigger, so Radix has nothing to
 * restore to and focus would otherwise land on `<body>`, which is the same loss
 * the in-flight handling above exists to prevent.
 */
export const KeepsFocusAfterConfirming: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const button = canvas.getByRole("button", { name: "Enable" });

		button.focus();
		await userEvent.keyboard("{Enter}");
		await waitFor(() => expect(body.getByTestId("dialog")).toBeVisible());

		await userEvent.click(body.getByTestId("confirm-button"));
		await waitFor(() =>
			expect(body.queryByTestId("dialog")).not.toBeInTheDocument(),
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
 * The external change is driven by the story rather than a timer, and applied
 * with `fireEvent` so no `pointerdown` reaches Radix's dismiss layer. A real
 * pointer click outside a modal dialog closes it, which would destroy the state
 * under test.
 */
export const SurvivesExternalEnabledChanges: Story = {
	render: function Harness(args) {
		const [enabled, setEnabled] = useState(false);

		return (
			<div className="flex flex-col gap-6">
				<div className="flex flex-row gap-4">
					<button type="button" onClick={() => setEnabled(true)}>
						Enable externally
					</button>
					<button type="button" onClick={() => setEnabled(false)}>
						Disable externally
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

		// Role queries skip the story root once the modal marks it aria-hidden, so
		// these are found by text and captured before the dialog opens.
		const enableExternally = canvas.getByText("Enable externally");
		const disableExternally = canvas.getByText("Disable externally");

		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));
		await waitFor(() => expect(body.getByText(title)).toBeVisible());
		const dialog = body.getByTestId("dialog");

		// The external change lands here, with the dialog open. The dialog ignores
		// it: the admin's intent to confirm is theirs to resolve, not the server's.
		fireEvent.click(enableExternally);

		// Radix flips `data-state` to "closed" the moment something closes the
		// dialog, so this needs no waiting and cannot be fooled by an animation
		// still in progress.
		await expect(dialog).toHaveAttribute("data-state", "open");
		await expect(body.getByTestId("dialog")).toBe(dialog);

		// Cancelling is the admin's own action, so it closes.
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(body.queryByText(title)).not.toBeInTheDocument(),
		);

		// Returning to disabled must not reopen the dialog.
		fireEvent.click(disableExternally);
		await expect(body.queryByText(title)).not.toBeInTheDocument();
		await expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const DisableSkipsConfirmationDialog: Story = {
	args: {
		enabled: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Disable" }));

		await expect(args.onChange).toHaveBeenCalledWith(false);
	},
};
