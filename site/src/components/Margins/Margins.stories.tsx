import { action } from "@storybook/addon-actions"
import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { Margins } from "./Margins"

export default {
  title: "components/Margins",
  component: Margins,
} as ComponentMeta<typeof Margins>

const Template: Story = (args) => (
  <Margins {...args}>
    <div style={{ width: "100%", background: "lightgrey" }}>
      Here's some content that won't get too wide!
    </div>
  </Margins>
)

export const Example = Template.bind({})
