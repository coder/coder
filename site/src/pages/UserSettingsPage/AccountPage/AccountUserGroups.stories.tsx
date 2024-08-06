import type { Meta, StoryObj } from "@storybook/react";
import type { ReducedGroup } from "api/typesGenerated";
import {
  MockGroup as MockGroup1,
  mockApiError,
} from "testHelpers/entities";
import { AccountUserGroups } from "./AccountUserGroups";

const MockReducedGroup1: ReducedGroup = {
  id: MockGroup1.id,
  name: MockGroup1.name,
  display_name: MockGroup1.display_name,
  organization_id: MockGroup1.organization_id,
  avatar_url: MockGroup1.avatar_url,
  total_member_count: 10,
};

const MockReducedGroup2: ReducedGroup = {
  ...MockReducedGroup1,
  display_name: "Goofy Goobers",
  total_member_count: 5,
};

const mockError = mockApiError({
  message: "Failed to retrieve your groups",
});

const meta: Meta<typeof AccountUserGroups> = {
  title: "pages/UserSettingsPage/AccountUserGroups",
  component: AccountUserGroups,
  args: {
    groups: [MockReducedGroup1, MockReducedGroup2],
    loading: false,
  },
};

export default meta;
type Story = StoryObj<typeof AccountUserGroups>;

export const Example: Story = {};

export const NoGroups: Story = {
  args: {
    groups: [],
  },
};

export const OneGroup: Story = {
  args: {
    groups: [MockReducedGroup1],
  },
};

export const Loading: Story = {
  args: {
    groups: undefined,
    loading: true,
  },
};

export const Error: Story = {
  args: {
    groups: undefined,
    error: mockError,
    loading: false,
  },
};

export const ErrorWithPreviousData: Story = {
  args: {
    groups: [MockReducedGroup1, MockReducedGroup2],
    error: mockError,
    loading: false,
  },
};
