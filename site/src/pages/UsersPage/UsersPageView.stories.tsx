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

import { type UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import { mockSuccessResult } from "components/PaginationWidget/PaginationContainer.mocks";

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
      ...mockSuccessResult,
      totalRecords: 2,
    } as UsePaginatedQueryResult,
  },
};

export default meta;
type Story = StoryObj<typeof UsersPageView>;

export const Admin: Story = {};

export const SmallViewport: Story = {
  parameters: {
    chromatic: { viewports: [600] },
  },
};

export const Member: Story = {
  args: { canEditUsers: false },
};

export const Empty: Story = {
  args: {
    users: [],
    usersQuery: {
      ...mockSuccessResult,
      totalRecords: 0,
    } as UsePaginatedQueryResult,
  },
};

export const EmptyPage: Story = {
  args: {
    users: [],
    isNonInitialPage: true,
    usersQuery: {
      ...mockSuccessResult,
      totalRecords: 0,
    } as UsePaginatedQueryResult,
  },
};

export const Error: Story = {
  args: {
    users: undefined,
    usersQuery: {
      ...mockSuccessResult,
      totalRecords: 0,
    } as UsePaginatedQueryResult,
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
