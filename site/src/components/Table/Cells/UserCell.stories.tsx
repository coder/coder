import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockUser, MockUserAgent } from "../../../test_helpers"
import { UserCell, UserCellProps } from "./UserCell"

export default {
  title: "Table/Cells/UserCell",
  component: UserCell,
  argTypes: {
    onPrimaryTextSelect: {
      action: "onPrimaryTextSelect",
    },
  },
} as ComponentMeta<typeof UserCell>

const Template: Story<UserCellProps> = (args) => <UserCell {...args} />

export const Example = Template.bind({})
Example.args = {
  Avatar: {
    username: MockUser.username,
  },
  caption: MockUserAgent.ip_address,
  primaryText: MockUser.email,
}
