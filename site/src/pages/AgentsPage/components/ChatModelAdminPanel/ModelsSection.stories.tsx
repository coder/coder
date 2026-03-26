import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelsSection } from "./ModelsSection";

const now = "2025-01-01T00:00:00Z";

const createProviderConfig = (
	overrides: Partial<TypesGen.ChatProviderConfig> &
		Pick<TypesGen.ChatProviderConfig, "id" | "provider">,
): TypesGen.ChatProviderConfig => ({
	id: overrides.id,
	provider: overrides.provider,
	display_name: overrides.display_name ?? "",
	enabled: overrides.enabled ?? true,
	has_api_key: overrides.has_api_key ?? false,
	base_url: overrides.base_url,
	source: overrides.source ?? "database",
	created_at: overrides.created_at ?? now,
	updated_at: overrides.updated_at ?? now,
});

const providerState: ProviderState = {
	provider: "openai",
	label: "OpenAI",
	providerConfigs: [
		createProviderConfig({
			id: "provider-config-id",
			provider: "openai",
			display_name: "OpenAI",
			enabled: true,
			has_api_key: true,
			source: "database",
		}),
	],
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: true,
	hasCatalogAPIKey: true,
	hasEffectiveAPIKey: true,
	isEnvPreset: false,
	baseURL: "",
};

const baseModelConfig: TypesGen.ChatModelConfig = {
	id: "model-config-id",
	provider: "openai",
	model: "gpt-4.1",
	display_name: "GPT-4.1",
	enabled: true,
	is_default: false,
	context_limit: 128000,
	compression_threshold: 80,
	created_at: now,
	updated_at: now,
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
		selectedModelOptionKey: null,
		onSelectedModelOptionChange: fn(),
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
				id: "model-config-id-zero-pricing",
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

const openAIMultiConfigProviderState: ProviderState = {
	provider: "openai",
	label: "OpenAI",
	providerConfigs: [
		createProviderConfig({
			id: "openai-prod-id",
			provider: "openai",
			display_name: "OpenAI Production",
			enabled: true,
			has_api_key: true,
			source: "database",
		}),
		createProviderConfig({
			id: "openai-unnamed-id",
			provider: "openai",
			display_name: "",
			enabled: true,
			has_api_key: true,
			source: "database",
		}),
	],
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: true,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: true,
	isEnvPreset: false,
	baseURL: "",
};

const anthropicEnvPresetProviderState: ProviderState = {
	provider: "anthropic",
	label: "Anthropic",
	providerConfigs: [
		createProviderConfig({
			id: "00000000-0000-0000-0000-000000000000",
			provider: "anthropic",
			enabled: true,
			has_api_key: true,
			source: "env_preset",
		}),
	],
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: false,
	hasCatalogAPIKey: true,
	hasEffectiveAPIKey: true,
	isEnvPreset: true,
	baseURL: "",
};

const googleExcludedProviderState: ProviderState = {
	provider: "google",
	label: "Google",
	providerConfigs: [
		createProviderConfig({
			id: "google-no-key-id",
			provider: "google",
			enabled: true,
			has_api_key: false,
			source: "database",
		}),
	],
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: false,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: false,
	isEnvPreset: false,
	baseURL: "",
};

export const MultiConfigAddDropdown: Story = {
	args: {
		providerStates: [
			openAIMultiConfigProviderState,
			anthropicEnvPresetProviderState,
			googleExcludedProviderState,
		],
		modelConfigs: [],
		selectedProvider: null,
		selectedProviderState: null,
		selectedModelOptionKey: null,
		onSelectedModelOptionChange: fn(),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		const trigger = await body.findByRole("button", { name: "Add model" });
		await userEvent.click(trigger);

		await waitFor(() => {
			expect(
				body.getByRole("menuitem", { name: /OpenAI Production/i }),
			).toBeInTheDocument();
			expect(
				body.getByRole("menuitem", { name: /^OpenAI 2$/i }),
			).toBeInTheDocument();
			expect(
				body.getByRole("menuitem", { name: /Anthropic/i }),
			).toBeInTheDocument();
		});

		const items = body.getAllByRole("menuitem");
		expect(items).toHaveLength(3);
	},
};
