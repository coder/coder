import { Story } from "@storybook/react"
import { VSCodeIcon } from "./VSCodeIcon"

export default {
  title: "icons/VSCodeIcon",
  component: VSCodeIcon,
}

const Template: Story = (args) => <VSCodeIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
