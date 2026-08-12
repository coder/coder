import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, within } from "storybook/test";
import {
	GROUP_MEMBER_AVATAR_LIMIT,
	getGroupMemberAvatarsQueryKey,
} from "#/api/queries/groups";
import { getDefaultFilterProps } from "#/components/Filter/storyHelpers";
import type { UsersFilter } from "#/components/Filter/UsersFilter";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	MockGroup,
	MockOrganization,
	MockPermissions,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { GroupsPageView, type GroupWithSpend } from "./GroupsPageView";

type FilterProps = ComponentProps<typeof UsersFilter>;

const meta: Meta<typeof GroupsPageView> = {
	title: "pages/OrganizationGroupsPage",
	component: GroupsPageView,
	args: {
		canCreateGroup: true,
		groupsEnabled: true,
		filterProps: getDefaultFilterProps<FilterProps>({
			values: {},
			menus: {},
		}),
		groupsQuery: {
			...mockSuccessResult,
			totalRecords: 1,
		} as UsePaginatedQueryResult,
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof GroupsPageView>;

const mockGroupWithSpend: GroupWithSpend = {
	...MockGroup,
	spend: undefined,
};

// AI-budget and pagination stories aren't about membership, so give their
// groups no members. Rows with a zero count skip the per-row avatar fetch.
const aiGroup = (id: string, name: string): GroupWithSpend => ({
	...mockGroupWithSpend,
	id,
	name,
	display_name: name,
	total_member_count: 0,
});

// Seeds the per-row member avatar preview query for a group so the row renders
// avatars deterministically without hitting the network.
const seedAvatars = (
	groupName: string,
	users: ReadonlyArray<typeof MockUserOwner>,
	totalCount: number,
) => ({
	key: getGroupMemberAvatarsQueryKey(
		MockOrganization.name,
		groupName,
		GROUP_MEMBER_AVATAR_LIMIT,
	),
	data: { users, count: totalCount },
});

export const Default: Story = {};

export const NotEnabled: Story = {
	args: {
		groups: [mockGroupWithSpend],
		groupsEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Learn about Premium" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
	},
};

export const NotEnabledWithoutLicenseAccess: Story = {
	args: {
		...NotEnabled.args,
		permissions: { ...MockPermissions, viewAllLicenses: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Learn about Premium" }),
		).not.toBeInTheDocument();
	},
};

export const WithGroups: Story = {
	args: {
		groups: [mockGroupWithSpend],
	},
	parameters: {
		queries: [seedAvatars(MockGroup.name, [MockUserOwner, MockUserMember], 2)],
	},
};

// A group with more members than fit in the preview: the row shows the capped
// avatars plus a "+N" badge derived from total_member_count.
export const WithMemberAvatars: Story = {
	args: {
		groups: [
			{
				...mockGroupWithSpend,
				id: "with-members",
				name: "with-members",
				display_name: "With members",
				total_member_count: 8,
			},
		],
	},
	parameters: {
		queries: [
			seedAvatars(
				"with-members",
				Array.from({ length: GROUP_MEMBER_AVATAR_LIMIT }, (_, i) => ({
					...MockUserOwner,
					id: `preview-${i}`,
					username: `member-${i}`,
				})),
				8,
			),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("+3")).toBeInTheDocument();
		await expect(canvas.getByText("8 members")).toBeInTheDocument();
	},
};

const totalRecords = 15;
const totalPages = 3;
const limit = totalRecords / totalPages;

// Multiple pages of results with the search field in use.
export const WithSearchAndPagination: Story = {
	args: {
		groups: Array.from({ length: limit }, (_, i) =>
			aiGroup(`group-${i}`, `Group ${i}`),
		),
		filterProps: getDefaultFilterProps<FilterProps>({
			query: "group",
			values: {},
			menus: {},
			used: true,
		}),
		groupsQuery: {
			...mockSuccessResult,
			totalRecords,
			totalPages,
			limit,
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
			{
				...aiGroup("ai-unlimited", "Unlimited"),
				spend: {
					group_id: "ai-unlimited",
					current_spend_micros: 25_492_000_000,
					spend_limit_micros: null,
					total_spend_limit_micros: null,
				},
			},
			{
				...aiGroup("ai-under", "Under budget"),
				spend: {
					group_id: "ai-under",
					current_spend_micros: 10_000_000,
					spend_limit_micros: 25_000_000,
					total_spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-warning", "Near limit"),
				spend: {
					group_id: "ai-warning",
					current_spend_micros: 46_000_000,
					spend_limit_micros: 25_000_000,
					total_spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-at-limit", "At limit"),
				spend: {
					group_id: "ai-at-limit",
					current_spend_micros: 50_000_000,
					spend_limit_micros: 25_000_000,
					total_spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-over", "Over budget"),
				spend: {
					group_id: "ai-over",
					current_spend_micros: 75_000_000,
					spend_limit_micros: 25_000_000,
					total_spend_limit_micros: 50_000_000,
				},
			},
			{
				...aiGroup("ai-zero-budget", "Zero budget"),
				spend: {
					group_id: "ai-zero-budget",
					current_spend_micros: 5_000_000,
					spend_limit_micros: 0,
					total_spend_limit_micros: 0,
				},
			},
			{
				...aiGroup("ai-zero-both", "Zero spend and budget"),
				spend: {
					group_id: "ai-zero-both",
					current_spend_micros: 0,
					spend_limit_micros: 0,
					total_spend_limit_micros: 0,
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
		showAIBudget: true,
		groupsQuery: mockInitialRenderResult as UsePaginatedQueryResult,
	},
};

// Spend still loading: every AI spend cell falls back to an em dash.
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
		showAIBudget: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByTestId("group-ai-unavailable"),
		).toHaveTextContent("\u2014");
	},
};

// AI Bridge hidden: no AI spend column.
export const WithoutAIBudgetColumn: Story = {
	args: {
		groups: [aiGroup("ai-hidden", "No AI column")],
		showAIBudget: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("AI spend")).not.toBeInTheDocument();
	},
};

export const WithDisplayGroup: Story = {
	args: {
		groups: [{ ...mockGroupWithSpend, name: "front-end" }],
	},
	parameters: {
		queries: [seedAvatars("front-end", [MockUserOwner, MockUserMember], 2)],
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

// A search that matches nothing shows filter-aware copy, not the
// create-first-group empty state.
export const NoSearchResults: Story = {
	args: {
		groups: [],
		filterProps: getDefaultFilterProps<FilterProps>({
			query: "nomatch",
			values: {},
			menus: {},
			used: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("No groups match your search"),
		).toBeInTheDocument();
		expect(canvas.queryByText("No groups yet")).not.toBeInTheDocument();
	},
};
