import type { Meta, StoryObj } from "@storybook/react";
import { MockUser, MockUser2 } from "../../../testHelpers/entities";
import { NavbarView } from "./NavbarView";

const meta: Meta<typeof NavbarView> = {
  title: "components/NavbarView",
  component: NavbarView,
  args: {
    user: MockUser,
  },
};

export default meta;
type Story = StoryObj<typeof NavbarView>;

export const ForAdmin: Story = {};

export const ForMember: Story = {
  args: {
    user: MockUser2,
    canViewAuditLog: false,
    canViewDeployment: false,
    canViewAllUsers: false,
  },
};

export const SmallViewport: Story = {
  parameters: {
    viewport: {
      defaultViewport: "tablet",
    },
    chromatic: { viewports: [420] },
  },
};
