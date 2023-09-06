import {
  MockUser,
  MockUser2,
  MockAssignableSiteRoles,
  MockAuthMethods,
} from "testHelpers/entities";
import { UsersTable } from "./UsersTable";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof UsersTable> = {
  title: "components/UsersTable",
  component: UsersTable,
  args: {
    isNonInitialPage: false,
    authMethods: MockAuthMethods,
  },
};

export default meta;
type Story = StoryObj<typeof UsersTable>;

export const Example: Story = {
  args: {
    users: [MockUser, MockUser2],
    roles: MockAssignableSiteRoles,
    canEditUsers: false,
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
        roles: [],
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
