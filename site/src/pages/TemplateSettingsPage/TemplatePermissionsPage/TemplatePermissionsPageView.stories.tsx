import type { Meta, StoryObj } from "@storybook/react";
import {
  MockOrganization,
  MockTemplateACL,
  MockTemplateACLEmpty,
} from "testHelpers/entities";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";

const meta: Meta<typeof TemplatePermissionsPageView> = {
  title: "pages/TemplateSettingsPage/TemplatePermissionsPageView",
  component: TemplatePermissionsPageView,
};

export default meta;
type Story = StoryObj<typeof TemplatePermissionsPageView>;

export const Empty: Story = {
  args: {
    templateACL: MockTemplateACLEmpty,
    canUpdatePermissions: false,
  },
};

export const WithTemplateACL: Story = {
  args: {
    templateACL: MockTemplateACL,
    canUpdatePermissions: false,
  },
};

export const WithUpdatePermissions: Story = {
  args: {
    templateACL: MockTemplateACL,
    canUpdatePermissions: true,
    organizationId: MockOrganization.id,
  },
};
