import type { Meta, StoryObj } from "@storybook/react";
import { DropdownButton } from "./DropdownButton";

const meta: Meta<typeof DropdownButton> = {
  title: "components/DropdownButton",
  component: DropdownButton,
  args: {
    children: "Open menu",
  },
};

export default meta;
type Story = StoryObj<typeof DropdownButton>;

export const Default: Story = {};
