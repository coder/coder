import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import {
  MockOwnerRole,
  MockSiteRoles,
  MockUserAdminRole,
} from "testHelpers/entities";
import { withDesktopViewport } from "testHelpers/storybook";
import { EditRolesButton } from "./EditRolesButton";

const meta: Meta<typeof EditRolesButton> = {
  title: "pages/UsersPage/EditRolesButton",
  component: EditRolesButton,
  args: {
    selectedRoleNames: new Set([MockUserAdminRole.name, MockOwnerRole.name]),
    roles: MockSiteRoles,
  },
  decorators: [withDesktopViewport],
};

export default meta;
type Story = StoryObj<typeof EditRolesButton>;

export const Closed: Story = {};

export const Open: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button"));
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
    userLoginType: "password",
    oidcRoleSync: false,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button"));
  },
};
