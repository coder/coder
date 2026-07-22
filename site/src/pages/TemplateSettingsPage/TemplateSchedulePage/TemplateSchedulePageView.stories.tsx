import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "react-query";
import { action } from "storybook/actions";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockTemplate } from "#/testHelpers/entities";
import { TemplateSchedulePageView } from "./TemplateSchedulePageView";

const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			retry: false,
			gcTime: 0,
			refetchOnWindowFocus: false,
			networkMode: "offlineFirst",
		},
	},
});

const meta: Meta<typeof TemplateSchedulePageView> = {
	title: "pages/TemplateSettingsPage/TemplateSchedulePageView",
	component: TemplateSchedulePageView,
	decorators: [
		(Story) => (
			<QueryClientProvider client={queryClient}>
				<Story />
			</QueryClientProvider>
		),
	],
};
export default meta;
type Story = StoryObj<typeof TemplateSchedulePageView>;

const defaultArgs = {
	allowAdvancedScheduling: true,
	template: MockTemplate,
	onSubmit: action("onSubmit"),
	onCancel: action("cancel"),
};

export const Example: Story = {
	args: { ...defaultArgs },
};

export const CantSetMaxTTL: Story = {
	args: { ...defaultArgs, allowAdvancedScheduling: false },
};

export const SubmitClearsActivityBumpWhenDefaultTTLIsZero: Story = {
	args: {
		...defaultArgs,
		template: {
			...MockTemplate,
			// Start with a non-zero activity bump so we can verify
			// it gets discarded when default TTL is set to 0.
			activity_bump_ms: 3 * 60 * 60 * 1000,
			// Disable user autostop so the guard applies (activity bump
			// remains editable when either default TTL or user autostop is
			// enabled).
			allow_user_autostop: false,
		},
		onSubmit: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const defaultTtlField = await canvas.findByLabelText(
			"Default autostop (hours)",
		);
		const activityBumpField = canvas.getByLabelText("Activity bump (hours)");

		await user.clear(defaultTtlField);
		await user.type(defaultTtlField, "0");

		await expect(activityBumpField).toBeDisabled();

		// Helper text explains why the field is disabled.
		await expect(
			canvas.getByText(
				/activity bump only applies when "default autostop" is configured or users are allowed to customize autostop/i,
			),
		).toBeInTheDocument();

		const submitButton = canvas.getByRole("button", { name: /save/i });
		await user.click(submitButton);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					activity_bump_ms: undefined,
				}),
			);
		});
	},
};

export const SubmitPreservesActivityBumpWhenAllowUserAutostopIsEnabled: Story =
	{
		args: {
			...defaultArgs,
			template: {
				...MockTemplate,
				activity_bump_ms: 3 * 60 * 60 * 1000,
				// Users can customize autostop on their workspaces, so activity
				// bump still has meaning even without a default TTL.
				allow_user_autostop: true,
			},
			onSubmit: fn(),
		},
		play: async ({ canvasElement, args }) => {
			const canvas = within(canvasElement);
			const user = userEvent.setup();

			const defaultTtlField = await canvas.findByLabelText(
				"Default autostop (hours)",
			);
			const activityBumpField = canvas.getByLabelText("Activity bump (hours)");

			await user.clear(defaultTtlField);
			await user.type(defaultTtlField, "0");

			// Activity bump stays enabled because allow_user_autostop is on.
			await expect(activityBumpField).not.toBeDisabled();

			const submitButton = canvas.getByRole("button", { name: /save/i });
			await user.click(submitButton);

			await waitFor(() => {
				expect(args.onSubmit).toHaveBeenCalledWith(
					expect.objectContaining({
						activity_bump_ms: 3 * 60 * 60 * 1000,
					}),
				);
			});
		},
	};

