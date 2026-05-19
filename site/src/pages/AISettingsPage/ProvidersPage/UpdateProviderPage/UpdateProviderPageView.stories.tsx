import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import UpdateProviderPageView from "./UpdateProviderPageView";

const meta: Meta<typeof UpdateProviderPageView> = {
	title: "pages/AISettingsPage/UpdateProviderPageView",
	component: UpdateProviderPageView,
	decorators: [withToaster],
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/openai",
				searchParams: { organizationId: MockDefaultOrganization.id },
			},
			routing: [
				{ path: "/ai/settings", useStoryElement: true },
				{ path: "/ai/settings/:providerId", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof UpdateProviderPageView>;

export const Default: Story = {};
