import { Story } from "@storybook/react"
import React from "react"
import { NavbarView, NavbarViewProps } from "."

export default {
  title: "components/NavbarView",
  component: NavbarView,
  argTypes: {
    onSignOut: { action: "Sign Out" },
  },
}

const Template: Story<NavbarViewProps> = (args: NavbarViewProps) => <NavbarView {...args} />

export const ForAdmin = Template.bind({})
ForAdmin.args = {
  user: { id: "1", username: "Administrator", email: "admin@coder.com", created_at: "dawn" },
  onSignOut: () => {
    return Promise.resolve()
  },
}

export const ForMember = Template.bind({})
ForMember.args = {
  user: { id: "1", username: "CathyCoder", email: "cathy@coder.com", created_at: "dawn" },
  onSignOut: () => {
    return Promise.resolve()
  },
}
