import { MockUser } from "testHelpers/entities";
import { UserDropdownContent } from "./UserDropdownContent";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof UserDropdownContent> = {
  title: "components/UserDropdownContent",
  component: UserDropdownContent,
};

export default meta;
type Story = StoryObj<typeof UserDropdownContent>;

export const ExampleNoRoles: Story = {
  args: {
    user: {
      ...MockUser,
      roles: [],
    },
  },
};

export const ExampleOneRole: Story = {
  args: {
    user: {
      ...MockUser,
      roles: [{ name: "member", display_name: "Member" }],
    },
  },
};

export const ExampleThreeRoles: Story = {
  args: {
    user: {
      ...MockUser,
      roles: [
        { name: "admin", display_name: "Admin" },
        { name: "member", display_name: "Member" },
        { name: "auditor", display_name: "Auditor" },
      ],
    },
  },
};
