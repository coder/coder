import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockSiteRoles, MockUser, MockUser2 } from "../../testHelpers/renderHelpers"
import { UsersPageView, UsersPageViewProps } from "./UsersPageView"

export default {
  title: "pages/UsersPageView",
  component: UsersPageView,
} as ComponentMeta<typeof UsersPageView>

const Template: Story<UsersPageViewProps> = (args) => <UsersPageView {...args} />

export const Admin = Template.bind({})
Admin.args = {
  users: [MockUser, MockUser2],
  roles: MockSiteRoles,
  canCreateUser: true,
  canEditUsers: true,
}

export const Member = Template.bind({})
Member.args = { ...Admin.args, canCreateUser: false, canEditUsers: false }

export const Empty = Template.bind({})
Empty.args = { ...Admin.args, users: [] }
