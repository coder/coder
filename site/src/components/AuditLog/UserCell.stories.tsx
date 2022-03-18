import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockUser, MockUserAgent } from "../../test_helpers"
import { UserCell, UserCellProps } from "./UserCell"

export default {
  title: "AuditLog/Cells/UserCell",
  component: UserCell,
  argTypes: {
    onSelectEmail: {
      action: "onSelectEmail",
    },
  },
} as ComponentMeta<typeof UserCell>

const Template: Story<UserCellProps> = (args) => <UserCell {...args} />

export const Example = Template.bind({})
Example.args = {
  user: MockUser,
  userAgent: MockUserAgent,
}
