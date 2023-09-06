import { Story } from "@storybook/react";
import { Expander, ExpanderProps } from "./Expander";

export default {
  title: "components/Expander",
  component: Expander,
  argTypes: {
    setExpanded: { action: "setExpanded" },
  },
};

const Template: Story<ExpanderProps> = (args) => <Expander {...args} />;

export const Expanded = Template.bind({});
Expanded.args = {
  expanded: true,
};

export const Collapsed = Template.bind({});
Collapsed.args = {
  expanded: false,
};
