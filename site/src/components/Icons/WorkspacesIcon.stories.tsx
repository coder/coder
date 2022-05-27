import { Story } from "@storybook/react"
import { WorkspacesIcon } from "./WorkspacesIcon"

export default {
  title: "icons/WorkspacesIcon",
  component: WorkspacesIcon,
}

const Template: Story = (args) => <WorkspacesIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
