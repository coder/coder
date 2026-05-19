import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockAIProviders,
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
} from "#/testHelpers/entities";
import ProvidersPageView from "./ProvidersPageView";

const meta: Meta<typeof ProvidersPageView> = {
	title: "pages/AISettingsPage/ProvidersPageView",
	component: ProvidersPageView,
	args: {
		isLoading: false,
		isFetching: false,
		providers: MockAIProviders,
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings" },
			routing: [
				{ path: "/ai/settings", useStoryElement: true },
				{ path: "/ai/settings/add", useStoryElement: true },
				{ path: "/ai/settings/:providerId", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ProvidersPageView>;

export const Default: Story = {};

export const Loading: Story = {
	args: {
		isLoading: true,
		isFetching: true,
	},
};

export const EmptyProviders: Story = {
	args: {
		providers: [],
	},
};

export const NoOrganizations: Story = {
	args: {
		organizations: [],
		providers: MockAIProviders,
	},
};

export const ManyOrganizations: Story = {
	args: {
		organizations: [
			MockDefaultOrganization,
			MockOrganization2,
			MockOrganization3,
		],
	},
};
