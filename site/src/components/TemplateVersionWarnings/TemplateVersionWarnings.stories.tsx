import { Story } from "@storybook/react";
import {
  TemplateVersionWarnings,
  TemplateVersionWarningsProps,
} from "./TemplateVersionWarnings";

export default {
  title: "components/TemplateVersionWarnings",
  component: TemplateVersionWarnings,
};

const Template: Story<TemplateVersionWarningsProps> = (args) => (
  <TemplateVersionWarnings {...args} />
);

export const UnsupportedWorkspaces = Template.bind({});
UnsupportedWorkspaces.args = {
  warnings: ["UNSUPPORTED_WORKSPACES"],
};
