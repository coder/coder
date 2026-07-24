import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	getGroupByIdQueryKey,
	getGroupMembersAISpendQueryKey,
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
import type {
	GroupAIBudget,
	GroupMemberAISpend,
	GroupMembersAISpend,
	ReducedUser,
	UserAISpendStatus,
} from "#/api/typesGenerated";
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

/** period_end is exclusive. */
const mockUserAISpend: UserAISpendStatus = {
	user_id: MockUserOwner.id,
	spend_limit_micros: 9_000_000_000,
	effective_group_id: MockGroupWithoutMembers.id,
	limit_source: "group",
	current_spend_micros: 1_345_000_000,
	period_start: "2026-06-01T00:00:00Z",
	period_end: "2026-07-01T00:00:00Z",
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

/** The members list loads but the spend fetch fails: budget cells fall back to an em dash. */
export const MembersSpendError: Story = {
	beforeEach: () => {
		spyOn(API, "getGroupMembersAISpend").mockRejectedValue(
			new Error("test members spend error"),
		);
	},
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({ users: [MockUserMember], count: 1 }),
			permissionsQuery({ canUpdateGroup: true }),
			{ key: meAISpendKey, data: mockUserAISpend },
			{ key: groupAIBudget(MockGroupWithoutMembers.id).queryKey, data: null },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Group members" });
		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserMember.id}`),
		).toHaveTextContent("\u2014");
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

const mockSpend: GroupMemberAISpend = {
	user_id: "",
	effective_group_id: MockGroupWithoutMembers.id,
	group_budget: { spend_limit_micros: 9_000_000_000, limit_source: "group" },
	group_spend_micros: 1_345_000_000,
};

const membersSpendQuery = (spends: readonly GroupMemberAISpend[]) => ({
	key: getGroupMembersAISpendQueryKey(
		MockGroupWithoutMembers.id,
		spends.map((spend) => spend.user_id),
	),
	data: {
		period_start: "2026-06-01T00:00:00Z",
		period_end: "2026-07-01T00:00:00Z",
		members: spends,
	} satisfies GroupMembersAISpend,
});

const mockGroupBudget: GroupAIBudget = {
	group_id: MockGroupWithoutMembers.id,
	spend_limit_micros: 7_000_000_000,
	created_at: "2026-06-01T00:00:00Z",
	updated_at: "2026-06-01T00:00:00Z",
};

export const WithMemberAIBudget: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [MockUserMember],
				count: 1,
			}),
			membersSpendQuery([
				{
					...mockSpend,
					user_id: MockUserMember.id,
					group_spend_micros: 3_235_000_000,
					group_budget: {
						spend_limit_micros: 7_000_000_000,
						limit_source: "group",
					},
				},
			]),
			permissionsQuery({ canUpdateGroup: true }),
			{ key: meAISpendKey, data: mockUserAISpend },
			{
				key: groupAIBudget(MockGroupWithoutMembers.id).queryKey,
				data: mockGroupBudget,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("AI budget")).toBeInTheDocument();
		await expect(await canvas.findByText("Budget group")).toBeInTheDocument();
		// Dates depend on the runner's timezone; match loosely.
		await expect(
			await canvas.findByText(/^AI budget period: \w+ \d+ - \w+ \d+, 2026$/),
		).toBeInTheDocument();

		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserMember.id}`),
		).toHaveTextContent("$3,235 USD");

		const body = within(document.body);
		await userEvent.click(
			within(canvas.getByText("AI budget")).getByRole("button", {
				name: "More info",
			}),
		);
		await expect(
			await body.findByText(
				/^Monthly AI spend for this user\. Resets .*The group's default limit is \$7,000 per member\.$/,
			),
		).toBeInTheDocument();
		await userEvent.click(
			within(canvas.getByText("Budget group")).getByRole("button", {
				name: "More info",
			}),
		);
		await expect(
			await body.findByText(
				/The group or individual budget currently responsible for this user's AI spend\./,
			),
		).toBeInTheDocument();
	},
};

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
		expect(canvas.queryByText("Budget group")).not.toBeInTheDocument();
		expect(canvas.queryByText(/AI budget period/)).not.toBeInTheDocument();
	},
};

export const AIBudgetActionDisabledForOtherGroup: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [MockUserOwner],
				count: 1,
			}),
			membersSpendQuery([
				{
					...mockSpend,
					user_id: MockUserOwner.id,
					effective_group_id: MockGroup2.id,
				},
			]),
			permissionsQuery({ canUpdateGroup: true }),
			{ key: meAISpendKey, data: mockUserAISpend },
			{ key: groupAIBudget(MockGroupWithoutMembers.id).queryKey, data: null },
			{
				key: getGroupByIdQueryKey(MockGroup2.id, { exclude_members: true }),
				data: MockGroup2,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		// Without a group default budget, the header note ends at the reset date.
		await userEvent.click(
			within(canvas.getByText("AI budget")).getByRole("button", {
				name: "More info",
			}),
		);
		await expect(
			await body.findByText(/^Monthly AI spend for this user\. Resets .*\.$/),
		).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");

		// The menu stays enabled while the governing group's name resolves.
		await canvas.findByText("developer");
		await userEvent.click(
			canvas.getAllByRole("button", { name: "Open menu" })[0],
		);
		const menuItem = await body.findByRole("menuitem", {
			name: "Manage AI budget",
		});
		await expect(menuItem).toHaveAttribute("aria-disabled", "true");
	},
};

