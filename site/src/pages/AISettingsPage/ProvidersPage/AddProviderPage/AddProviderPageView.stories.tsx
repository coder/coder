import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { withToaster } from "#/testHelpers/storybook";
import { addableProviders } from "../components/addableProviderTypes";
import AddProviderPageView from "./AddProviderPageView";

const meta: Meta<typeof AddProviderPageView> = {
	title: "pages/AISettingsPage/AddProviderPage",
	component: AddProviderPageView,
	decorators: [withToaster],
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

export default meta;
type Story = StoryObj<typeof AddProviderPageView>;

export const AddAnthropic: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "anthropic")!,
	},
};

export const AddOpenAI: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "openai")!,
	},
};

export const AddBedrock: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "bedrock")!,
	},
};
