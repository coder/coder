import type { Meta, StoryObj } from "@storybook/react";
import {
  MockUser,
  MockUser2,
  MockAssignableSiteRoles,
  MockAuthMethodsPasswordOnly,
  MockGroup,
  MockUserAdminRole,
  MockTemplateAdminRole,
  MockMemberRole,
  MockAuditorRole,
} from "testHelpers/entities";
import { UsersTable } from "./UsersTable";

const mockGroupsByUserId = new Map([
  [MockUser.id, [MockGroup]],
  [MockUser2.id, [MockGroup]],
]);

const meta: Meta<typeof UsersTable> = {
  title: "pages/UsersPage/UsersTable",
  component: UsersTable,
  args: {
    isNonInitialPage: false,
    authMethods: MockAuthMethodsPasswordOnly,
  },
};

export default meta;
type Story = StoryObj<typeof UsersTable>;

export const Example: Story = {
  args: {
    users: [MockUser, MockUser2],
    roles: MockAssignableSiteRoles,
    canEditUsers: false,
    groupsByUserId: mockGroupsByUserId,
  },
};

export const Editable: Story = {
  args: {
    users: [
      MockUser,
      MockUser2,
      {
        ...MockUser,
        username: "John Doe",
        email: "john.doe@coder.com",
        roles: [
          MockUserAdminRole,
          MockTemplateAdminRole,
          MockMemberRole,
          MockAuditorRole,
        ],
        status: "dormant",
      },
      {
        ...MockUser,
        username: "Roger Moore",
        email: "roger.moore@coder.com",
        roles: [],
        status: "suspended",
      },
      {
        ...MockUser,
        username: "OIDC User",
        email: "oidc.user@coder.com",
        roles: [],
        status: "active",
        login_type: "oidc",
      },
    ],
    roles: MockAssignableSiteRoles,
    canEditUsers: true,
    canViewActivity: true,
    groupsByUserId: mockGroupsByUserId,
  },
};

export const Empty: Story = {
  args: {
    users: [],
    roles: MockAssignableSiteRoles,
  },
};

export const Loading: Story = {
  args: {
    users: [],
    roles: MockAssignableSiteRoles,
    isLoading: true,
  },
  parameters: {
    chromatic: { pauseAnimationAtEnd: true },
  },
};
