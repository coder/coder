import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { withToaster } from "#/testHelpers/storybook";
import UpdateProviderPageView from "./UpdateProviderPageView";

const meta: Meta<typeof UpdateProviderPageView> = {
	title: "pages/AISettingsPage/UpdateProviderPageView",
	component: UpdateProviderPageView,
	decorators: [withToaster],
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/openai" },
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
