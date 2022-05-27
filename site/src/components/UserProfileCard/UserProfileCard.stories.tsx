import { Story } from "@storybook/react"
import React from "react"
import { UserProfileCard, UserProfileCardProps } from "./UserProfileCard"

export default {
  title: "components/UserDropdown",
  component: UserProfileCard,
  argTypes: {
    onSignOut: { action: "Sign Out" },
  },
}

const Template: Story<UserProfileCardProps> = (args: UserProfileCardProps) => <UserProfileCard {...args} />

export const ExampleNoRoles = Template.bind({})
ExampleNoRoles.args = {
  user: {
    id: "1",
    username: "CathyCoder",
    email: "cathy@coder.com",
    created_at: "dawn",
    status: "active",
    organization_ids: [],
    roles: [],
  },
}

export const ExampleOneRole = Template.bind({})
ExampleOneRole.args = {
  user: {
    id: "1",
    username: "CathyCoder",
    email: "cathy@coder.com",
    created_at: "dawn",
    status: "active",
    organization_ids: [],
    roles: [{ name: "member", display_name: "Member" }],
  },
}

export const ExampleThreeRoles = Template.bind({})
ExampleThreeRoles.args = {
  user: {
    id: "1",
    username: "CathyCoder",
    email: "cathy@coder.com",
    created_at: "dawn",
    status: "active",
    organization_ids: [],
    roles: [
      { name: "admin", display_name: "Admin" },
      { name: "member", display_name: "Member" },
      { name: "auditor", display_name: "Auditor" },
    ],
  },
}
