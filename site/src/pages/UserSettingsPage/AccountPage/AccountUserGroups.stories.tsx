import type { Meta, StoryObj } from "@storybook/react";
import type { Group } from "api/typesGenerated";
import {
  MockGroup as MockGroup1,
  MockUser,
  mockApiError,
} from "testHelpers/entities";
import { AccountUserGroups } from "./AccountUserGroups";

const MockGroup2: Group = {
  ...MockGroup1,
  avatar_url: "",
  display_name: "Goofy Goobers",
  members: [MockUser],
};

const mockError = mockApiError({
  message: "Failed to retrieve your groups",
});

const meta: Meta<typeof AccountUserGroups> = {
  title: "pages/UserSettingsPage/AccountUserGroups",
  component: AccountUserGroups,
  args: {
    groups: [MockGroup1, MockGroup2],
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
    groups: [MockGroup1],
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
    groups: [MockGroup1, MockGroup2],
    error: mockError,
    loading: false,
  },
};