/** The other-org effective group can't be resolved, so the action disables. */
export const WithMemberAIBudgetInAnotherOrg: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [MockUserOwner],
				count: 1,
			}),
			membersSpendQuery([
				{
					...mockSpend,
					user_id: MockUserOwner.id,
					effective_group_id: null,
					group_budget: null,
				},
			]),
			permissionsQuery({ canUpdateGroup: true }),
			{ key: meAISpendKey, data: mockUserAISpend },
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
		await userEvent.click(
			within(cell).getByRole("button", { name: "More info" }),
		);
		await expect(
			await body.findByText(/managed by a group in another organization/),
		).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");

		await userEvent.click(
			canvas.getAllByRole("button", { name: "Open menu" })[0],
		);
		const menuItem = await body.findByRole("menuitem", {
			name: "Manage AI budget",
		});
		await expect(menuItem).toHaveAttribute("aria-disabled", "true");
	},
};

export const OpenAIBudgetForCurrentGroupMember: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [MockUserOwner],
				count: 1,
			}),
			membersSpendQuery([{ ...mockSpend, user_id: MockUserOwner.id }]),
			permissionsQuery({ canUpdateGroup: true }),
			{ key: meAISpendKey, data: mockUserAISpend },
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
			await body.findByRole("menuitem", { name: "Manage AI budget" }),
		);
		await userEvent.click(await body.findByText("Override group budget"));
		await expect(
			await body.findByText("Front-End (default)"),
		).toBeInTheDocument();
	},
};

/** Unresolvable via getGroupById, standing in for another org's group. */
const unresolvedGroupId = "external-org-group";

/** Per-state details are covered by GroupMemberBudgetCells.stories. */

const showcaseMembers: ReducedUser[] = [
	{
		...MockUserMember,
		id: "member-none",
		username: "alice",
		name: "Alice Chen",
	},
	{
		...MockUserMember,
		id: "member-unlimited",
		username: "bob",
		name: "Bob Diaz",
	},
	{
		...MockUserMember,
		id: "member-elsewhere",
		username: "priya",
		name: "Priya Nair",
	},
	{
		...MockUserMember,
		id: "member-regular",
		username: "jordan",
		name: "Jordan Lee",
	},
	{
		...MockUserMember,
		id: "member-custom",
		username: "sam",
		name: "Sam Okafor",
		status: "dormant",
	},
	{
		...MockUserMember,
		id: "member-near",
		username: "morgan",
		name: "Morgan Ito",
	},
	{
		...MockUserMember,
		id: "member-over",
		username: "casey",
		name: "Casey Novak",
	},
	{
		...MockUserMember,
		id: "member-other-group",
		username: "riley",
		name: "Riley Park",
		status: "suspended",
	},
];

const showcaseSpends: GroupMemberAISpend[] = [
	{
		...mockSpend,
		user_id: "member-none",
		group_budget: { spend_limit_micros: 0, limit_source: "group" },
		group_spend_micros: 0,
		effective_group_id: MockGroupWithoutMembers.organization_id,
	},
	{
		...mockSpend,
		user_id: "member-unlimited",
		group_budget: null,
		group_spend_micros: 0,
		effective_group_id: MockGroupWithoutMembers.organization_id,
	},
	{
		...mockSpend,
		user_id: "member-elsewhere",
		group_spend_micros: 456_000_000,
		effective_group_id: unresolvedGroupId,
	},
	{
		...mockSpend,
		user_id: "member-regular",
		group_budget: { spend_limit_micros: 7_000_000_000, limit_source: "group" },
		group_spend_micros: 3_235_000_000,
	},
	{
		...mockSpend,
		user_id: "member-custom",
		group_budget: {
			spend_limit_micros: 9_000_000_000,
			limit_source: "user_override",
		},
		group_spend_micros: 7_175_000_000,
	},
	{
		...mockSpend,
		user_id: "member-near",
		group_budget: { spend_limit_micros: 7_000_000_000, limit_source: "group" },
		group_spend_micros: 6_735_000_000,
	},
	{
		...mockSpend,
		user_id: "member-over",
		group_budget: { spend_limit_micros: 7_000_000_000, limit_source: "group" },
		group_spend_micros: 7_200_000_000,
	},
	{
		...mockSpend,
		user_id: "member-other-group",
		group_spend_micros: 456_000_000,
		effective_group_id: MockGroup2.id,
	},
];

export const AIBudgetShowcase: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: showcaseMembers,
				count: 8,
			}),
			membersSpendQuery(showcaseSpends),
			permissionsQuery({ canUpdateGroup: true }),
			{ key: meAISpendKey, data: mockUserAISpend },
			{
				key: groupAIBudget(MockGroupWithoutMembers.id).queryKey,
				data: mockGroupBudget,
			},
			{
				key: getGroupByIdQueryKey(unresolvedGroupId, { exclude_members: true }),
				data: null,
			},
			{
				key: getGroupByIdQueryKey(MockGroup2.id, { exclude_members: true }),
				data: MockGroup2,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Group members" });

		// Every member renders their own joined spend cell.
		for (const member of showcaseMembers) {
			await expect(
				await canvas.findByTestId(`member-ai-budget-${member.id}`),
			).toBeInTheDocument();
		}

		const body = within(document.body);

		// Everyone (unset) must not disable the override action.
		await userEvent.click(
			canvas.getAllByRole("button", { name: "Open menu" })[0],
		);
		const manageItem = await body.findByRole("menuitem", {
			name: "Manage AI budget",
		});
		await expect(manageItem).not.toHaveAttribute("aria-disabled", "true");
		await userEvent.keyboard("{Escape}");

		// Another named group does disable it.
		const otherGroupMenu = await canvas.findAllByRole("button", {
			name: "Open menu",
		});
		await userEvent.click(otherGroupMenu[7]);
		const disabledItem = await body.findByRole("menuitem", {
			name: "Manage AI budget",
		});
		await expect(disabledItem).toHaveAttribute("aria-disabled", "true");
	},
};
