import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import { TooltipProvider } from "components/Tooltip/Tooltip";
import { expect, fn, userEvent, within } from "storybook/test";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelsSection } from "./ModelsSection";

const providerState: ProviderState = {
	provider: "openai",
	label: "OpenAI",
	providerConfig: {
		id: "provider-config-id",
		provider: "openai",
		display_name: "OpenAI",
		enabled: true,
		has_api_key: true,
		base_url: undefined,
		source: "database",
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
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
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-01-01T00:00:00Z",
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
		const body = within(canvasElement.ownerDocument.body);
		const canvas = within(canvasElement);

		// The warning icon should be visible in the subtitle line.
		const warningIcon =
			canvas.getByRole("img", { hidden: true }) ||
			canvas.container.querySelector(".text-content-warning");
		expect(warningIcon).toBeTruthy();

		// Hovering the icon reveals the tooltip text.
		const trigger = canvas.container.querySelector(".text-content-warning");
		if (trigger) {
			await userEvent.hover(trigger);
			await expect(
				await body.findByText("Model pricing is not defined"),
			).toBeInTheDocument();
		}
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
		// No warning icon should be present.
		const warningIcon = canvas.container.querySelector(".text-content-warning");
		expect(warningIcon).toBeNull();
	},
};
