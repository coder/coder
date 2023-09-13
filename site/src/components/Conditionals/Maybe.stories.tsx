import { StoryObj, Meta } from "@storybook/react";
import { Maybe } from "./Maybe";

const meta: Meta<typeof Maybe> = {
  title: "components/Conditionals/Maybe",
  component: Maybe,
  args: {
    children: "Now you see me",
  },
};

export default meta;
type Story = StoryObj<typeof Maybe>;

export const ConditionIsTrue: Story = {
  args: {
    condition: true,
  },
};

export const ConditionIsFalse: Story = {
  args: {
    condition: false,
  },
};
