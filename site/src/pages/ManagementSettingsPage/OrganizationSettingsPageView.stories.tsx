import type { Meta, StoryObj } from "@storybook/react";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";
import { MockOrganization } from "testHelpers/entities";

const meta: Meta<typeof OrganizationSettingsPageView> = {
  title: "pages/OrganizationSettingsPageView",
  component: OrganizationSettingsPageView,
  args: {
    org: MockOrganization,
  },
};

export default meta;
type Story = StoryObj<typeof OrganizationSettingsPageView>;

export const Example: Story = {};
