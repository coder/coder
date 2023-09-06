import Box from "@mui/material/Box"
import { Story } from "@storybook/react"
import { MockUser } from "../../../../testHelpers/entities"
import { UserDropdown, UserDropdownProps } from "./UserDropdown"

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

export const Example = Template.bind({})
Example.args = {
  user: MockUser,
  onSignOut: () => {
    return Promise.resolve()
  },
}
