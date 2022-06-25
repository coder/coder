import { ComponentMeta, Story } from "@storybook/react"
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

export const Error = Template.bind({})
Error.args = {
  users: [MockUser, MockUser2],
  roles: MockSiteRoles,
  canEditUsers: true,
  error: {
    response: {
      data: {
        message: "Invalid user search query.",
        validations: [
          {
            field: "status",
            detail: `Query param "status" has invalid value: "inactive" is not a valid user status`,
          },
        ],
      },
    },
    isAxiosError: true,
  },
}
