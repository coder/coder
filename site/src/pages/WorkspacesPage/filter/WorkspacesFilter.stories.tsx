import { mockApiError } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	getDefaultFilterProps,
	MockMenu,
} from "components/Filter/storyHelpers";
import type { WorkspaceFilterState } from "./WorkspacesFilter";
import { WorkspacesFilter } from "./WorkspacesFilter";

const defaultFilterProps = getDefaultFilterProps<WorkspaceFilterState>({
	query: "owner:me",
	menus: {
		user: MockMenu,
		template: MockMenu,
		status: MockMenu,
		organizations: MockMenu,
	},
	values: {
		owner: "me",
		template: undefined,
		status: undefined,
	},
});

const meta: Meta<typeof WorkspacesFilter> = {
	title: "pages/WorkspacesPage/WorkspacesFilter",
	component: WorkspacesFilter,
	args: {
		filter: defaultFilterProps.filter,
		error: undefined,
		templateMenu: MockMenu,
		statusMenu: MockMenu,
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof WorkspacesFilter>;

export const Default: Story = {};

export const WithUserMenu: Story = {
	args: {
		userMenu: MockMenu,
	},
};

export const WithOrganizations: Story = {
	args: {
		userMenu: MockMenu,
		organizationsMenu: MockMenu,
	},
	parameters: {
		showOrganizations: true,
	},
};

export const Loading: Story = {
	args: {
		statusMenu: {
			...MockMenu,
			isInitializing: true,
		},
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Invalid filter query",
			validations: [{ field: "filter", detail: "Invalid filter syntax" }],
		}),
	},
};

export const WithDormantPreset: Story = {
	parameters: {
		features: ["advanced_template_scheduling"],
	},
};
