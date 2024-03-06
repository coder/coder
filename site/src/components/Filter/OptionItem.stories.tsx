import type { Meta, StoryObj } from "@storybook/react";
import { OptionItem } from "./filter";

const meta: Meta<typeof OptionItem> = {
  title: "components/Filter/OptionItem",
  component: OptionItem,
  decorators: [
    (Story) => {
      return (
        <div style={{ width: "300px" }}>
          <Story />
        </div>
      );
    },
  ],
};

export default meta;
type Story = StoryObj<typeof OptionItem>;

export const Selected: Story = {
  args: {
    option: {
      label: "Success option",
      value: "success",
    },
    isSelected: true,
  },
};

export const NotSelected: Story = {
  args: {
    option: {
      label: "Success option",
      value: "success",
    },
    isSelected: false,
  },
};
