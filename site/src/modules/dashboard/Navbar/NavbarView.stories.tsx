import type { Meta, StoryObj } from "@storybook/react";
import { within, userEvent } from "@storybook/test";
import { chromaticWithTablet } from "testHelpers/chromatic";
import { MockUser, MockUser2 } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { NavbarView } from "./NavbarView";

const meta: Meta<typeof NavbarView> = {
  title: "modules/dashboard/NavbarView",
  parameters: { chromatic: chromaticWithTablet, layout: "fullscreen" },
  component: NavbarView,
  args: {
    user: MockUser,
    canViewAllUsers: true,
    canViewAuditLog: true,
    canViewDeployment: true,
    canViewHealth: true,
    canViewOrganizations: true,
  },
  decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof NavbarView>;

export const ForAdmin: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Deployment" }));
  },
};

export const ForAuditor: Story = {
  args: {
    user: MockUser2,
    canViewAllUsers: false,
    canViewAuditLog: true,
    canViewDeployment: false,
    canViewHealth: false,
    canViewOrganizations: false,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Deployment" }));
  },
};

export const ForOrgAdmin: Story = {
  args: {
    user: MockUser2,
    canViewAllUsers: false,
    canViewAuditLog: true,
    canViewDeployment: false,
    canViewHealth: false,
    canViewOrganizations: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Deployment" }));
  },
};

export const ForMember: Story = {
  args: {
    user: MockUser2,
    canViewAllUsers: false,
    canViewAuditLog: false,
    canViewDeployment: false,
    canViewHealth: false,
    canViewOrganizations: false,
  },
};

export const CustomLogo: Story = {
  args: {
    logo_url: "/icon/github.svg",
  },
};
