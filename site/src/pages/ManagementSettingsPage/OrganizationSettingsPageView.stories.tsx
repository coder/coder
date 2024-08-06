import type { Meta, StoryObj } from "@storybook/react";
import {
  MockDefaultOrganization,
  MockOrganization,
} from "testHelpers/entities";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const meta: Meta<typeof OrganizationSettingsPageView> = {
  title: "pages/OrganizationSettingsPageView",
  component: OrganizationSettingsPageView,
  args: {
    organization: MockOrganization,
    canEdit: true,
  },
};

export default meta;
type Story = StoryObj<typeof OrganizationSettingsPageView>;

export const Example: Story = {};

export const DefaultOrg: Story = {
  args: {
    organization: MockDefaultOrganization,
  },
};

export const CannotEdit: Story = {
  args: {
    canEdit: false,
  },
};
