import { Story } from "@storybook/react"
import { TabPanel, TabPanelProps } from "./TabPanel"

export default {
  title: "components/TabPanel",
  component: TabPanel,
}

const Template: Story<TabPanelProps> = (args: TabPanelProps) => (
  <TabPanel {...args} />
)

export const Example = Template.bind({})
Example.args = {
  title: "Title",
  menuItems: [
    { label: "OAuth Settings", path: "oauthSettings" },
    { label: "Security", path: "oauthSettings", hasChanges: true },
    { label: "Hardware", path: "oauthSettings" },
  ],
}
