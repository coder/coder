import { ComponentMeta, Story } from "@storybook/react"
import { MockUser, MockUserAgent } from "../../testHelpers/renderHelpers"
import { UserCell, UserCellProps } from "./UserCell"

export default {
  title: "components/UserCell",
  component: UserCell,
} as ComponentMeta<typeof UserCell>

const Template: Story<UserCellProps> = (args) => <UserCell {...args} />

export const AuditLogExample = Template.bind({})
AuditLogExample.args = {
  Avatar: {
    username: MockUser.username,
    avatarURL: "",
  },
  caption: MockUserAgent.ip_address,
  primaryText: MockUser.email,
  onPrimaryTextSelect: () => {
    return
  },
}

export const AuditLogEmptyUserExample = Template.bind({})
AuditLogEmptyUserExample.args = {
  Avatar: {
    username: MockUser.username,
    avatarURL: "",
  },
  caption: MockUserAgent.ip_address,
  primaryText: "Deleted User",
  onPrimaryTextSelect: undefined,
}
