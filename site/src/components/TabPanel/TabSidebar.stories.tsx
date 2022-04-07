import { Story } from "@storybook/react"
import React from "react"
import { TabSidebar, TabSidebarProps } from "./TabSidebar"

export default {
  title: "TabPanel/TabSidebar",
  component: TabSidebar,
}

const Template: Story<TabSidebarProps> = (args: TabSidebarProps) => <TabSidebar {...args} />

export const Example = Template.bind({})
Example.args = {
  menuItems: [
    { label: "OAuth Settings", path: "oauthSettings" },
    { label: "Security", path: "security", hasChanges: true },
    { label: "Hardware", path: "hardware" },
  ],
}
