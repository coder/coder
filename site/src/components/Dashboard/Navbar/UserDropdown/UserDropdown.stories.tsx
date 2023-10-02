import { MockUser } from "testHelpers/entities";
import { UserDropdown } from "./UserDropdown";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof UserDropdown> = {
  title: "components/UserDropdown",
  component: UserDropdown,
  args: {
    user: MockUser,
  },
};

export default meta;
type Story = StoryObj<typeof UserDropdown>;

export const Example: Story = {};
