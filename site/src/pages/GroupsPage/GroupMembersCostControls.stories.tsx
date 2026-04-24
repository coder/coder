import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReducedUser } from "#/api/typesGenerated";
import {
	GroupMembersMockupTable,
	type MemberWithBudget,
} from "./GroupMembersPage";

const meta: Meta<typeof GroupMembersMockupTable> = {
	title: "pages/OrganizationGroupsPage/GroupMembersCostControls",
	component: GroupMembersMockupTable,
};

export default meta;
type Story = StoryObj<typeof GroupMembersMockupTable>;

const fiveDaysAgo = new Date(
	Date.now() - 5 * 24 * 60 * 60 * 1000,
).toISOString();
const twelveDaysAgo = new Date(
	Date.now() - 12 * 24 * 60 * 60 * 1000,
).toISOString();
const now = new Date().toISOString();

const mockMemberAdmin: ReducedUser = {
	id: "member-admin",
	username: "Username",
	name: "Username",
	email: "email address",
	avatar_url: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
	created_at: "",
	updated_at: "",
	last_seen_at: fiveDaysAgo,
	status: "active",
	login_type: "password",
};

const mockMemberServiceAccount: ReducedUser = {
	id: "member-service",
	username: "Team-Pineapple",
	name: "Team-Pineapple",
	email: "service@coder.com",
	avatar_url: "",
	created_at: "",
	updated_at: "",
	last_seen_at: now,
	status: "active",
	login_type: "token",
	is_service_account: true,
};

const mockMemberRegular: ReducedUser = {
	id: "member-regular",
	username: "username",
	name: "username",
	email: "email address",
	avatar_url: "",
	created_at: "",
	updated_at: "",
	last_seen_at: twelveDaysAgo,
	status: "dormant",
	login_type: "password",
};

const mockMemberOverBudget: ReducedUser = {
	id: "member-over",
	username: "user-name",
	name: "user-name",
	email: "email address",
	avatar_url: "",
	created_at: "",
	updated_at: "",
	last_seen_at: now,
	status: "active",
	login_type: "password",
};

const membersWithBudget: MemberWithBudget[] = [
	{
		member: mockMemberAdmin,
		roles: [{ name: "Admin" }, { name: "Elite AI" }],
	},
	{
		member: mockMemberServiceAccount,
		roles: [{ name: "Service account" }],
		budget: {
			spentUSD: 5492,
			limitUSD: 7000,
			budgetType: "group",
			attributedGroup: "Pineapple",
			customMonthlyBudget: 12000,
			breakdown: [
				{
					groupName: "Devops",
					label: "custom",
					amountUSD: 312,
				},
				{
					groupName: "Devops",
					label: "group",
					amountUSD: 4128,
				},
				{
					groupName: "Flaming devs",
					label: "group",
					amountUSD: 1052,
				},
			],
		},
	},
	{
		member: mockMemberRegular,
		roles: [{ name: "Member" }, { name: "AI" }],
		budget: {
			spentUSD: 7235,
			limitUSD: 9000,
			budgetType: "individual",
		},
	},
	{
		member: mockMemberOverBudget,
		roles: [{ name: "Member" }],
		budget: {
			spentUSD: 6978,
			limitUSD: 7000,
			budgetType: "group",
			attributedGroup: "Pineapple",
		},
	},
];

export const CostControlsMockup: Story = {
	args: {
		membersWithBudget,
	},
};
