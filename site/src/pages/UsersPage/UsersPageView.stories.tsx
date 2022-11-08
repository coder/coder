import { ComponentMeta, Story } from "@storybook/react"
import { createPaginationRef } from "components/PaginationWidget/utils"
import {
  MockAssignableSiteRoles,
  MockUser,
  MockUser2,
} from "../../testHelpers/renderHelpers"
import { UsersPageView, UsersPageViewProps } from "./UsersPageView"

export default {
  title: "pages/UsersPageView",
  component: UsersPageView,
  argTypes: {
    paginationRef: {
      defaultValue: createPaginationRef({ page: 1, limit: 25 }),
    },
    isNonInitialPage: {
      defaultValue: false,
    },
    users: {
      defaultValue: [MockUser, MockUser2],
    },
    roles: {
      defaultValue: MockAssignableSiteRoles,
    },
    canEditUsers: {
      defaultValue: true,
    },
  },
} as ComponentMeta<typeof UsersPageView>

const Template: Story<UsersPageViewProps> = (args) => (
  <UsersPageView {...args} />
)

export const Admin = Template.bind({})

export const SmallViewport = Template.bind({})
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}

export const Member = Template.bind({})
Member.args = { canEditUsers: false }

export const Empty = Template.bind({})
Empty.args = { users: [] }

export const EmptyPage = Template.bind({})
EmptyPage.args = { users: [], isNonInitialPage: true }

export const Error = Template.bind({})
Error.args = {
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
