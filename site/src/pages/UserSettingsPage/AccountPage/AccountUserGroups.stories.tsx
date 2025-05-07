import type { Meta, StoryObj } from "@storybook/react";
import { MockGroup as MockGroup1, mockApiError } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { AccountUserGroups } from "./AccountUserGroups";

const MockGroup2 = {
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
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof AccountUserGroups>;

export const Example: Story = {};

export const ExampleWithOrganizations: Story = {
	parameters: {
		showOrganizations: true,
	},
};

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

export const WithError: Story = {
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
