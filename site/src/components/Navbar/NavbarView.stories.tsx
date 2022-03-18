import { Story } from "@storybook/react"
import React from "react"
import { NavbarView, NavbarViewProps } from "./NavbarView"

export default {
  title: "Page/NavbarView",
  component: NavbarView,
  argTypes: {
    onSignOut: { action: "Sign Out" },
  },
}

const Template: Story<NavbarViewProps> = (args: NavbarViewProps) => <NavbarView {...args} />

export const Primary = Template.bind({})
Primary.args = {
  user: { id: "1", username: "CathyCoder", email: "cathy@coder.com", created_at: "dawn" },
  onSignOut: () => {
    return Promise.resolve()
  },
}
