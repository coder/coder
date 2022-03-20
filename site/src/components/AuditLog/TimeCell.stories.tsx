import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { TimeCell, TimeCellProps } from "./TimeCell"

export default {
  title: "AuditLog/Cells/TimeCell",
  component: TimeCell,
} as ComponentMeta<typeof TimeCell>

const Template: Story<TimeCellProps> = (args) => <TimeCell {...args} />

export const Example = Template.bind({})
Example.args = {
  date: new Date(),
}
