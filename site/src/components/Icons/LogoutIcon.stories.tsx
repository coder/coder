import { Story } from "@storybook/react"
import { LogoutIcon } from "./LogoutIcon"

export default {
  title: "icons/LogoutIcon",
  component: LogoutIcon,
}

const Template: Story = (args) => <LogoutIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
