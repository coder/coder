import { ComponentMeta, Story } from "@storybook/react";
import { Margins } from "./Margins";

export default {
  title: "components/Margins",
  component: Margins,
} as ComponentMeta<typeof Margins>;

const Template: Story = (args) => (
  <Margins {...args}>
    <div style={{ width: "100%", background: "black" }}>
      Here is some content that will not get too wide!
    </div>
  </Margins>
);

export const Example = Template.bind({});
