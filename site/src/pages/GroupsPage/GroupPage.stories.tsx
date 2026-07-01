import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import {
	API,
	type GroupMemberAICostControl,
	type GroupMemberWithAICostControl,
	type UserAISpend,
} from "#/api/api";
import {
	getGroupByIdQueryKey,
	getGroupMembersQueryKey,
	getGroupQueryKey,
	getGroupsForUserQueryKey,
	groupAIBudget,
	groupPermissionsKey,
} from "#/api/queries/groups";
import { organizationMembersKey } from "#/api/queries/organizations";
import {
	getUserAIBudgetOverrideQueryKey,
	meAISpendKey,
} from "#/api/queries/users";
import type { ReducedUser } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockGroup,
	MockGroup2,
	MockGroupWithoutMembers,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import GroupMembersPage from "./GroupMembersPage";
import GroupPage from "./GroupPage";

const meta: Meta<typeof GroupPage> = {
	title: "pages/OrganizationGroupsPage/GroupPage",
	component: GroupPage,
	decorators: [withDashboardProvider],
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					organization: MockDefaultOrganization.name,
					groupName: MockGroupWithoutMembers.name,
				},
			},
			routing: reactRouterOutlet(
				{ path: "/organizations/:organization/groups/:groupName" },
				<GroupMembersPage />,
			),
		}),
	},
};

const groupQuery = (data: unknown) => ({
	key: getGroupQueryKey(
		MockDefaultOrganization.name,
		MockGroupWithoutMembers.name,
		{
			exclude_members: true,
		},
	),
	data,
});

const groupMembersQuery = (data: unknown) => ({
	key: getGroupMembersQueryKey(
		MockDefaultOrganization.name,
		MockGroupWithoutMembers.name,
		{
			limit: 25,
			offset: 0,
			q: "",
		},
	),
	data,
});

const permissionsQuery = (data: unknown, id?: string) => ({
	key: groupPermissionsKey(id ?? MockGroupWithoutMembers.id),
	data,
});

const membersQuery = (data: unknown) => ({
	key: organizationMembersKey(MockDefaultOrganization.id, {
		limit: 25,
		q: "",
	}),
	data,
});

// Drives the period label and reset date; period_end is exclusive.
const aiSpendQuery = {
	key: meAISpendKey,
	data: {
		user_id: MockUserOwner.id,
		spend_limit_micros: 9_000_000_000,
		effective_group_id: MockGroupWithoutMembers.id,
		limit_source: "group",
		current_spend_micros: 1_345_000_000,
		period_start: "2026-06-01T00:00:00Z",
		period_end: "2026-07-01T00:00:00Z",
	} satisfies UserAISpend,
};

export default meta;
type Story = StoryObj<typeof GroupPage>;

export const LoadingGroup: Story = {
	parameters: {
		queries: [groupQuery(null), groupMembersQuery(null), permissionsQuery({})],
	},
};

export const LoadingGroupMembers: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery(null),
			permissionsQuery({}),
		],
	},
};

export const GroupError: Story = {
	beforeEach: () => {
		spyOn(API, "getGroup").mockRejectedValue(new Error("test group error"));
		spyOn(API, "getGroupMembers").mockResolvedValue({
			users: [],
			count: 0,
		});
		spyOn(API, "checkAuthorization").mockResolvedValue({});
	},
};

export const GroupMembersError: Story = {
	beforeEach: () => {
		spyOn(API, "getGroup").mockResolvedValue(MockGroupWithoutMembers);
		spyOn(API, "getGroupMembers").mockRejectedValue(
			new Error("test group members error"),
		);
		spyOn(API, "checkAuthorization").mockResolvedValue({});
	},
};

export const LoadingPermissions: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
			permissionsQuery(null),
		],
	},
};

export const NoUpdatePermission: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
			permissionsQuery({ canUpdateGroup: false }),
		],
	},
};

