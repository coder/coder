import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { TargetCell, TargetCellProps } from "./TargetCell"

export default {
  title: "AuditLog/Cells/TargetCell",
  component: TargetCell,
  argTypes: {
    onSelect: {
      action: "onSelect",
    },
  },
} as ComponentMeta<typeof TargetCell>

const Template: Story<TargetCellProps> = (args) => <TargetCell {...args} />

export const Example = Template.bind({})
Example.args = {
  name: "Coder frontend",
  type: "project",
}
