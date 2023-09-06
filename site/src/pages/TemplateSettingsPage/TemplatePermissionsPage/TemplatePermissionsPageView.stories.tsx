import { Story } from "@storybook/react";
import {
  MockOrganization,
  MockTemplateACL,
  MockTemplateACLEmpty,
} from "testHelpers/entities";
import {
  TemplatePermissionsPageView,
  TemplatePermissionsPageViewProps,
} from "./TemplatePermissionsPageView";

export default {
  title: "pages/TemplatePermissionsPageView",
  component: TemplatePermissionsPageView,
};

const Template: Story<TemplatePermissionsPageViewProps> = (
  args: TemplatePermissionsPageViewProps,
) => <TemplatePermissionsPageView {...args} />;

export const Empty = Template.bind({});
Empty.args = {
  templateACL: MockTemplateACLEmpty,
  canUpdatePermissions: false,
};

export const WithTemplateACL = Template.bind({});
WithTemplateACL.args = {
  templateACL: MockTemplateACL,
  canUpdatePermissions: false,
};

export const WithUpdatePermissions = Template.bind({});
WithUpdatePermissions.args = {
  templateACL: MockTemplateACL,
  canUpdatePermissions: true,
  organizationId: MockOrganization.id,
};
