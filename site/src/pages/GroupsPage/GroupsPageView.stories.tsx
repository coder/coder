import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Group } from "#/api/typesGenerated";
import {
	MockGroup,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import type { GroupBudgetInfo } from "./GroupsPageView";
import { GroupsPageView } from "./GroupsPageView";

const meta: Meta<typeof GroupsPageView> = {
	title: "pages/OrganizationGroupsPage",
	component: GroupsPageView,
};

export default meta;
type Story = StoryObj<typeof GroupsPageView>;

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

// Mockup groups to match the cost controls presentation screenshot.
const mockDevopsGroup: Group = {
	id: "group-devops",
	name: "Devops",
	display_name: "Devops",
	avatar_url: "",
	organization_id: "org-1",
	organization_name: "coder",
	organization_display_name: "Coder",
	members: [
		MockUserOwner,
		MockUserMember,
		{ ...MockUserOwner, username: "mw", avatar_url: "" },
		{ ...MockUserMember, username: "ar", avatar_url: "" },
		{ ...MockUserOwner, username: "dev5", avatar_url: "" },
	],
	quota_allowance: 0,
	source: "user",
	total_member_count: 5,
};

const mockSomeGroup: Group = {
	id: "group-some",
	name: "Some-Group",
	display_name: "Some-Group",
	avatar_url: "",
	organization_id: "org-1",
	organization_name: "coder",
	organization_display_name: "Coder",
	members: [
		MockUserOwner,
		MockUserMember,
		{ ...MockUserOwner, username: "mw", avatar_url: "" },
		{ ...MockUserMember, username: "cw", avatar_url: "" },
	],
	quota_allowance: 0,
	source: "user",
	total_member_count: 4,
};

const mockCellText1: Group = {
	id: "group-cell-1",
	name: "Cell text",
	display_name: "Cell text",
	avatar_url: "",
	organization_id: "org-1",
	organization_name: "coder",
	organization_display_name: "Coder",
	members: [
		MockUserOwner,
		MockUserMember,
		{ ...MockUserOwner, username: "mw", avatar_url: "" },
	],
	quota_allowance: 0,
	source: "user",
	total_member_count: 3,
};

const mockCellText2: Group = {
	id: "group-cell-2",
	name: "Cell text",
	display_name: "Cell text",
	avatar_url: "",
	organization_id: "org-1",
	organization_name: "coder",
	organization_display_name: "Coder",
	members: [
		MockUserOwner,
		MockUserMember,
		{ ...MockUserOwner, username: "mw", avatar_url: "" },
		{ ...MockUserMember, username: "cw", avatar_url: "" },
	],
	quota_allowance: 0,
	source: "user",
	total_member_count: 4,
};

const costControlsGroups: Group[] = [
	mockDevopsGroup,
	mockSomeGroup,
	mockCellText1,
	mockCellText2,
];

const costControlsBudgets: Record<string, GroupBudgetInfo> = {
	"group-devops": { spentUSD: 25492, limitUSD: null, aiSeats: 2 },
	"group-some": { spentUSD: 110345, limitUSD: 127000, aiSeats: 27 },
	"group-cell-1": { spentUSD: 32211, limitUSD: 50000, aiSeats: 0 },
	"group-cell-2": { spentUSD: 174978, limitUSD: 175000, aiSeats: 36 },
};

export const CostControlsMockup: Story = {
	args: {
		groups: costControlsGroups,
		canCreateGroup: true,
		groupsEnabled: true,
		budgets: costControlsBudgets,
	},
};
