import { Story } from "@storybook/react"
import { Logo } from "./Logo"

export default {
  title: "icons/Logo",
  component: Logo,
}

const Template: Story = (args) => <Logo fill="black" {...args} />

export const Example = Template.bind({})
Example.args = {}
