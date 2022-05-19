import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockSiteRoles, MockUser, MockUser2 } from "../../testHelpers/renderHelpers"
import { UsersTable, UsersTableProps } from "./UsersTable"

export default {
  title: "components/UsersTable",
  component: UsersTable,
} as ComponentMeta<typeof UsersTable>

const Template: Story<UsersTableProps> = (args) => <UsersTable {...args} />

export const Example = Template.bind({})
Example.args = {
  users: [MockUser, MockUser2],
  roles: MockSiteRoles,
  canEditUsers: false,
}

export const Editable = Template.bind({})
Editable.args = {
  users: [MockUser, MockUser2],
  roles: MockSiteRoles,
  canEditUsers: true,
}

export const Empty = Template.bind({})
Empty.args = {
  users: [],
  roles: MockSiteRoles,
}
