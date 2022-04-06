import Box from "@material-ui/core/Box"
import { Story } from "@storybook/react"
import React from "react"
import { UserDropdown, UserDropdownProps } from "./UserDropdown"

export default {
  title: "Page/UserDropdown",
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

export const Example = Template.bind({})
Example.args = {
  user: { id: "1", username: "CathyCoder", email: "cathy@coder.com", created_at: "dawn" },
  onSignOut: () => {
    return Promise.resolve()
  },
}
