import { Story } from "@storybook/react"
import React from "react"
import { Sidebar, SidebarProps } from "./"

export default {
  title: "Page/Sidebar",
  component: Sidebar,
}

const Template: Story<SidebarProps> = (args: SidebarProps) => <Sidebar {...args} />

export const Example = Template.bind({})
Example.args = {
  activeItem: "oauthSettings",
  menuItems: [
    { label: "OAuth Settings", value: "oauthSettings" },
    { label: "Security", value: "security", hasChanges: true },
    { label: "Hardware", value: "hardware" },
  ],
}
