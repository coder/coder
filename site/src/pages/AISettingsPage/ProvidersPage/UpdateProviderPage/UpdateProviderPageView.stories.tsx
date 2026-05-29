import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { chatModelConfigs } from "#/api/queries/chats";
import type { AIProvider, ChatModelConfig } from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import UpdateProviderPageView from "./UpdateProviderPageView";

const routingFor = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: "/ai/settings", useStoryElement: true },
			{ path: "/ai/settings/:providerId", useStoryElement: true },
		],
	});

const model = (
	overrides: Partial<ChatModelConfig> &
		Pick<ChatModelConfig, "id" | "provider" | "model">,
): ChatModelConfig => ({
	id: overrides.id,
	provider: overrides.provider,
	ai_provider_id: overrides.ai_provider_id,
	model: overrides.model,
	display_name: overrides.display_name ?? overrides.model,
	enabled: overrides.enabled ?? true,
	is_default: overrides.is_default ?? false,
	context_limit: overrides.context_limit ?? 200000,
	compression_threshold: overrides.compression_threshold ?? 70,
	model_config: overrides.model_config,
	created_at: overrides.created_at ?? "2026-05-14T10:00:00Z",
	updated_at: overrides.updated_at ?? "2026-05-14T10:00:00Z",
});

const seed = (
	provider: AIProvider,
	models: readonly ChatModelConfig[] = [],
) => ({
	queries: [
		{ key: ["ai", "providers", provider.name], data: provider },
		{ key: chatModelConfigs().queryKey, data: models },
	],
});

const openAIAssociatedModels = [
	model({
		id: "model-openai-default",
		provider: "openai",
		ai_provider_id: MockAIProviderOpenAI.id,
		model: "gpt-4o",
		display_name: "GPT-4o",
		is_default: true,
	}),
	model({
		id: "model-openai-secondary",
		provider: "openai",
		ai_provider_id: MockAIProviderOpenAI.id,
		model: "gpt-4o-mini",
		display_name: "GPT-4o Mini",
	}),
] satisfies readonly ChatModelConfig[];

const anthropicFallbackModel = model({
	id: "model-anthropic-fallback",
	provider: "anthropic",
	ai_provider_id: MockAIProviderAnthropic.id,
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
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
		reactRouter: routingFor(`/ai/settings/${MockAIProviderOpenAI.name}`),
		...seed(MockAIProviderOpenAI),
	},
};

export const Anthropic: Story = {
	parameters: {
		reactRouter: routingFor(`/ai/settings/${MockAIProviderAnthropic.name}`),
		...seed(MockAIProviderAnthropic),
	},
};

export const Bedrock: Story = {
	parameters: {
		reactRouter: routingFor(`/ai/settings/${MockAIProviderBedrock.name}`),
		...seed(MockAIProviderBedrock),
	},
};

// No seeded query: the page renders the loader while useQuery fetches.
export const Loading: Story = {
	parameters: {
		reactRouter: routingFor("/ai/settings/loading-provider"),
	},
};

export const DeleteDialogOpen: Story = {
	parameters: {
		reactRouter: routingFor(`/ai/settings/${MockAIProviderOpenAI.name}`),
		...seed(MockAIProviderOpenAI),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const deleteButton = await canvas.findByRole("button", {
			name: /^delete$/i,
		});
		await userEvent.click(deleteButton);
		// The dialog renders via Radix portal, so search the document, not
		// just the story canvas.
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
		await expect(await screen.findByText(/irreversible/i)).toBeInTheDocument();
	},
};

export const DeleteDialogWithAssociatedModels: Story = {
	parameters: {
		reactRouter: routingFor(`/ai/settings/${MockAIProviderOpenAI.name}`),
		...seed(MockAIProviderOpenAI, openAIAssociatedModels),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /^delete$/i }),
		);

		await expect(
			await screen.findByText(/Deleting this provider will also disable/i),
		).toBeInTheDocument();
		await expect(screen.getByText("2 models")).toBeInTheDocument();
		await expect(
			screen.getByText(
				"Your default model will be disabled. No other model is available to become the default.",
			),
		).toBeInTheDocument();
	},
};

export const DeleteDialogCascadeConfirmed: Story = {
	parameters: {
		reactRouter: routingFor(`/ai/settings/${MockAIProviderOpenAI.name}`),
		...seed(MockAIProviderOpenAI, [
			...openAIAssociatedModels,
			anthropicFallbackModel,
		]),
	},
	beforeEach: () => {
		spyOn(API.experimental, "updateChatModelConfig").mockResolvedValue(
			anthropicFallbackModel,
		);
		spyOn(API, "deleteAIProvider").mockResolvedValue(undefined);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /^delete$/i }),
		);
		await userEvent.type(
			await screen.findByLabelText(
				`Type ${MockAIProviderOpenAI.name} to confirm`,
			),
			MockAIProviderOpenAI.name,
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Delete provider" }),
		);

		await waitFor(() => {
			expect(API.experimental.updateChatModelConfig).toHaveBeenCalledTimes(3);
		});
		expect(API.experimental.updateChatModelConfig).toHaveBeenNthCalledWith(
			1,
			"model-anthropic-fallback",
			{ is_default: true },
		);
		expect(API.experimental.updateChatModelConfig).toHaveBeenNthCalledWith(
			2,
			"model-openai-default",
			{ enabled: false },
		);
		expect(API.experimental.updateChatModelConfig).toHaveBeenNthCalledWith(
			3,
			"model-openai-secondary",
			{ enabled: false },
		);
		await waitFor(() => {
			expect(API.deleteAIProvider).toHaveBeenCalledWith(
				MockAIProviderOpenAI.name,
			);
		});
	},
};

export const DeleteDialogCascadeFailure: Story = {
	parameters: {
		reactRouter: routingFor(`/ai/settings/${MockAIProviderOpenAI.name}`),
		...seed(MockAIProviderOpenAI, [
			...openAIAssociatedModels,
			anthropicFallbackModel,
		]),
	},
	beforeEach: () => {
		spyOn(API.experimental, "updateChatModelConfig")
			.mockResolvedValueOnce(anthropicFallbackModel)
			.mockRejectedValueOnce(new Error("Failed to disable model."));
		spyOn(API, "deleteAIProvider").mockResolvedValue(undefined);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /^delete$/i }),
		);
		await userEvent.type(
			await screen.findByLabelText(
				`Type ${MockAIProviderOpenAI.name} to confirm`,
			),
			MockAIProviderOpenAI.name,
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Delete provider" }),
		);

		await waitFor(() => {
			expect(API.experimental.updateChatModelConfig).toHaveBeenCalledTimes(2);
		});
		expect(API.deleteAIProvider).not.toHaveBeenCalled();
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
		await expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();
		await expect(
			await screen.findByText("Failed to disable model."),
		).toBeInTheDocument();
	},
};
