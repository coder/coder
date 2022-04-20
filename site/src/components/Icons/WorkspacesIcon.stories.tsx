import { Story } from "@storybook/react"
import React from "react"
import { WorkspacesIcon } from "./WorkspacesIcon"

export default {
  title: "icons/WorkspacesIcon",
  component: WorkspacesIcon,
}

const Template: Story = (args) => <WorkspacesIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
