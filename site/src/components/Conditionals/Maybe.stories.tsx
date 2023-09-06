import { Story } from "@storybook/react";
import { Maybe, MaybeProps } from "./Maybe";

export default {
  title: "components/Conditionals/Maybe",
  component: Maybe,
};

const Template: Story<MaybeProps> = (args: MaybeProps) => (
  <Maybe {...args}>Now you see me</Maybe>
);

export const ConditionIsTrue = Template.bind({});
ConditionIsTrue.args = {
  condition: true,
};

export const ConditionIsFalse = Template.bind({});
ConditionIsFalse.args = {
  condition: false,
};
