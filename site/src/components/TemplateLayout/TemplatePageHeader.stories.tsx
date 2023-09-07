import { ComponentMeta, Story } from "@storybook/react";
import { MockTemplate, MockTemplateVersion } from "testHelpers/entities";
import {
  TemplatePageHeader,
  TemplatePageHeaderProps,
} from "./TemplatePageHeader";

export default {
  title: "Components/TemplatePageHeader",
  component: TemplatePageHeader,
  args: {
    template: MockTemplate,
    activeVersion: MockTemplateVersion,
    permissions: {
      canUpdateTemplate: true,
    },
  },
} as ComponentMeta<typeof TemplatePageHeader>;

const Template: Story<TemplatePageHeaderProps> = (args) => (
  <TemplatePageHeader {...args} />
);

export const CanUpdate = Template.bind({});
CanUpdate.args = {};

export const CanNotUpdate = Template.bind({});
CanNotUpdate.args = {
  permissions: {
    canUpdateTemplate: false,
  },
};
