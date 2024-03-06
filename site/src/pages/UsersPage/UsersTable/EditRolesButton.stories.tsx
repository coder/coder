import type { Meta, StoryObj } from "@storybook/react";
import {
  MockOwnerRole,
  MockSiteRoles,
  MockUserAdminRole,
} from "testHelpers/entities";
import { EditRolesButton } from "./EditRolesButton";

const meta: Meta<typeof EditRolesButton> = {
  title: "pages/UsersPage/EditRolesButton",
  component: EditRolesButton,
  args: {
    isDefaultOpen: true,
  },
};

export default meta;
type Story = StoryObj<typeof EditRolesButton>;

const selectedRoleNames = new Set([MockUserAdminRole.name, MockOwnerRole.name]);

export const Open: Story = {
  args: {
    selectedRoleNames,
    roles: MockSiteRoles,
  },
  parameters: {
    chromatic: { delay: 300 },
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
    selectedRoleNames,
    roles: MockSiteRoles,
    userLoginType: "password",
    oidcRoleSync: false,
  },
  parameters: {
    chromatic: { delay: 300 },
  },
};
