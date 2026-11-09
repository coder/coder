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
		error: null,
		providers: MockAIProviders,
	},
	parameters: {
		// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
		pixel: { exclude: true },
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/providers" },
			routing: [
				{ path: "/ai/settings/providers", useStoryElement: true },
				{ path: "/ai/settings/providers/add", useStoryElement: true },
				{ path: "/ai/settings/providers/:providerId", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ProvidersPageView>;

export const Default: Story = {};

const longBaseUrl =
	"https://ai-gateway-proxy.internal.example-corporation.com/v1/providers/anthropic/aws-external-anthropic.us-east-1.api.aws";

export const LongBaseURL: Story = {
	args: {
		providers: [
			{ ...MockAIProviders[0], base_url: longBaseUrl },
			...MockAIProviders.slice(1),
		],
	},
	// No play function, so this story is safe to screenshot.
	parameters: { pixel: { exclude: false } },
};

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

export const LoadError: Story = {
	args: {
		isLoading: false,
		isFetching: false,
		error: new Error("Failed to load providers"),
		providers: [],
	},
};

export const AddProviderDropdownOpen: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = await canvas.findByRole("button", {
			name: /add provider/i,
		});
		await userEvent.click(trigger);
	},
};
