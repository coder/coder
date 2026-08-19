import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { maxAIBudgetDollars } from "#/modules/groups";
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
		await expect(canvas.getByLabelText("Monthly limit per member")).toHaveValue(
			1000,
		);
		const helper = canvas.getByText(/month, based on/i);
		await expect(helper).toHaveTextContent(
			"This group's limit is $7,000/month, based on 7 members.",
		);
		await expect(
			canvas.getByRole("link", {
				name: /learn how budgets apply across groups/i,
			}),
		).toBeInTheDocument();
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
			canvas.getByLabelText("Monthly limit per member"),
		).toHaveAttribute("placeholder", "no budget");
		await expect(
			canvas.getByText("This group doesn't have a budget set."),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(/Members will fall back to another group's limit/),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", {
				name: /learn how budgets apply across groups/i,
			}),
		).toHaveAttribute(
			"href",
			expect.stringContaining(
				"/ai-coder/ai-gateway/cost-controls#effective-group-resolution",
			),
		);
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
		await expect(canvas.getByLabelText("Monthly limit per member")).toHaveValue(
			0,
		);
		const summary = canvas.getByText(/This group's limit has been set to/);
		await expect(summary).toHaveTextContent(
			"This group's limit has been set to $0.",
		);
		await expect(
			canvas.getByText(
				/A \$0 limit blocks AI access for members that aren't in another group/,
			),
		).toBeInTheDocument();
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
		const helper = canvas.getByText(/month, based on/i);
		await expect(helper).toHaveTextContent(
			"This group's limit is $99.99/month, based on 1 member.",
		);
	},
};

// A budget above the configurable maximum blocks saving.
export const AIBudgetAboveMaximum: Story = {
	args: {
		showAISettings: true,
		initialBudgetDollars: null,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Monthly limit per member");

		await userEvent.type(input, String(maxAIBudgetDollars + 1));
		// Blur to surface the error, matching the touched-then-validate flow.
		await userEvent.tab();
		await expect(
			await canvas.findByText("Enter an amount between 0 and $1,000,000."),
		).toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "Save" }));
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const SaveWithBudget: Story = {
	args: {
		showAISettings: true,
		initialBudgetDollars: null,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Monthly limit per member");
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
