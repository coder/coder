import { Meta, StoryObj } from "@storybook/react";
import { MockAuditLog, MockAuditLog2, MockUser } from "testHelpers/entities";
import { AuditPageView } from "./AuditPageView";
import { ComponentProps } from "react";
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
    count: 1000,
    page: 1,
    limit: 25,
    isAuditLogVisible: true,
    filterProps: defaultFilterProps,
  },
};

export default meta;
type Story = StoryObj<typeof AuditPageView>;

export const AuditPage: Story = {};

export const Loading = {
  args: {
    auditLogs: undefined,
    count: undefined,
    isNonInitialPage: false,
  },
};

export const EmptyPage = {
  args: {
    auditLogs: [],
    isNonInitialPage: true,
  },
};

export const NoLogs = {
  args: {
    auditLogs: [],
    count: 0,
    isNonInitialPage: false,
  },
};

export const NotVisible = {
  args: {
    isAuditLogVisible: false,
  },
};

export const AuditPageSmallViewport = {
  parameters: {
    chromatic: { viewports: [600] },
  },
};
