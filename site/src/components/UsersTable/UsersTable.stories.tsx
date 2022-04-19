import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockUser, MockUser2 } from "../../testHelpers"
import { UsersTable, UsersTableProps } from "./UsersTable"

export default {
  title: "Components/UsersTable",
  component: UsersTable,
} as ComponentMeta<typeof UsersTable>

const Template: Story<UsersTableProps> = (args) => <UsersTable {...args} />

export const Example = Template.bind({})
Example.args = {
  users: [MockUser, MockUser2],
}

export const Empty = Template.bind({})
Empty.args = {
  users: [],
}
