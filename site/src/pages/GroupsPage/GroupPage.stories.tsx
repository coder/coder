import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API, type GroupMemberAISpend } from "#/api/api";
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
import { getUserAIBudgetOverrideQueryKey } from "#/api/queries/users";
import type { ReducedUser, UserAIBudgetOverride } from "#/api/typesGenerated";
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

const mockOwnerOverride: UserAIBudgetOverride = {
	user_id: MockUserOwner.id,
	group_id: MockGroup2.id,
	spend_limit_micros: 12_000_000_000,
	created_at: "2026-01-01T00:00:00Z",
	updated_at: "2026-01-01T00:00:00Z",
};

// Defaults to an override charged to the page's group; pass overrides per case.
const memberSpend = (
	userId: string,
	overrides: Partial<GroupMemberAISpend> = {},
): GroupMemberAISpend => ({
	user_id: userId,
	current_spend_micros: 1_345_000_000,
	spend_limit_micros: 9_000_000_000,
	effective_group_id: MockGroupWithoutMembers.id,
	limit_source: "override",
	...overrides,
});

const memberWithoutSpend: ReducedUser = {
	...MockUserMember,
	id: "no-spend-user",
	username: "no-spend",
};

const aiSpendQuery = (data: GroupMemberAISpend[]) => ({
	key: getGroupMembersAISpendQueryKey(MockGroupWithoutMembers.id),
	data,
});

export const WithMemberAISpend: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: [...MockGroup.members, memberWithoutSpend],
				count: MockGroup.members.length + 1,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			// memberWithoutSpend is omitted to exercise the missing-spend "-" fallback.
			aiSpendQuery([
				memberSpend(MockUserOwner.id, { spend_limit_micros: null }),
				memberSpend(MockUserMember.id, {
					current_spend_micros: 5_492_000_000,
					spend_limit_micros: 7_000_000_000,
					limit_source: "group",
				}),
			]),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("AI budget")).toBeInTheDocument();
		await expect(await canvas.findByText("Budget type")).toBeInTheDocument();
		// Override source, no limit.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserOwner.id}`),
		).toHaveTextContent("$1,345 / unlimited USD");
		await expect(await canvas.findByText("Individual")).toBeInTheDocument();
		// Group source, finite limit.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserMember.id}`),
		).toHaveTextContent("$5,492 / $7,000 USD");
		await expect(await canvas.findByText("Group")).toBeInTheDocument();
		// No spend reported for this member.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${memberWithoutSpend.id}`),
		).toHaveTextContent("-");
	},
};

export const WithMemberAISpendLoading: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
			permissionsQuery({ canUpdateGroup: true }),
		],
	},
	beforeEach: () => {
		spyOn(API, "getGroupMembersAISpend").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("AI budget")).toBeInTheDocument();
		// Skeletons render while spend loads, so no amount is shown yet.
		await expect(
			await canvas.findByTestId(`member-ai-budget-${MockUserOwner.id}`),
		).not.toHaveTextContent("$");
	},
};

export const WithMemberAISpendEffectiveGroupMismatch: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({ users: [MockUserOwner], count: 1 }),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery([
				memberSpend(MockUserOwner.id, {
					effective_group_id: "other-group-id",
					limit_source: "group",
				}),
			]),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Off the effective group: spend shows without a limit, and no budget type.
		const cell = await canvas.findByTestId(
			`member-ai-budget-${MockUserOwner.id}`,
		);
		await expect(cell).toHaveTextContent("$1,345");
		await expect(cell).not.toHaveTextContent("USD");
		await expect(canvas.queryByText("Group")).not.toBeInTheDocument();
	},
};

export const OpenAIBudgetFromMemberMenu: Story = {
	parameters: {
		features: ["aibridge"],
		experiments: ["ai-gateway-cost-control"],
		queries: [
			groupQuery(MockGroupWithoutMembers),
			groupMembersQuery({
				users: MockGroup.members,
				count: MockGroup.members.length,
			}),
			permissionsQuery({ canUpdateGroup: true }),
			aiSpendQuery([
				memberSpend(MockUserOwner.id, { effective_group_id: MockGroup2.id }),
			]),
			{
				key: getGroupByIdQueryKey(MockGroup2.id, { exclude_members: true }),
				data: MockGroup2,
			},
			{
				key: getUserAIBudgetOverrideQueryKey(MockUserOwner.id),
				data: mockOwnerOverride,
			},
			{
				key: getGroupsForUserQueryKey(
					MockUserOwner.id,
					MockGroupWithoutMembers.organization_id,
				),
				data: [MockGroup],
			},
			{
				key: groupAIBudget(MockGroup2.id).queryKey,
				data: null,
			},
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
		await expect(
			await body.findByText("Custom monthly budget"),
		).toBeInTheDocument();
		await expect(await body.findByText("developer")).toBeInTheDocument();
	},
};
