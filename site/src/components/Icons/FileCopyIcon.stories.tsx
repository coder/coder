import { Story } from "@storybook/react"
import React from "react"
import { FileCopyIcon } from "./FileCopyIcon"

export default {
  title: "icons/FileCopyIcon",
  component: FileCopyIcon,
}

const Template: Story = (args) => <FileCopyIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
