import Box from "@material-ui/core/Box"
import { Story } from "@storybook/react"
import React from "react"
import { AdminDropdown } from "."

export default {
  title: "Navbar/AdminDropdown",
  component: AdminDropdown,
}

const Template: Story = () => (
  <Box style={{ backgroundColor: "#000", width: 100 }}>
    <AdminDropdown />
  </Box>
)

export const Example = Template.bind({})
