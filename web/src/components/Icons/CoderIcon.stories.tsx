import { Story } from "@storybook/react"
import { CoderIcon } from "./CoderIcon"

export default {
  title: "icons/CoderIcon",
  component: CoderIcon,
}

const Template: Story = (args) => <CoderIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