export const EveryoneGroup: Story = {
	parameters: {
		queries: [
			groupQuery({
				...MockGroupWithoutMembers,
				// The everyone group has the same ID as the organization.
				id: MockDefaultOrganization.id,
			}),
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
			permissionsQuery({ canUpdateGroup: true }, MockDefaultOrganization.id),
		],
	},
};

export const MembersError: Story = {
	beforeEach() {
		spyOn(API, "getGroup").mockResolvedValue(MockGroupWithoutMembers);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			canUpdateGroup: true,
		});
		spyOn(API, "getOrganizationPaginatedMembers").mockRejectedValue(
			new Error("test members error"),
		);
	},
	parameters: {
		queries: [
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Add users" }),
		);
	},
};

export const NoMembers: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({ users: [], count: 0 }),
			permissionsQuery({ canUpdateGroup: true }),
			membersQuery({ members: [] }),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Add users" }),
		);
	},
};

export const FiltersByMembers: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			membersQuery({
				members: [MockOrganizationMember, MockOrganizationMember2],
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Add users" }),
		);
	},
};

// Member row with inline AI cost control; defaults to the page's group.
const memberWithSpend = (
	user: ReducedUser,
	overrides: Partial<GroupMemberAICostControl> = {},
): GroupMemberWithAICostControl => ({
	...user,
	ai_cost_control: {
		current_spend_micros: 1_345_000_000,
		spend_limit_micros: 9_000_000_000,
		effective_group_id: MockGroupWithoutMembers.id,
		limit_source: "override",
		...overrides,
	},
});

const memberWithoutSpend: GroupMemberWithAICostControl = {
	...MockUserMember,
	id: "no-spend-user",
	username: "no-spend",
};

export const WithMemberAIBudget: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [
					// Override source, no limit.
					memberWithSpend(MockUserOwner, { spend_limit_micros: null }),
					// Group source, finite limit.
					memberWithSpend(MockUserMember, {
						current_spend_micros: 5_492_000_000,
						spend_limit_micros: 7_000_000_000,
						limit_source: "group",
					}),
					// No cost control exercises the missing-spend "-" fallback.
					memberWithoutSpend,
				],
				count: 3,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("AI budget")).toBeInTheDocument();
		await expect(await canvas.findByText("Budget source")).toBeInTheDocument();
		await expect(
			await canvas.findByText("AI budget period June 1 - June 30, 2026"),
		).toBeInTheDocument();
		// Override source, no limit.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserOwner.id}`),
		).toHaveTextContent("$1,345 / Unlimited USD");
		await expect(await canvas.findByText("Individual")).toBeInTheDocument();
		// Group source, finite limit.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserMember.id}`),
		).toHaveTextContent("$5,492 / $7,000 USD");
		await expect(await canvas.findByText("Group")).toBeInTheDocument();
		// No spend reported for this member.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${memberWithoutSpend.id}`),
		).toHaveTextContent("\u2014");

		// Column header tooltips.
		const body = within(document.body);
		await userEvent.click(
			within(canvas.getByText("AI budget")).getByRole("button", {
				name: "More info",
			}),
		);
		await expect(
			await body.findByText(/^Monthly AI API cost for this user\. Resets /),
		).toBeInTheDocument();
		await userEvent.click(
			within(canvas.getByText("Budget source")).getByRole("button", {
				name: "More info",
			}),
		);
		await expect(
			await body.findByText(
				/Users with group type will inherit the group budget allowance\./,
			),
		).toBeInTheDocument();
		await expect(
			await body.findByText(
				/Users with individual type have a budget override\./,
			),
		).toBeInTheDocument();
	},
};

// Budget governed by another group: both columns em-dash and a tooltip names
// the governing group.
export const WithMemberAIBudgetFromAnotherGroup: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [
					memberWithSpend(MockUserOwner, {
						effective_group_id: MockGroup2.id,
						limit_source: "group",
					}),
				],
				count: 1,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery,
			{
				key: getGroupByIdQueryKey(MockGroup2.id, { exclude_members: true }),
				data: MockGroup2,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const cell = await canvas.findByTestId(
			`member-ai-budget-${MockUserOwner.id}`,
		);
		await expect(cell).toHaveTextContent("\u2014");
		await expect(cell).not.toHaveTextContent("USD");
		await expect(canvas.queryByText("Group")).not.toBeInTheDocument();
		// The info tooltip names the group that governs the budget.
		await userEvent.click(
			within(cell).getByRole("button", { name: "More info" }),
		);
		// The emphasized group name splits the sentence across nodes.
		await expect(await body.findByText(/attributed to the/)).toHaveTextContent(
			"This user's budget is attributed to the developer group.",
		);
	},
};

