import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { ActionCell, ActionCellProps } from "./ActionCell"

export default {
  title: "AuditLog/Cells/ActionCell",
  component: ActionCell,
} as ComponentMeta<typeof ActionCell>

const Template: Story<ActionCellProps> = (args) => <ActionCell {...args} />

export const Success = Template.bind({})
Success.args = {
  action: "create",
  statusCode: 200,
}

export const Failure = Template.bind({})
Failure.args = {
  action: "create",
  statusCode: 500,
}
