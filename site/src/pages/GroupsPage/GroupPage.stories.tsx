import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationMember,
	MockOrganizationMember2,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { getGroupQueryKey, groupPermissionsKey } from "api/queries/groups";
import { organizationMembersKey } from "api/queries/organizations";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import GroupPage from "./GroupPage";

const meta: Meta<typeof GroupPage> = {
	title: "pages/OrganizationGroupsPage/GroupPage",
	component: GroupPage,
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					organization: MockDefaultOrganization.name,
					groupName: MockGroup.name,
				},
			},
			routing: { path: "/organizations/:organization/groups/:groupName" },
		}),
	},
};

const groupQuery = (data: unknown) => ({
	key: getGroupQueryKey(MockDefaultOrganization.name, MockGroup.name),
	data,
});

const permissionsQuery = (data: unknown, id?: string) => ({
	key: groupPermissionsKey(id ?? MockGroup.id),
	data,
});

const membersQuery = (data: unknown) => ({
	key: organizationMembersKey(MockDefaultOrganization.id),
	data,
});

export default meta;
type Story = StoryObj<typeof GroupPage>;

export const LoadingGroup: Story = {
	parameters: {
		queries: [groupQuery(null), permissionsQuery({})],
	},
};

export const GroupError: Story = {
	beforeEach: () => {
		spyOn(API, "getGroup").mockRejectedValue(new Error("test group error"));
		spyOn(API, "checkAuthorization").mockResolvedValue({});
	},
};

export const LoadingPermissions: Story = {
	parameters: {
		queries: [groupQuery(MockGroup), permissionsQuery(null)],
	},
};

export const NoUpdatePermission: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroup),
			permissionsQuery({ canUpdateGroup: false }),
		],
	},
};

export const EveryoneGroup: Story = {
	parameters: {
		queries: [
			groupQuery({
				...MockGroup,
				// The everyone group has the same ID as the organization.
				id: MockDefaultOrganization.id,
			}),
			permissionsQuery({ canUpdateGroup: true }, MockDefaultOrganization.id),
		],
	},
};

export const MembersError: Story = {
	beforeEach() {
		spyOn(API, "getGroup").mockResolvedValue(MockGroup);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			canUpdateGroup: true,
		});
		spyOn(API, "getOrganizationPaginatedMembers").mockRejectedValue(
			new Error("test members error"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const combobox = await canvas.findByRole("combobox");
		await userEvent.click(combobox);
	},
};

export const NoMembers: Story = {
	parameters: {
		queries: [
			groupQuery({
				...MockGroup,
				members: [],
			}),
			permissionsQuery({ canUpdateGroup: true }),
			membersQuery([]),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const combobox = await canvas.findByRole("combobox");
		await userEvent.click(combobox);
	},
};

export const FiltersByMembers: Story = {
	parameters: {
		queries: [
			groupQuery(MockGroup),
			permissionsQuery({ canUpdateGroup: true }),
			membersQuery([MockOrganizationMember, MockOrganizationMember2]),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Open" }));
	},
};
