import { Story } from "@storybook/react"
import { CloseIcon } from "./CloseIcon"

export default {
  title: "icons/CloseIcon",
  component: CloseIcon,
}

const Template: Story = (args) => <CloseIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
