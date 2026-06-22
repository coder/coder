import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Group } from "#/api/typesGenerated";
import { MockGroup } from "#/testHelpers/entities";
import { GroupsPageView } from "./GroupsPageView";

const meta: Meta<typeof GroupsPageView> = {
	title: "pages/OrganizationGroupsPage",
	component: GroupsPageView,
};

export default meta;
type Story = StoryObj<typeof GroupsPageView>;

const aiGroup = (id: string, name: string): Group => ({
	...MockGroup,
	id,
	name,
	display_name: name,
});

export const NotEnabled: Story = {
	args: {
		groups: [MockGroup],
		canCreateGroup: true,
		groupsEnabled: false,
	},
};

export const WithGroups: Story = {
	args: {
		groups: [MockGroup],
		canCreateGroup: true,
		groupsEnabled: true,
	},
};

export const WithAIBudgets: Story = {
	args: {
		canCreateGroup: true,
		groupsEnabled: true,
		groups: [
			aiGroup("ai-unlimited", "Unlimited"),
			aiGroup("ai-under", "Under budget"),
			aiGroup("ai-warning", "Near limit"),
			aiGroup("ai-at-limit", "At limit"),
			aiGroup("ai-over", "Over budget"),
			aiGroup("ai-zero-budget", "Zero budget"),
			aiGroup("ai-zero-both", "Zero spend and budget"),
			aiGroup("ai-no-data", "No data"),
		],
		aiBudget: {
			isLoading: false,
			// "ai-no-data" is omitted to exercise the missing-spend "-" fallback.
			spend: [
				{
					group_id: "ai-unlimited",
					current_spend_micros: 25_492_000_000,
					spend_limit_micros: null,
				},
				{
					group_id: "ai-under",
					current_spend_micros: 10_000_000,
					spend_limit_micros: 50_000_000,
				},
				{
					group_id: "ai-warning",
					current_spend_micros: 46_000_000,
					spend_limit_micros: 50_000_000,
				},
				{
					group_id: "ai-at-limit",
					current_spend_micros: 50_000_000,
					spend_limit_micros: 50_000_000,
				},
				{
					group_id: "ai-over",
					current_spend_micros: 75_000_000,
					spend_limit_micros: 50_000_000,
				},
				{
					group_id: "ai-zero-budget",
					current_spend_micros: 5_000_000,
					spend_limit_micros: 0,
				},
				{
					group_id: "ai-zero-both",
					current_spend_micros: 0,
					spend_limit_micros: 0,
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-unlimited"),
		).toHaveTextContent("$25,492 / unlimited USD");
		await expect(await canvas.findByTestId("group-ai-under")).toHaveTextContent(
			"$10 / $50 USD",
		);
		await expect(
			await canvas.findByTestId("group-ai-at-limit"),
		).toHaveTextContent("$50 / $50 USD");
		await expect(
			await canvas.findByTestId("group-ai-zero-budget"),
		).toHaveTextContent("$5 / $0 USD");
		await expect(
			await canvas.findByTestId("group-ai-no-data"),
		).toHaveTextContent("-");
	},
};

export const WithAIBudgetsLoading: Story = {
	args: {
		groups: [MockGroup],
		canCreateGroup: true,
		groupsEnabled: true,
		aiBudget: { spend: undefined, isLoading: true },
	},
};

export const WithDisplayGroup: Story = {
	args: {
		groups: [{ ...MockGroup, name: "front-end" }],
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
