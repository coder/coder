import type { Meta, StoryObj } from "@storybook/react";
import { MockAuthMethodsAll, MockUser } from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withGlobalSnackbar,
} from "testHelpers/storybook";
import UsersPage from "./UsersPage";
import { groupsQueryKey } from "api/queries/groups";
import { MockGroups } from "pages/UsersPage/storybookData/groups";
import { authMethodsQueryKey, usersKey } from "api/queries/users";
import { rolesQueryKey } from "api/queries/roles";
import { MockRoles } from "pages/UsersPage/storybookData/roles";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { deploymentConfigQueryKey } from "api/queries/deployment";

const meta: Meta<typeof UsersPage> = {
	title: "pages/UsersPage",
	component: UsersPage,
	parameters: {
		queries: [
			// TODO: Investigate the reason behind the UI making two query calls:
			//       1. One with offset 0
			//       2. Another with offset 25
			{
				key: usersKey({ limit: 25, offset: 25, q: "" }),
				data: {
					users: MockUsers,
					count: 60,
				},
			},
			{
				key: usersKey({ limit: 25, offset: 0, q: "" }),
				data: {
					users: MockUsers,
					count: 60,
				},
			},
			{
				key: groupsQueryKey,
				data: MockGroups,
			},
			{
				key: authMethodsQueryKey,
				data: MockAuthMethodsAll,
			},
			{
				key: rolesQueryKey,
				data: MockRoles,
			},
			{
				key: deploymentConfigQueryKey,
				data: {
					config: {
						oidc: {
							user_role_field: "role",
						},
					},
					options: [],
				},
			},
		],
		user: MockUser,
		permissions: {
			createUser: true,
			updateUsers: true,
			viewDeploymentValues: true,
		},
	},
	decorators: [withGlobalSnackbar, withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof UsersPage>;

export const Loaded: Story = {};