// AI Bridge hidden: neither the AI budget nor the Budget source column renders.
export const WithoutMemberAIBudgetColumn: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({ users: [MockUserOwner], count: 1 }),
			permissionsQuery({ canUpdateGroup: true }),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Group members" });
		expect(canvas.queryByText("AI budget")).not.toBeInTheDocument();
		expect(canvas.queryByText("Budget source")).not.toBeInTheDocument();
		expect(canvas.queryByText(/AI budget period/)).not.toBeInTheDocument();
	},
};

// Budget from another group: the override action is disabled here.
export const AIBudgetActionDisabledForOtherGroup: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [
					memberWithSpend(MockUserOwner, {
						effective_group_id: MockGroup2.id,
					}),
				],
				count: 1,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery,
			{
				key: getGroupByIdQueryKey(MockGroup2.id, { exclude_members: true }),
				data: MockGroup2,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getAllByRole("button", { name: "Open menu" })[0],
		);
		const menuItem = await body.findByRole("menuitem", { name: "AI Budget" });
		await expect(menuItem).toHaveAttribute("aria-disabled", "true");
	},
};

// effective_group_id null: both columns em-dash; dialog marks no "(default)".
export const WithMemberAIBudgetWithoutEffectiveGroup: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [
					memberWithSpend(MockUserOwner, {
						effective_group_id: null,
						limit_source: "group",
					}),
				],
				count: 1,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery,
			{ key: getUserAIBudgetOverrideQueryKey(MockUserOwner.id), data: null },
			{
				key: getGroupsForUserQueryKey(
					MockUserOwner.id,
					MockGroupWithoutMembers.organization_id,
				),
				data: [MockGroup2],
			},
			{ key: groupAIBudget(MockGroupWithoutMembers.id).queryKey, data: null },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		const cell = await canvas.findByTestId(
			`member-ai-budget-${MockUserOwner.id}`,
		);
		await expect(cell).toHaveTextContent("\u2014");
		await expect(cell).not.toHaveTextContent("USD");
		// Generic fallback when no group name resolves.
		await userEvent.click(
			within(cell).getByRole("button", { name: "More info" }),
		);
		await expect(
			await body.findByText(/attributed to another group/),
		).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");

		await userEvent.click(
			canvas.getAllByRole("button", { name: "Open menu" })[0],
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "AI Budget" }),
		);
		await userEvent.click(await body.findByText("Override group budget"));
		await expect(
			await body.findByText("Custom monthly budget"),
		).toBeInTheDocument();
		await expect(body.queryByText(/\(default\)/)).not.toBeInTheDocument();
	},
};

// Governed by the viewed group: the dialog marks it "(default)".
export const OpenAIBudgetForCurrentGroupMember: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [
					memberWithSpend(MockUserOwner, {
						effective_group_id: MockGroupWithoutMembers.id,
						limit_source: "group",
					}),
				],
				count: 1,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery,
			{ key: getUserAIBudgetOverrideQueryKey(MockUserOwner.id), data: null },
			{
				key: getGroupsForUserQueryKey(
					MockUserOwner.id,
					MockGroupWithoutMembers.organization_id,
				),
				data: [MockGroup2],
			},
			{ key: groupAIBudget(MockGroupWithoutMembers.id).queryKey, data: null },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getAllByRole("button", { name: "Open menu" })[0],
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "AI Budget" }),
		);
		await userEvent.click(await body.findByText("Override group budget"));
		await expect(
			await body.findByText("Front-End (default)"),
		).toBeInTheDocument();
	},
};
