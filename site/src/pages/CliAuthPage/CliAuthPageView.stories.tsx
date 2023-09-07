import { Story } from "@storybook/react";
import { CliAuthPageView, CliAuthPageViewProps } from "./CliAuthPageView";

export default {
  title: "pages/CliAuthPageView",
  component: CliAuthPageView,
  argTypes: {
    sessionToken: { control: "text" },
  },
  args: {
    sessionToken: "some-session-token",
  },
};

const Template: Story<CliAuthPageViewProps> = (args) => (
  <CliAuthPageView {...args} />
);

export const Example = Template.bind({});
Example.args = {};
