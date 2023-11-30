import { Meta, StoryObj } from "@storybook/react";
import {
  MockUser,
  MockUser2,
  MockAssignableSiteRoles,
  mockApiError,
  MockAuthMethodsPasswordOnly,
} from "testHelpers/entities";
import { UsersPageView } from "./UsersPageView";
import { ComponentProps } from "react";
import {
  MockMenu,
  getDefaultFilterProps,
} from "components/Filter/storyHelpers";

type FilterProps = ComponentProps<typeof UsersPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
  query: "owner:me",
  menus: {
    status: MockMenu,
  },
  values: {
    status: "active",
  },
});

const meta: Meta<typeof UsersPageView> = {
  title: "pages/UsersPage",
  component: UsersPageView,
  args: {
    isNonInitialPage: false,
    users: [MockUser, MockUser2],
    roles: MockAssignableSiteRoles,

    canEditUsers: true,
    filterProps: defaultFilterProps,
    authMethods: MockAuthMethodsPasswordOnly,

    usersQuery: {
      isSuccess: true,
      currentPage: 1,
      limit: 25,
      totalRecords: 2,
      hasNextPage: false,
      hasPreviousPage: false,
      totalPages: 1,
      currentChunk: 1,
      isPreviousData: false,
      goToFirstPage: () => {},
      goToPreviousPage: () => {},
      goToNextPage: () => {},
      onPageChange: () => {},
    },
  },
};

export default meta;
type Story = StoryObj<typeof UsersPageView>;

export const Admin: Story = {};

export const SmallViewport = {
  parameters: {
    chromatic: { viewports: [600] },
  },
};

export const Member = {
  args: { canEditUsers: false },
};

export const Empty = {
  args: { users: [], count: 0 },
};

export const EmptyPage = {
  args: {
    users: [],
    count: 0,
    isNonInitialPage: true,
  },
};

export const Error = {
  args: {
    users: undefined,
    count: 0,
    filterProps: {
      ...defaultFilterProps,
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
  },
};
