import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { AIProvider } from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderClaudePlatformAWS,
	MockAIProviderClaudePlatformAWSAPIKey,
	MockAIProviderCopilot,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import UpdateProviderPageView from "./UpdateProviderPageView";

const routingFor = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: "/ai/settings/providers", useStoryElement: true },
			{ path: "/ai/settings/providers/:providerId", useStoryElement: true },
		],
	});

const seed = (provider: AIProvider) => ({
	queries: [{ key: ["ai", "providers", provider.name], data: provider }],
});

const meta: Meta<typeof UpdateProviderPageView> = {
	title: "pages/AISettingsPage/UpdateProviderPageView",
	component: UpdateProviderPageView,
	decorators: [withToaster],
};

export default meta;
type Story = StoryObj<typeof UpdateProviderPageView>;

export const OpenAI: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
		),
		...seed(MockAIProviderOpenAI),
	},
};

export const Anthropic: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderAnthropic.name}`,
		),
		...seed(MockAIProviderAnthropic),
	},
};

export const Bedrock: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderBedrock.name}`,
		),
		...seed(MockAIProviderBedrock),
	},
};

// Copilot has no stored credential, so the edit form renders no API key
// field and keeps the immutable name disabled.
export const Copilot: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderCopilot.name}`,
		),
		...seed(MockAIProviderCopilot),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const name = await canvas.findByLabelText(/^name/i);
		expect(name).toBeDisabled();
		expect(canvas.queryByLabelText(/api key/i)).not.toBeInTheDocument();
	},
};

// Claude Platform in iam mode signs requests, so the form shows the AWS
// credential inputs and no workspace key.
export const ClaudePlatformIam: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderClaudePlatformAWS.name}`,
		),
		...seed(MockAIProviderClaudePlatformAWS),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("radio", { name: /claude platform for aws/i }),
		).toBeChecked();
		expect(canvas.getByRole("radio", { name: /aws iam/i })).toBeChecked();
		expect(canvas.getByLabelText(/workspace id/i)).toHaveValue("wrkspc_123");
		expect(
			canvas.queryByRole("textbox", { name: /workspace api key/i }),
		).not.toBeInTheDocument();
	},
};

// In api_key mode the workspace key comes from the provider's api_keys, so it
// is seeded with the masked value the API returned.
export const ClaudePlatformWorkspaceKey: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderClaudePlatformAWSAPIKey.name}`,
		),
		...seed(MockAIProviderClaudePlatformAWSAPIKey),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("radio", { name: /workspace api key/i }),
		).toBeChecked();
		expect(
			canvas.getByRole("textbox", { name: /workspace api key/i }),
		).toHaveValue(MockAIProviderClaudePlatformAWSAPIKey.api_keys[0].masked);
		expect(
			canvas.queryByRole("textbox", { name: /^access key$/i }),
		).not.toBeInTheDocument();
	},
};

// No seeded query: the page renders the loader while useQuery fetches.
export const Loading: Story = {
	parameters: {
		reactRouter: routingFor("/ai/settings/providers/loading-provider"),
	},
};

export const DeleteDialogOpen: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
		),
		...seed(MockAIProviderOpenAI),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const deleteButton = await canvas.findByRole("button", {
			name: /^delete$/i,
		});
		await userEvent.click(deleteButton);
		// DeleteDialog renders via Radix portal, so search the document, not
		// just the story canvas.
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
		await expect(await screen.findByText(/irreversible/i)).toBeInTheDocument();
	},
};
