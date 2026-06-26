import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
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
			location: { path: "/ai/settings/providers/add" },
			routing: [
				{ path: "/ai/settings/providers", useStoryElement: true },
				{ path: "/ai/settings/providers/add", useStoryElement: true },
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Add an Anthropic provider");
	},
};

export const AddOpenAI: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "openai")!,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Add an OpenAI provider");
	},
};

export const AddBedrock: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "bedrock")!,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Add an AWS Bedrock provider");
	},
};

export const AddCopilot: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "copilot")!,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Add a GitHub Copilot provider");
	},
};
