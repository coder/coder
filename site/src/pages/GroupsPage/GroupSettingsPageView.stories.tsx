import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockGroup } from "#/testHelpers/entities";
import GroupSettingsPageView from "./GroupSettingsPageView";

const meta: Meta<typeof GroupSettingsPageView> = {
	title: "pages/OrganizationGroupsPage/GroupSettingsPageView",
	component: GroupSettingsPageView,
	args: {
		onCancel: fn(),
		onSubmit: fn(),
		group: MockGroup,
		showAISettings: false,
		initialBudgetDollars: null,
		formErrors: undefined,
		isUpdating: false,
	},
};

export default meta;
type Story = StoryObj<typeof GroupSettingsPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Without the AI add-on, the AI budget section is hidden.
		await expect(canvas.queryByText("AI budget")).not.toBeInTheDocument();
	},
};

export const WithAIBudget: Story = {
	args: {
		showAISettings: true,
		group: { ...MockGroup, total_member_count: 7 },
		initialBudgetDollars: 1000,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("AI budget")).toBeInTheDocument();
		const helper = canvas.getByText(/month maximum/i);
		await expect(helper).toHaveTextContent(
			"$7,000/month maximum, based on 7 members.",
		);
	},
};

export const AIBudgetUncapped: Story = {
	args: {
		showAISettings: true,
		initialBudgetDollars: null,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Leave empty for uncapped spend."),
		).toBeInTheDocument();
	},
};

export const AIBudgetDisabled: Story = {
	args: {
		showAISettings: true,
		group: { ...MockGroup, total_member_count: 7 },
		initialBudgetDollars: 0,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A budget of 0 is valid and reads as disabled spend.
		const helper = canvas.getByText(/month maximum/i);
		await expect(helper).toHaveTextContent(
			"$0/month maximum, based on 7 members.",
		);
	},
};

export const AIBudgetDecimal: Story = {
	args: {
		showAISettings: true,
		group: { ...MockGroup, total_member_count: 1 },
		initialBudgetDollars: 99.99,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Cents are kept when the amount is not a whole dollar.
		const helper = canvas.getByText(/month maximum/i);
		await expect(helper).toHaveTextContent(
			"$99.99/month maximum, based on 1 member.",
		);
	},
};

export const SaveWithBudget: Story = {
	args: {
		showAISettings: true,
		initialBudgetDollars: null,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Monthly budget per member (USD)");
		await userEvent.type(input, "25");
		await userEvent.click(canvas.getByRole("button", { name: "Save" }));
		// onSubmit fires asynchronously with (values, formikHelpers).
		await waitFor(() =>
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ monthly_budget_per_member: "25" }),
				expect.anything(),
			),
		);
	},
};
