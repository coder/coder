import Box from "@material-ui/core/Box"
import { Story } from "@storybook/react"
import React from "react"
import { UserDropdown, UserDropdownProps } from "./UsersDropdown"

export default {
  title: "components/UserDropdown",
  component: UserDropdown,
  argTypes: {
    onSignOut: { action: "Sign Out" },
  },
}

const Template: Story<UserDropdownProps> = (args: UserDropdownProps) => (
  <Box style={{ backgroundColor: "#000", width: 88 }}>
    <UserDropdown {...args} />
  </Box>
)

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
  onSignOut: () => {
    return Promise.resolve()
  },
}

export const ExampleOneRole = Template.bind({})
ExampleNoRoles.args = {
  user: {
    id: "1",
    username: "CathyCoder",
    email: "cathy@coder.com",
    created_at: "dawn",
    status: "active",
    organization_ids: [],
    roles: [{ name: "member", display_name: "Member" }],
  },
  onSignOut: () => {
    return Promise.resolve()
  },
}

export const ExampleThreeRoles = Template.bind({})
ExampleNoRoles.args = {
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
  onSignOut: () => {
    return Promise.resolve()
  },
}
