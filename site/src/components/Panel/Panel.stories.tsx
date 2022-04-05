import { Story } from "@storybook/react"
import React from "react"
import { Panel, PanelProps } from "./"

export default {
  title: "Page/Panel",
  component: Panel,
}

const Template: Story<PanelProps> = (args: PanelProps) => <Panel {...args} />

export const Example = Template.bind({})
Example.args = {
  title: "Panel title",
  activeTab: "oauthSettings",
  menuItems: [
    { label: "OAuth Settings", value: "oauthSettings" },
    { label: "Security", value: "oauthSettings", hasChanges: true },
    { label: "Hardware", value: "oauthSettings" },
  ],
}
