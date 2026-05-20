import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockAIProviders } from "#/testHelpers/entities";
import ProvidersPageView from "./ProvidersPageView";

const meta: Meta<typeof ProvidersPageView> = {
	title: "pages/AISettingsPage/ProvidersPageView",
	component: ProvidersPageView,
	args: {
		isLoading: false,
		isFetching: false,
		providers: MockAIProviders,
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

// Open the "Add provider" dropdown so the Storybook surface captures
// the eight-row provider menu (matching the design) in a single shot.
export const AddProviderDropdownOpen: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = await canvas.findByRole("button", {
			name: /add provider/i,
		});
		await userEvent.click(trigger);
	},
};
