import { ComponentMeta, Story } from "@storybook/react";
import { MockTemplate } from "../../testHelpers/entities";
import { SelectedTemplate, SelectedTemplateProps } from "./SelectedTemplate";

export default {
  title: "components/SelectedTemplate",
  component: SelectedTemplate,
} as ComponentMeta<typeof SelectedTemplate>;

const Template: Story<SelectedTemplateProps> = (args) => (
  <SelectedTemplate {...args} />
);

export const WithIcon = Template.bind({});
WithIcon.args = {
  template: {
    ...MockTemplate,
    icon: "/icon/docker.png",
  },
};

export const WithoutIcon = Template.bind({});
WithoutIcon.args = {
  template: {
    ...MockTemplate,
    icon: "",
  },
};
