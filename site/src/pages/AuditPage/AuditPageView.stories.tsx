import { Meta, StoryObj } from "@storybook/react";
import { type ComponentProps } from "react";
import { chromaticWithTablet } from "testHelpers/chromatic";
import { MockAuditLog, MockAuditLog2, MockUser } from "testHelpers/entities";
import {
  mockInitialRenderResult,
  mockSuccessResult,
} from "components/PaginationWidget/PaginationContainer.mocks";
import { type UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import { AuditPageView } from "./AuditPageView";

import {
  MockMenu,
  getDefaultFilterProps,
} from "components/Filter/storyHelpers";

type FilterProps = ComponentProps<typeof AuditPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
  query: `owner:me`,
  values: {
    username: MockUser.username,
    action: undefined,
    resource_type: undefined,
  },
  menus: {
    user: MockMenu,
    action: MockMenu,
    resourceType: MockMenu,
  },
});

const meta: Meta<typeof AuditPageView> = {
  title: "pages/AuditPage",
  component: AuditPageView,
  args: {
    auditLogs: [MockAuditLog, MockAuditLog2],
    isAuditLogVisible: true,
    filterProps: defaultFilterProps,
  },
};

export default meta;
type Story = StoryObj<typeof AuditPageView>;

export const AuditPage: Story = {
  parameters: { chromatic: chromaticWithTablet },
  args: {
    auditsQuery: mockSuccessResult,
  },
};

export const Loading: Story = {
  args: {
    auditLogs: undefined,
    isNonInitialPage: false,
    auditsQuery: mockInitialRenderResult,
  },
};

export const EmptyPage: Story = {
  args: {
    auditLogs: [],
    isNonInitialPage: true,
    auditsQuery: {
      ...mockSuccessResult,
      totalRecords: 0,
    } as UsePaginatedQueryResult,
  },
};

export const NoLogs: Story = {
  args: {
    auditLogs: [],
    isNonInitialPage: false,
    auditsQuery: {
      ...mockSuccessResult,
      totalRecords: 0,
    } as UsePaginatedQueryResult,
  },
};

export const NotVisible: Story = {
  args: {
    isAuditLogVisible: false,
    auditsQuery: mockInitialRenderResult,
  },
};
