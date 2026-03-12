import { render, screen } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { TooltipProvider } from "components/Tooltip/Tooltip";
import { describe, expect, it, vi } from "vitest";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelsSection } from "./ModelsSection";

vi.mock("./ProviderIcon", () => ({
	ProviderIcon: ({ provider }: { provider: string }) => (
		<div data-testid="provider-icon">{provider}</div>
	),
}));

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

const renderModelsSection = (
	modelConfigs: readonly TypesGen.ChatModelConfig[],
) => {
	return render(
		<TooltipProvider>
			<ModelsSection
				sectionLabel="Models"
				providerStates={[providerState]}
				selectedProvider="openai"
				selectedProviderState={providerState}
				onSelectedProviderChange={vi.fn()}
				modelConfigs={modelConfigs}
				modelConfigsUnavailable={false}
				isCreating={false}
				isUpdating={false}
				isDeleting={false}
				onCreateModel={vi.fn()}
				onUpdateModel={vi.fn()}
				onDeleteModel={vi.fn()}
			/>
		</TooltipProvider>,
	);
};

describe("ModelsSection", () => {
	it("shows a warning when a model has no custom pricing configured", () => {
		renderModelsSection([baseModelConfig]);

		expect(
			screen.getByText("Model pricing is not defined"),
		).toBeInTheDocument();
	});

	it("hides the warning when a model has explicit zero pricing", () => {
		renderModelsSection([
			{
				...baseModelConfig,
				model_config: {
					cost: {
						output_price_per_million_tokens: 0,
					},
				},
			},
		]);

		expect(
			screen.queryByText("Model pricing is not defined"),
		).not.toBeInTheDocument();
	});
});
