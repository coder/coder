import { MockTemplate } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "react-query";
import { action } from "storybook/actions";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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
