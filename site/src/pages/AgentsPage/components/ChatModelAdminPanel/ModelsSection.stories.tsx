import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelsSection } from "./ModelsSection";
import {
	createModelProviderAttachments,
	createOpenAIModelConfig,
	createOpenAIProviderConfig,
	type StoryFixtureOptions,
} from "./storyFixtures";

const now = "2025-01-01T00:00:00Z";
const storyFixtureOptions: StoryFixtureOptions = { now };

const primaryProviderConfig = createOpenAIProviderConfig(
	"d889b26b-9d4e-4e1b-94de-d9a4f625bbf7",
	"OpenAI Primary",
	{
		has_api_key: true,
		has_effective_api_key: true,
		base_url: "https://api.openai.com/v1",
	},
	storyFixtureOptions,
);

const fallbackProviderConfig = createOpenAIProviderConfig(
	"e03c44a3-91d0-4f08-8a95-14a0268cb2d5",
	"OpenAI Fallback",
	{
		has_api_key: true,
		has_effective_api_key: true,
		base_url: "https://fallback.openai.example.com/v1",
	},
	storyFixtureOptions,
);

const sandboxProviderConfig = createOpenAIProviderConfig(
	"a19ad8d4-35ad-4e47-8243-5f7f14cc57f8",
	"OpenAI Sandbox",
	{
		has_api_key: false,
		has_effective_api_key: false,
		allow_user_api_key: true,
		base_url: "https://sandbox.openai.example.com/v1",
	},
	storyFixtureOptions,
);

const providerState: ProviderState = {
	provider: "openai",
	label: "OpenAI",
	providerConfig: primaryProviderConfig,
	providerConfigs: [
		primaryProviderConfig,
		fallbackProviderConfig,
		sandboxProviderConfig,
	],
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: true,
	hasCatalogAPIKey: true,
	hasEffectiveAPIKey: true,
	isEnvPreset: false,
	baseURL: primaryProviderConfig.base_url ?? "",
};

const baseModelConfig = createOpenAIModelConfig(
	"f3f8f726-3a3f-4b85-bf5d-4ba4d427e5fe",
	"gpt-4.1",
	{
		display_name: "GPT-4.1",
		context_limit: 128000,
		compression_threshold: 80,
	},
	storyFixtureOptions,
);

const multiAttachmentModelConfig: TypesGen.ChatModelConfig = {
	...baseModelConfig,
	id: "a7b0e7f6-1cd6-4472-b1f7-6457f9f0b9d8",
	display_name: "GPT-4.1 Router",
	provider_configs: createModelProviderAttachments([
		sandboxProviderConfig,
		fallbackProviderConfig,
		primaryProviderConfig,
	]),
};

const meta: Meta<typeof ModelsSection> = {
	title: "pages/AgentsPage/ChatModelAdminPanel/ModelsSection",
	component: ModelsSection,
	args: {
		sectionLabel: "Models",
		providerStates: [providerState],
		selectedProvider: "openai",
		selectedProviderState: providerState,
		onSelectedProviderChange: fn(),
		modelConfigs: [baseModelConfig],
		modelConfigsUnavailable: false,
		isCreating: false,
		isUpdating: false,
		isDeleting: false,
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
		onDeleteModel: fn(async () => undefined),
	},
	decorators: [
		(Story) => (
			<TooltipProvider>
				<Story />
			</TooltipProvider>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ModelsSection>;

export const ShowsPricingWarning: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Model pricing is not defined"),
		).toBeInTheDocument();
	},
};

export const HidesPricingWarningForExplicitZeroPricing: Story = {
	args: {
		modelConfigs: [
			{
				...baseModelConfig,
				id: "5304021d-6d9b-4d6a-a6f4-9fb504ea4a75",
				model_config: {
					cost: {
						output_price_per_million_tokens: "0",
					},
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByText("Model pricing is not defined"),
		).not.toBeInTheDocument();
	},
};

export const MultipleAttachments: Story = {
	args: {
		modelConfigs: [multiAttachmentModelConfig],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("OpenAI Sandbox (+2 more)"),
		).toBeInTheDocument();
	},
};
