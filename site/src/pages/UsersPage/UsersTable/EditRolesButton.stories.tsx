import {
  MockOwnerRole,
  MockSiteRoles,
  MockUserAdminRole,
} from "testHelpers/entities";
import { EditRolesButton } from "./EditRolesButton";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof EditRolesButton> = {
  title: "pages/UsersPage/EditRolesButton",
  component: EditRolesButton,
  args: {
    isDefaultOpen: true,
  },
};

export default meta;
type Story = StoryObj<typeof EditRolesButton>;

export const Open: Story = {
  args: {
    roles: MockSiteRoles,
    selectedRoles: [MockUserAdminRole, MockOwnerRole],
  },
  parameters: {
    chromatic: { delay: 300 },
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
    roles: MockSiteRoles,
    selectedRoles: [MockUserAdminRole, MockOwnerRole],
    userLoginType: "password",
    oidcRoleSync: false,
  },
  parameters: {
    chromatic: { delay: 300 },
  },
};
