import { ComponentMeta, Story } from "@storybook/react"
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

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  ...Admin.args,
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}

export const Member = Template.bind({})
Member.args = { ...Admin.args, canCreateUser: false, canEditUsers: false }

export const Empty = Template.bind({})
Empty.args = { ...Admin.args, users: [] }

export const Error = Template.bind({})
Error.args = {
  ...Admin.args,
  users: undefined,
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
