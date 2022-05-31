import { Story } from "@storybook/react"
import { ErrorIcon } from "./ErrorIcon"

export default {
  title: "icons/ErrorIcon",
  component: ErrorIcon,
}

const Template: Story = (args) => <ErrorIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
