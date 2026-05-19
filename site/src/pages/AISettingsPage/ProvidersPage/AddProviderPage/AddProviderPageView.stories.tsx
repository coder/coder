import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import AddProviderPageView from "./AddProviderPageView";

const meta: Meta<typeof AddProviderPageView> = {
	title: "pages/AISettingsPage/AddProviderPage",
	component: AddProviderPageView,
	decorators: [withToaster],
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/add",
				searchParams: { organizationId: MockDefaultOrganization.id },
			},
			routing: [
				{ path: "/ai/settings", useStoryElement: true },
				{ path: "/ai/settings/add", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AddProviderPageView>;

export const Default: Story = {};

export const NoOrganizationParam: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/add" },
			routing: [
				{ path: "/ai/settings", useStoryElement: true },
				{ path: "/ai/settings/add", useStoryElement: true },
			],
		}),
	},
};

export const NoOrganizations: Story = {
	args: {
		organizations: [],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/add" },
			routing: [
				{ path: "/ai/settings", useStoryElement: true },
				{ path: "/ai/settings/add", useStoryElement: true },
			],
		}),
	},
};
