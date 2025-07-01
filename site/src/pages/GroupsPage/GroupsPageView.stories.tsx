import type { Meta, StoryObj } from "@storybook/react";
import { MockGroup } from "testHelpers/entities";
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
