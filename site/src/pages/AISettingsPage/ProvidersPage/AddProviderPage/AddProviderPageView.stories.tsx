import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { mockApiError } from "#/testHelpers/entities";
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

// Server validation errors for base_url must render inline on the Endpoint
// input via backendFieldName, not only in the top-of-form ErrorAlert.
export const WithBaseUrlValidationError: Story = {
	args: {
		provider: addableProviders.find((p) => p.value === "openai-compat")!,
	},
	beforeEach: () => {
		spyOn(API, "createAIProvider").mockRejectedValue(
			mockApiError({
				message: "Invalid AI provider request.",
				validations: [{ field: "base_url", detail: "server base_url error" }],
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(await canvas.findByLabelText(/^name/i), "localai");
		await userEvent.type(
			canvas.getByLabelText(/^endpoint\s*\*?$/i),
			"http://localai:8080/v1",
		);
		await userEvent.type(canvas.getByLabelText(/api key/i), "sk-local");
		await userEvent.click(
			canvas.getByRole("button", { name: /add provider/i }),
		);
		// The endpoint input renders the server field error inline (via
		// backendFieldName), distinct from the top-of-form ErrorAlert.
		const endpointInput = canvas.getByLabelText(/^endpoint\s*\*?$/i);
		await waitFor(() =>
			expect(endpointInput).toHaveAttribute("aria-invalid", "true"),
		);
	},
};
