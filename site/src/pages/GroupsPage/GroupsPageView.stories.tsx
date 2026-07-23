import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { MockGroup } from "#/testHelpers/entities";
import { GroupsPageView, type GroupWithSpend } from "./GroupsPageView";

const meta: Meta<typeof GroupsPageView> = {
	title: "pages/OrganizationGroupsPage",
	component: GroupsPageView,
};

export default meta;
type Story = StoryObj<typeof GroupsPageView>;

const mockGroupWithSpend: GroupWithSpend = {
	...MockGroup,
	spend: undefined,
};

const aiGroup = (id: string, name: string): GroupWithSpend => ({
	...mockGroupWithSpend,
	id,
	name,
	display_name: name,
});

export const NotEnabled: Story = {
	args: {
		groups: [{ ...mockGroupWithSpend }],
		canCreateGroup: true,
		groupsEnabled: false,
	},
};

export const WithGroups: Story = {
	args: {
		groups: [{ ...mockGroupWithSpend }],
		canCreateGroup: true,
		groupsEnabled: true,
	},
};

export const WithAIBudgets: Story = {
	args: {
		canCreateGroup: true,
		groupsEnabled: true,
		showAIBudget: true,
		groups: [
			{
				...aiGroup("ai-unlimited", "Unlimited"),
				spend: {
					group_id: "ai-unlimited",
					current_spend_micros: 25_492_000_000,
					spend_limit_micros: null,
				},
			},
			{
				...aiGroup("ai-under", "Under budget"),
				spend: {
					group_id: "ai-under",
					current_spend_micros: 10_000_000,
					spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-warning", "Near limit"),
				spend: {
					group_id: "ai-warning",
					current_spend_micros: 46_000_000,
					spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-at-limit", "At limit"),
				spend: {
					group_id: "ai-at-limit",
					current_spend_micros: 50_000_000,
					spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-over", "Over budget"),
				spend: {
					group_id: "ai-over",
					current_spend_micros: 75_000_000,
					spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-zero-budget", "Zero budget"),
				spend: {
					group_id: "ai-zero-budget",
					current_spend_micros: 5_000_000,
					spend_limit_micros: 0,
				},
			},
			{
				...aiGroup("ai-zero-both", "Zero spend and budget"),
				spend: {
					group_id: "ai-zero-both",
					current_spend_micros: 0,
					spend_limit_micros: 0,
				},
			},
			// No spend exercises the missing-spend em-dash fallback.
			aiGroup("ai-no-data", "No data"),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-unlimited"),
		).toHaveTextContent("$25,492 / Unlimited USD");
		await expect(await canvas.findByTestId("group-ai-under")).toHaveTextContent(
			"$10 / $50 USD",
		);
		await expect(
			await canvas.findByTestId("group-ai-warning"),
		).toHaveTextContent("$46 / $50 USD");
		await expect(
			await canvas.findByTestId("group-ai-at-limit"),
		).toHaveTextContent("$50 / $50 USD");
		await expect(
			await canvas.findByTestId("group-ai-zero-budget"),
		).toHaveTextContent("$5 / $0 USD");
		await expect(
			await canvas.findByTestId("group-ai-no-data"),
		).toHaveTextContent("\u2014");
	},
};

// Groups still loading: the table shows skeleton rows including the AI column.
export const WithAIBudgetsLoading: Story = {
	args: {
		groups: undefined,
		canCreateGroup: true,
		groupsEnabled: true,
		showAIBudget: true,
	},
};

// Spend still loading: every AI budget cell falls back to an em dash.
export const WithAIBudgetsSpendLoading: Story = {
	args: {
		groups: [aiGroup("ai-loading", "Spend loading")],
		canCreateGroup: true,
		groupsEnabled: true,
		showAIBudget: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-loading"),
		).toHaveTextContent("\u2014");
	},
};

// The spend fetch failed: the column header shows a warning and cells an em dash.
export const WithAIBudgetsSpendError: Story = {
	args: {
		groups: [aiGroup("ai-errored", "Spend errored")],
		spendError: true,
		canCreateGroup: true,
		groupsEnabled: true,
		showAIBudget: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-errored"),
		).toHaveTextContent("\u2014");
		await expect(
			canvas.getByRole("button", { name: "More info" }),
		).toBeInTheDocument();
	},
};

// Cost control unset for a group: the cell falls back to an em dash.
export const WithAIBudgetsSpendUnavailable: Story = {
	args: {
		groups: [aiGroup("ai-unavailable", "Spend unavailable")],
		canCreateGroup: true,
		groupsEnabled: true,
		showAIBudget: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-unavailable"),
		).toHaveTextContent("\u2014");
	},
};

// AI Bridge hidden: no AI budget column.
export const WithoutAIBudgetColumn: Story = {
	args: {
		groups: [aiGroup("ai-hidden", "No AI column")],
		canCreateGroup: true,
		groupsEnabled: true,
		showAIBudget: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("AI budget")).not.toBeInTheDocument();
	},
};

export const WithDisplayGroup: Story = {
	args: {
		groups: [{ ...mockGroupWithSpend, name: "front-end" }],
		canCreateGroup: true,
		groupsEnabled: true,
	},
};

export const EmptyGroup: Story = {
	args: {
		groups: [],
		canCreateGroup: false,
		groupsEnabled: true,
	},
};

export const EmptyGroupWithPermission: Story = {
	args: {
		groups: [],
		canCreateGroup: true,
		groupsEnabled: true,
	},
};
