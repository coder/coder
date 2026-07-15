import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { GroupAICostControl, GroupWithAICostControl } from "#/api/api";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import { MockGroup } from "#/testHelpers/entities";
import { GroupsPageView } from "./GroupsPageView";

const meta: Meta<typeof GroupsPageView> = {
	title: "pages/OrganizationGroupsPage",
	component: GroupsPageView,
	args: {
		canCreateGroup: true,
		groupsEnabled: true,
		filterProps: {
			filter: {
				query: "",
				values: {},
				update: () => {},
				debounceUpdate: () => {},
				cancelDebounce: () => {},
				used: false,
			},
		},
		groupsQuery: {
			...mockSuccessResult,
			totalRecords: 1,
		} as UsePaginatedQueryResult,
	},
};

export default meta;
type Story = StoryObj<typeof GroupsPageView>;

const aiGroup = (
	id: string,
	name: string,
	ai_cost_control?: GroupAICostControl,
): GroupWithAICostControl => ({
	...MockGroup,
	id,
	name,
	display_name: name,
	ai_cost_control,
});

export const Default: Story = {};

export const NotEnabled: Story = {
	args: {
		groups: [MockGroup],
		groupsEnabled: false,
	},
};

export const WithGroups: Story = {
	args: {
		groups: [MockGroup],
	},
};

// Multiple pages of results with the search field in use.
export const WithSearchAndPagination: Story = {
	args: {
		groups: [
			aiGroup("group-a", "Group A"),
			aiGroup("group-b", "Group B"),
			aiGroup("group-c", "Group C"),
		],
		filterProps: {
			filter: {
				query: "group",
				values: {},
				update: () => {},
				debounceUpdate: () => {},
				cancelDebounce: () => {},
				used: true,
			},
		},
		groupsQuery: {
			...mockSuccessResult,
			totalRecords: 60,
			totalPages: 3,
			hasNextPage: true,
		} as UsePaginatedQueryResult,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Filter")).toHaveValue("group");
	},
};

// Groups still loading: the pagination + table render their loading states.
export const Loading: Story = {
	args: {
		groups: undefined,
		groupsQuery: mockInitialRenderResult as UsePaginatedQueryResult,
	},
};

export const WithAIBudgets: Story = {
	args: {
		showAIBudget: true,
		groups: [
			aiGroup("ai-unlimited", "Unlimited", {
				current_spend_micros: 25_492_000_000,
				spend_limit_micros: null,
			}),
			aiGroup("ai-under", "Under budget", {
				current_spend_micros: 10_000_000,
				spend_limit_micros: 50_000_000,
			}),
			aiGroup("ai-warning", "Near limit", {
				current_spend_micros: 46_000_000,
				spend_limit_micros: 50_000_000,
			}),
			aiGroup("ai-at-limit", "At limit", {
				current_spend_micros: 50_000_000,
				spend_limit_micros: 50_000_000,
			}),
			aiGroup("ai-over", "Over budget", {
				current_spend_micros: 75_000_000,
				spend_limit_micros: 50_000_000,
			}),
			aiGroup("ai-zero-budget", "Zero budget", {
				current_spend_micros: 5_000_000,
				spend_limit_micros: 0,
			}),
			aiGroup("ai-zero-both", "Zero spend and budget", {
				current_spend_micros: 0,
				spend_limit_micros: 0,
			}),
			// No cost control exercises the missing-spend "-" fallback.
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
		).toHaveTextContent("-");
	},
};

// Groups still loading: the table shows skeleton rows including the AI column.
export const WithAIBudgetsLoading: Story = {
	args: {
		groups: undefined,
		showAIBudget: true,
		groupsQuery: mockInitialRenderResult as UsePaginatedQueryResult,
	},
};

// Cost control unset for a group: the cell falls back to "-".
export const WithAIBudgetsSpendUnavailable: Story = {
	args: {
		groups: [aiGroup("ai-unavailable", "Spend unavailable")],
		showAIBudget: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-unavailable"),
		).toHaveTextContent("-");
	},
};

// AI Bridge hidden: no AI budget column.
export const WithoutAIBudgetColumn: Story = {
	args: {
		groups: [aiGroup("ai-hidden", "No AI column")],
		showAIBudget: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("AI budget")).not.toBeInTheDocument();
	},
};

export const WithDisplayGroup: Story = {
	args: {
		groups: [{ ...MockGroup, name: "front-end" }],
	},
};

export const EmptyGroup: Story = {
	args: {
		groups: [],
		canCreateGroup: false,
	},
};

export const EmptyGroupWithPermission: Story = {
	args: {
		groups: [],
	},
};
