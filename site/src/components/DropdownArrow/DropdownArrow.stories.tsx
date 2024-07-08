import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { DropdownArrow } from "./DropdownArrow";

const meta: Meta<typeof DropdownArrow> = {
  title: "components/DropdownArrow",
  parameters: { chromatic },
  component: DropdownArrow,
  args: {},
};

export default meta;
type Story = StoryObj<typeof DropdownArrow>;

export const Open: Story = {};
export const Close: Story = { args: { close: true } };
export const WithColor: Story = { args: { color: "#f00" } };
