import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { ActionCell, ActionCellProps } from "./ActionCell"

export default {
  title: "AuditLog/Cells/ActionCell",
  component: ActionCell,
} as ComponentMeta<typeof ActionCell>

const Template: Story<ActionCellProps> = (args) => <ActionCell {...args} />

export const Example = Template.bind({})
Example.args = {
  action: "create",
}
