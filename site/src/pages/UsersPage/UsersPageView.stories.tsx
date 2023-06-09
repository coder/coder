import { Meta, StoryObj } from "@storybook/react"
import { createPaginationRef } from "components/PaginationWidget/utils"
import {
  MockUser,
  MockUser2,
  MockAssignableSiteRoles,
  mockApiError,
} from "testHelpers/entities"
import { UsersPageView } from "./UsersPageView"
import { action } from "@storybook/addon-actions"

const meta: Meta<typeof UsersPageView> = {
  title: "pages/UsersPageView",
  component: UsersPageView,
  args: {
    paginationRef: createPaginationRef({ page: 1, limit: 25 }),
    isNonInitialPage: false,
    users: [MockUser, MockUser2],
    roles: MockAssignableSiteRoles,
    canEditUsers: true,
    filterProps: {
      onFilter: action("onFilter"),
      filter: "",
    },
  },
}

export default meta
type Story = StoryObj<typeof UsersPageView>

export const Admin: Story = {}

export const SmallViewport = {
  parameters: {
    chromatic: { viewports: [600] },
  },
}

export const Member = {
  args: { canEditUsers: false },
}

export const Empty = {
  args: { users: [] },
}

export const EmptyPage = {
  args: {
    users: [],
    isNonInitialPage: true,
  },
}

export const Error = {
  args: {
    users: undefined,
    error: mockApiError({
      message: "Invalid user search query.",
      validations: [
        {
          field: "status",
          detail: `Query param "status" has invalid value: "inactive" is not a valid user status`,
        },
      ],
    }),
  },
}
