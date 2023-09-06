import { ComponentMeta, Story } from "@storybook/react";
import { FormFooter, FormFooterProps } from "./FormFooter";

export default {
  title: "components/FormFooter",
  component: FormFooter,
  argTypes: {
    onCancel: { action: "cancel" },
  },
} as ComponentMeta<typeof FormFooter>;

const Template: Story<FormFooterProps> = (args) => <FormFooter {...args} />;

export const Ready = Template.bind({});
Ready.args = {
  isLoading: false,
};

export const Custom = Template.bind({});
Custom.args = {
  isLoading: false,
  submitLabel: "Create",
};

export const Loading = Template.bind({});
Loading.args = {
  isLoading: true,
};