export const ReEnablesActivityBumpWhenDefaultTTLIsSetBack: Story = {
	args: {
		...defaultArgs,
		template: {
			...MockTemplate,
			allow_user_autostop: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const defaultTtlField = await canvas.findByLabelText(
			"Default autostop (hours)",
		);
		const activityBumpField = canvas.getByLabelText("Activity bump (hours)");

		// Guard fires: default TTL is 0 and user autostop is off.
		await user.clear(defaultTtlField);
		await user.type(defaultTtlField, "0");
		await expect(activityBumpField).toBeDisabled();

		// Setting default TTL back to a non-zero value re-enables the field.
		await user.clear(defaultTtlField);
		await user.type(defaultTtlField, "8");
		await expect(activityBumpField).not.toBeDisabled();
	},
};

export const ReEnablesActivityBumpWhenAllowUserAutostopIsToggledOn: Story = {
	args: {
		...defaultArgs,
		template: {
			...MockTemplate,
			allow_user_autostop: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const defaultTtlField = await canvas.findByLabelText(
			"Default autostop (hours)",
		);
		const activityBumpField = canvas.getByLabelText("Activity bump (hours)");
		const allowUserAutostopCheckbox = canvas.getByRole("checkbox", {
			name: /allow users to customize autostop/i,
		});

		// Guard fires: default TTL is 0 and user autostop is off.
		await user.clear(defaultTtlField);
		await user.type(defaultTtlField, "0");
		await expect(activityBumpField).toBeDisabled();

		// Toggling user autostop back on re-enables the field without
		// touching the default TTL.
		await user.click(allowUserAutostopCheckbox);
		await expect(activityBumpField).not.toBeDisabled();
	},
};

export const AutostopReminderHelperTextHidesWhenAllowUserAutostopIsEnabled: Story =
	{
		args: {
			...defaultArgs,
			template: {
				...MockTemplate,
				// The reminder helper text used to always show the "no autostop
				// deadline" hint when the template had no default TTL and no
				// autostop requirement. It should now suppress the hint when
				// allow_user_autostop is on, since users still have a deadline.
				allow_user_autostop: true,
				autostop_requirement: { days_of_week: [], weeks: 1 },
				time_til_autostop_notify_ms: 0,
			},
		},
		play: async ({ canvasElement }) => {
			const canvas = within(canvasElement);
			const user = userEvent.setup();

			const defaultTtlField = await canvas.findByLabelText(
				"Default autostop (hours)",
			);

			// Clear default TTL so the only remaining deadline source is
			// allow_user_autostop.
			await user.clear(defaultTtlField);
			await user.type(defaultTtlField, "0");

			// The "no autostop deadline" hint must NOT appear.
			await expect(
				canvas.queryByText(
					/autostop reminders only apply when an autostop deadline is configured/i,
				),
			).not.toBeInTheDocument();
		},
	};

export const SubmitAutostopReminderConvertsHoursToMs: Story = {
	args: {
		...defaultArgs,
		template: {
			...MockTemplate,
			time_til_autostop_notify_ms: 0,
		},
		onSubmit: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const reminderField = await canvas.findByLabelText(
			"Autostop reminder (hours)",
		);

		await user.clear(reminderField);
		await user.type(reminderField, "2");

		const submitButton = canvas.getByRole("button", { name: /save/i });
		await user.click(submitButton);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					time_til_autostop_notify_ms: 2 * 60 * 60 * 1000,
				}),
			);
		});
	},
};

export const SubmitClearsAutostopReminder: Story = {
	args: {
		...defaultArgs,
		template: {
			...MockTemplate,
			// Start with a non-zero autostop reminder so we can verify
			// it gets cleared to 0 when the field is emptied.
			time_til_autostop_notify_ms: 2 * 60 * 60 * 1000,
		},
		onSubmit: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const reminderField = await canvas.findByLabelText(
			"Autostop reminder (hours)",
		);

		await user.clear(reminderField);
		await user.type(reminderField, "0");

		const submitButton = canvas.getByRole("button", { name: /save/i });
		await user.click(submitButton);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					time_til_autostop_notify_ms: 0,
				}),
			);
		});
	},
};
