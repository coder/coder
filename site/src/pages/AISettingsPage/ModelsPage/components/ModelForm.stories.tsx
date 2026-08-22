import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fn,
	mocked,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { aiModelPrices } from "#/api/queries/aiProviders";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockGPT5BelowThresholdModelPrice,
	MockGPT5ModelPrice,
} from "#/testHelpers/chatModels";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockAzureProviderState,
	MockDisabledProviderState,
	MockGoogleProviderState,
	MockOpenAIProviderState,
	mockClaude,
	mockGPT5,
	mockProviderDisabledModel,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

const onUpdateModel = fn(
	async (
		_modelConfigId: string,
		_req: TypesGen.UpdateChatModelConfigRequest,
	): Promise<unknown> => undefined,
);

// The price loading placeholder is transient: it disappears once the
// debounce settles and the lookup resolves, so poll for it instead of
// asserting synchronously.
const waitForPriceLoading = async (canvas: ReturnType<typeof within>) => {
	await waitFor(() =>
		expect(canvas.getByLabelText("Input price loading")).toBeInTheDocument(),
	);
};

const meta: Meta<typeof ModelForm> = {
	title: "pages/AISettingsPage/ModelsPage/ModelForm",
	component: ModelForm,
	decorators: [withToaster, withDashboardProvider],
	args: {
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		onProviderChange: fn(),
		isSaving: false,
		isDeleting: false,
		onCreateModel: fn(async () => undefined),
		onUpdateModel,
	},
	parameters: {
		features: ["aibridge"],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/models/add" },
			routing: [
				{ path: "/ai/settings/models/add", useStoryElement: true },
				{ path: "/ai/settings/models", element: <div>Models</div> },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ModelForm>;

export const Add: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: /add an? OpenAI model/i }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("checkbox", {
				name: /set as coder agents default model/i,
			}),
		).toBeInTheDocument();
		const submit = canvas.getByRole("button", { name: /add model/i });
		await expect(submit).toBeDisabled();
	},
};

export const AddValidSubmit: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const modelInput = canvas.getByLabelText(/model identifier/i);
		await userEvent.type(modelInput, "gpt-5");
		const contextLimit = canvas.getByLabelText(/context limit/i);
		await userEvent.type(contextLimit, "200000");
		const submit = canvas.getByRole("button", { name: /add model/i });
		await expect(submit).toBeEnabled();
		await userEvent.click(submit);
		await expect(args.onCreateModel).toHaveBeenCalledTimes(1);
	},
};

export const AddSetAsDefault: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/model identifier/i), "gpt-5");
		await userEvent.type(canvas.getByLabelText(/context limit/i), "200000");
		await userEvent.click(
			canvas.getByRole("checkbox", {
				name: /set as coder agents default model/i,
			}),
		);
		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		await expect(args.onCreateModel).toHaveBeenCalledTimes(1);
		await expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({ is_default: true }),
		);
	},
};

export const LeaveWithUnsavedChanges: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/model identifier/i), "gpt-5");
		await userEvent.click(
			canvas.getByRole("link", { name: /back to models/i }),
		);
		const dialog = await screen.findByRole("dialog", {
			name: /unsaved changes/i,
		});
		await expect(dialog).toBeInTheDocument();
	},
};

export const ReplaceDefaultWarning: Story = {
	args: {
		currentDefaultModel: { ...mockGPT5, is_default: true },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByLabelText(/model identifier/i),
			"gpt-5-mini",
		);
		await userEvent.type(canvas.getByLabelText(/context limit/i), "200000");
		await userEvent.click(
			canvas.getByRole("checkbox", {
				name: /set as coder agents default model/i,
			}),
		);
		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		await expect(
			await screen.findByText(/replace default model/i),
		).toBeInTheDocument();
		await expect(args.onCreateModel).not.toHaveBeenCalled();
		await userEvent.click(screen.getByRole("button", { name: /^confirm$/i }));
		await expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({ is_default: true }),
		);
	},
};

export const AddHidesDisabledProviders: Story = {
	args: {
		providerStates: [
			MockOpenAIProviderState,
			MockAnthropicProviderState,
			MockDisabledProviderState,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("combobox", { name: /provider/i }));
		// Exact accessible-name matches guard the aria-hidden icon fix: a
		// regressed icon would turn an option's name into "OpenAI OpenAI".
		await screen.findByRole("option", { name: "OpenAI" });
		await screen.findByRole("option", { name: "Anthropic" });
		await expect(screen.getAllByRole("option")).toHaveLength(2);
		await expect(
			screen.queryByRole("option", { name: /Secondary/ }),
		).not.toBeInTheDocument();
	},
};

export const AddBlocksDisabledSelectedProvider: Story = {
	args: {
		providerStates: [MockOpenAIProviderState, MockDisabledProviderState],
		selectedProviderState: MockDisabledProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A ?provider= query param can preselect a disabled provider on
		// the add page.
		await expect(
			canvas.getByText(/OpenAI Secondary is disabled/),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: /add model/i }),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("combobox", { name: /provider/i }));
		await expect(
			screen.queryByRole("option", { name: /Secondary/ }),
		).not.toBeInTheDocument();
	},
};

export const EditKeepsDisabledProviderVisible: Story = {
	args: {
		providerStates: [MockOpenAIProviderState, MockDisabledProviderState],
		selectedProviderState: MockDisabledProviderState,
		editingModel: mockProviderDisabledModel,
		onDeleteModel: fn(async () => undefined),
		onDuplicate: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: /provider/i }),
		).toHaveTextContent("OpenAI Secondary");
	},
};

export const Edit: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
		onDuplicate: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: /^update model$/i }),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /^cancel$/i }),
		).toBeVisible();
		await expect(
			canvas.getByRole("checkbox", {
				name: /set as coder agents default model/i,
			}),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText(/model identifier/i)).toBeEnabled();
		await expect(
			canvas.getByRole("combobox", { name: /provider/i }),
		).toBeEnabled();
		await userEvent.click(
			canvas.getByRole("button", { name: /model actions/i }),
		);
		await expect(
			screen.getByRole("menuitem", { name: /duplicate model/i }),
		).toBeInTheDocument();
		await expect(
			screen.getByRole("menuitem", { name: /delete/i }),
		).toBeInTheDocument();
	},
};

export const EditDefaultBadge: Story = {
	args: {
		editingModel: { ...mockGPT5, is_default: true, enabled: true },
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/^default$/i)).toBeInTheDocument();
	},
};

export const EditDisabledBadge: Story = {
	args: {
		editingModel: { ...mockGPT5, is_default: false, enabled: false },
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/^disabled$/i)).toBeInTheDocument();
	},
};

export const EditSaveSubmits: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const save = canvas.getByRole("button", { name: /^update model$/i });
		// Update is disabled until the user makes a change.
		await expect(save).toBeDisabled();
		await userEvent.type(canvas.getByLabelText(/display name/i), " (updated)");
		await expect(save).toBeEnabled();
		await userEvent.click(save);
		await expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
	},
};

export const EditClearingLastOptionSendsEmptyConfig: Story = {
	args: {
		editingModel: { ...mockGPT5, model_config: { temperature: 1 } },
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /advanced/i }));
		const temperature = canvas.getByLabelText(/temperature/i);
		await expect(temperature).toHaveValue("1");
		await userEvent.clear(temperature);
		await userEvent.click(
			canvas.getByRole("button", { name: /^update model$/i }),
		);
		// The empty object must be sent explicitly: omitting model_config
		// preserves the stored options server-side.
		await expect(args.onUpdateModel).toHaveBeenCalledWith(
			mockGPT5.id,
			expect.objectContaining({ model_config: {} }),
		);
	},
};

export const EditUpdateDisabledUntilDirty: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const save = canvas.getByRole("button", { name: /^update model$/i });
		await expect(save).toBeDisabled();
		const displayName = canvas.getByLabelText(/display name/i);
		await userEvent.type(displayName, " (edited)");
		await expect(save).toBeEnabled();
		await userEvent.clear(displayName);
		await userEvent.type(displayName, mockGPT5.display_name);
		await expect(save).toBeDisabled();
	},
};

// Changing only the provider dropdown does not dirty the formik state
// (providerKeyOverride lives outside form.values), so a naive form.dirty
// gate leaves the save button disabled. `canSubmit` OR's in
// `hasProviderChange` to fix this. Stripping that clause flips this story
// red.
export const EditUpdateEnabledOnProviderChange: Story = {
	args: {
		editingModel: mockGPT5,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const save = canvas.getByRole("button", { name: /^update model$/i });
		await expect(save).toBeEnabled();
	},
};

export const ReasoningEffortInProviderConfiguration: Story = {
	args: {
		selectedProviderState: MockAnthropicProviderState,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("button", { name: /provider configuration/i }),
		);
		const thinkingBudget = canvas.getByLabelText(/thinking budget tokens/i);
		const maxSelect = canvas.getByRole("combobox", {
			name: /max reasoning effort/i,
		});
		const defaultSelect = canvas.getByRole("combobox", {
			name: /default reasoning effort/i,
		});
		await expect(maxSelect).toBeVisible();
		await expect(defaultSelect).toBeVisible();
		await expect(thinkingBudget.compareDocumentPosition(maxSelect)).toBe(
			Node.DOCUMENT_POSITION_FOLLOWING,
		);
		await expect(maxSelect.compareDocumentPosition(defaultSelect)).toBe(
			Node.DOCUMENT_POSITION_FOLLOWING,
		);
		await expect(defaultSelect).toHaveTextContent("Not set");
		await expect(maxSelect).toHaveTextContent("Not set");

		await userEvent.type(canvas.getByLabelText(/model identifier/i), "gpt-5");
		await userEvent.type(canvas.getByLabelText(/context limit/i), "200000");

		await userEvent.click(defaultSelect);
		for (const option of [
			"None",
			"Minimal",
			"Low",
			"Medium",
			"High",
			"Xhigh",
			"Max",
		]) {
			await expect(
				await screen.findByRole("option", { name: option }),
			).toBeInTheDocument();
		}
		await userEvent.click(
			await screen.findByRole("option", { name: "Medium" }),
		);

		await userEvent.click(maxSelect);
		await userEvent.click(await screen.findByRole("option", { name: "Max" }));

		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		await expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config: expect.objectContaining({
					reasoning_effort: { default: "medium", max: "max" },
				}),
			}),
		);
	},
};

export const ReasoningEffortValidationError: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /provider configuration/i }),
		);
		const defaultSelect = canvas.getByRole("combobox", {
			name: /default reasoning effort/i,
		});
		const maxSelect = canvas.getByRole("combobox", {
			name: /max reasoning effort/i,
		});

		await userEvent.click(defaultSelect);
		await userEvent.click(await screen.findByRole("option", { name: "High" }));
		await userEvent.click(maxSelect);
		await userEvent.click(await screen.findByRole("option", { name: "Low" }));

		await expect(
			canvas.getByText(
				"Default reasoning effort must not exceed the max reasoning effort.",
			),
		).toBeVisible();
	},
};

// thinking_level and thinking_budget are mutually exclusive on Google
// models: setting either one disables the other until it is cleared.
export const GoogleThinkingLevelBudgetMutualExclusion: Story = {
	args: {
		providerStates: [MockGoogleProviderState],
		selectedProviderState: MockGoogleProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /provider configuration/i }),
		);
		const budget = canvas.getByLabelText(/thinking config thinking budget/i);
		const level = canvas.getByRole("combobox", {
			name: /thinking config thinking level/i,
		});
		await expect(budget).toBeEnabled();
		await expect(level).toBeEnabled();

		await userEvent.click(level);
		await userEvent.click(await screen.findByRole("option", { name: "Low" }));
		await expect(budget).toBeDisabled();

		await userEvent.click(level);
		await userEvent.click(
			await screen.findByRole("option", { name: "Default" }),
		);
		await expect(budget).toBeEnabled();

		await userEvent.type(budget, "2048");
		await expect(level).toBeDisabled();

		await userEvent.clear(budget);
		await expect(level).toBeEnabled();
	},
};

// The catalog fallback covers an entitled deployment with no matching price
// book row. Values come from the baked-in catalog and must not be submitted.
export const CostEstimateFieldsAreImmutable: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		queries: [
			{
				key: aiModelPrices("anthropic", "claude-sonnet-4-5").queryKey,
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(
			canvas.getByText(/model prices are managed by AI Gateway/i),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", {
				name: /learn how to configure model prices/i,
			}),
		).toHaveAttribute(
			"href",
			expect.stringContaining(
				"/ai-coder/ai-gateway/cost-controls#configure-model-prices",
			),
		);

		const expectedValues: [RegExp, string][] = [
			[/^input$/i, "3"],
			[/^output$/i, "15"],
			[/cache read/i, "0.30"],
			[/cache write/i, "3.75"],
		];
		for (const [name, value] of expectedValues) {
			const field = canvas.getByLabelText(name);
			await expect(field).toHaveValue(value);
			await expect(field).toHaveAttribute("readonly");
		}

		// Update is disabled until the form is dirty. A display name edit
		// unlocks it so the payload can be checked for pricing keys.
		await userEvent.type(canvas.getByLabelText(/display name/i), " (updated)");
		await userEvent.click(
			canvas.getByRole("button", { name: /^update model$/i }),
		);
		await expect(onUpdateModel).toHaveBeenCalledTimes(1);
		expect(onUpdateModel.mock.calls[0]?.[1]).toStrictEqual({
			display_name: "Claude Sonnet 4.5 (updated)",
			model_config: {},
		});
	},
};

// With the AI Gateway feature entitled, prices come from the live price
// book, so admin overrides and models missing from the catalog are
// reflected. mockGPT5 is openai/gpt-5, absent from the catalog but present
// in the price book. Prices are micro-units per million tokens.
export const CostEstimateFromLivePriceBook: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		queries: [
			{
				key: aiModelPrices("openai", "gpt-5").queryKey,
				data: [MockGPT5ModelPrice],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(canvas.getByLabelText(/^input$/i)).toHaveValue("1.25");
		await expect(canvas.getByLabelText(/^output$/i)).toHaveValue("10");
		await expect(canvas.getByLabelText(/cache read/i)).toHaveValue("0.125");
		await expect(canvas.getByLabelText(/cache write/i)).toHaveValue("");
	},
};

// A live price below $0.0001 per million tokens would render as $0.00 and
// read as free, so the field shows a threshold instead.
export const CostEstimateBelowThresholdPrice: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		queries: [
			{
				key: aiModelPrices("openai", "gpt-5").queryKey,
				data: [MockGPT5BelowThresholdModelPrice],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		const inputField = canvas.getByLabelText(/^input$/i);
		await expect(inputField).toHaveValue("0.0001");
		await expect(canvas.getByText("<$")).toBeInTheDocument();
		// The threshold marker is part of the input's accessible description
		// so screen readers announce "less than $0.0001" rather than an exact
		// price.
		await expect(inputField).toHaveAccessibleDescription(
			"less than $0.0001 USD per million tokens",
		);
	},
};

// Editing the model identifier fires a live price lookup per settled value
// rather than per keystroke, so typing a full identifier costs one request.
export const CostEstimateDebouncesLivePriceLookup: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getAIModelPrices").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		const modelInput = canvas.getByLabelText(/model identifier/i);
		await userEvent.clear(modelInput);
		await userEvent.type(modelInput, "gpt-5.5");
		await waitFor(() =>
			expect(API.experimental.getAIModelPrices).toHaveBeenCalledWith({
				provider: "openai",
				model: "gpt-5.5",
			}),
		);
		const calls = mocked(API.experimental.getAIModelPrices).mock.calls.map(
			([params]) => params.model,
		);
		expect(calls).toStrictEqual(["gpt-5", "gpt-5.5"]);
	},
};

// On an entitled form the live lookup may override the catalog, so
// committing a catalog model shows the loading placeholder immediately
// rather than briefly flashing catalog prices before the lookup fires. In
// add mode the identifier autocomplete only commits the model when an
// option is selected, so select one instead of typing free text.
export const CostEstimateShowsLoadingWhileDebouncePending: Story = {
	args: {
		selectedProviderState: MockAnthropicProviderState,
	},
	beforeEach: () => {
		spyOn(API.experimental, "getAIModelPrices").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await userEvent.type(
			canvas.getByLabelText(/model identifier/i),
			"claude-haiku",
		);
		await userEvent.click(
			await screen.findByRole("option", { name: /claude haiku 4\.5/i }),
		);
		await waitForPriceLoading(canvas);
		// The catalog price must not render while the lookup is pending.
		expect(canvas.queryByDisplayValue("1")).not.toBeInTheDocument();
	},
};

// Clearing the identifier discards the settled live price immediately:
// until the debounce settles the section shows the loading placeholder
// rather than the previous model's prices.
export const CostEstimateClearingModelDiscardsStalePrices: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		queries: [
			{
				key: aiModelPrices("openai", "gpt-5").queryKey,
				data: [MockGPT5ModelPrice],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(canvas.getByLabelText(/^input$/i)).toHaveValue("1.25");

		await userEvent.clear(canvas.getByLabelText(/model identifier/i));
		await waitForPriceLoading(canvas);
		expect(canvas.queryByDisplayValue("1.25")).not.toBeInTheDocument();
	},
};

// A price book row is the deployment's own pricing, so it wins outright. A
// null category on the row means the model is unpriced there and bills as
// zero, so the field stays blank instead of falling back to the catalog.
// The catalog only fills in when the model has no row at all.
export const CostEstimateRowWinsOverCatalog: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		queries: [
			{
				key: aiModelPrices("anthropic", "claude-sonnet-4-5").queryKey,
				data: [
					{
						provider: "anthropic",
						model: "claude-sonnet-4-5",
						input_price: 4000000,
						output_price: null,
						cache_read_price: null,
						cache_write_price: null,
						created_at: "2026-02-18T12:00:00.000Z",
						updated_at: "2026-02-18T12:00:00.000Z",
					},
				],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		// The override sets input to $4. Output and both cache categories are
		// null on the row, so they render blank even though the catalog has
		// values for them.
		await expect(canvas.getByLabelText(/^input$/i)).toHaveValue("4");
		await expect(canvas.getByLabelText(/^output$/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/cache read/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/cache write/i)).toHaveValue("");
	},
};

// Without the AI Gateway entitlement the price endpoint is not queried, so
// a model with no catalog entry gets the empty state.
export const CostEstimateUnavailableForUnknownModel: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		features: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(
			canvas.getByText("No pricing data for this model."),
		).toBeInTheDocument();
		await expect(canvas.queryByLabelText(/^input$/i)).not.toBeInTheDocument();
	},
};

// The entitlement gate means the endpoint is not called at all without the
// AI Gateway feature. A catalog model still shows catalog prices.
export const CostEstimateCatalogOnlyWhenNotEntitled: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		features: [],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getAIModelPrices").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(canvas.getByLabelText(/^input$/i)).toHaveValue("3");
		await expect(canvas.getByLabelText(/^output$/i)).toHaveValue("15");
		expect(API.experimental.getAIModelPrices).not.toHaveBeenCalled();
	},
};

// Without the entitlement the live price query is disabled, so changing the
// model identifier must not flash loading placeholders: catalog prices are
// synchronously available and update immediately.
export const CostEstimateCatalogNotHiddenByDebounce: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	parameters: {
		features: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		const modelInput = canvas.getByLabelText(/model identifier/i);
		await userEvent.clear(modelInput);
		await userEvent.type(modelInput, "claude-haiku-4-5");
		await expect(canvas.getByLabelText(/^input$/i)).toHaveValue("1");
		expect(canvas.queryByLabelText(/price loading/i)).not.toBeInTheDocument();
	},
};

// A failed lookup must not linger while the debounce settles for a new
// model: the section shows the loading placeholder until the new request
// has actually fired.
export const CostEstimateErrorClearsWhileDebouncePending: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getAIModelPrices").mockRejectedValue(
			new Error("request failed"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(
			canvas.getByText("Couldn't load pricing."),
		).toBeInTheDocument();

		const modelInput = canvas.getByLabelText(/model identifier/i);
		await userEvent.type(modelInput, "-20251001");
		await waitForPriceLoading(canvas);
		expect(
			canvas.queryByText("Couldn't load pricing."),
		).not.toBeInTheDocument();
	},
};

// While the price book lookup is in flight, the four boxes stay rendered
// with a loading placeholder in each instead of a catalog price. The
// catalog must not appear because the model may have a deployment override.
export const CostEstimateLoading: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getAIModelPrices").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		// Each box shows a loading placeholder while the lookup is in flight.
		for (const label of ["Input", "Output", "Cache read", "Cache write"]) {
			await expect(
				canvas.getByLabelText(`${label} price loading`),
			).toBeInTheDocument();
		}
		// The catalog numbers must not render while the lookup is pending.
		expect(canvas.queryByDisplayValue("3")).not.toBeInTheDocument();
	},
};

// When the price book lookup fails, the section says so instead of falling
// back to the catalog, because the model may have a deployment override.
export const CostEstimateError: Story = {
	args: {
		editingModel: mockClaude,
		selectedProviderState: MockAnthropicProviderState,
		onDeleteModel: fn(async () => undefined),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getAIModelPrices").mockRejectedValue(
			new Error("request failed"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /cost estimate/i }),
		);
		await expect(
			canvas.getByText("Couldn't load pricing."),
		).toBeInTheDocument();
		// The catalog numbers must not render on error.
		expect(canvas.queryByDisplayValue("3")).not.toBeInTheDocument();
		expect(canvas.queryByDisplayValue("15")).not.toBeInTheDocument();
	},
};

export const UseResponsesAPIForOpenAI: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/model identifier/i), "gpt-5");
		await userEvent.type(canvas.getByLabelText(/context limit/i), "200000");
		await userEvent.click(canvas.getByRole("button", { name: /advanced/i }));

		const toggle = canvas.getByRole("radiogroup", {
			name: /use responses api/i,
		});
		await userEvent.click(within(toggle).getByRole("radio", { name: "On" }));
		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		await expect(args.onCreateModel).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config: expect.objectContaining({
					openai_config: { use_responses_api: true },
				}),
			}),
		);
	},
};

export const UseResponsesAPIHiddenForAnthropic: Story = {
	args: {
		selectedProviderState: MockAnthropicProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /advanced/i }));
		await expect(
			canvas.queryByRole("radiogroup", { name: /use responses api/i }),
		).not.toBeInTheDocument();
	},
};

// Azure resolves to the OpenAI option schema through provider_aliases, so
// scoping has to gate on the raw provider type to keep this control out.
export const UseResponsesAPIHiddenForAzure: Story = {
	args: {
		providerStates: [MockOpenAIProviderState, MockAzureProviderState],
		selectedProviderState: MockAzureProviderState,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /advanced/i }));
		await expect(
			canvas.queryByRole("radiogroup", { name: /use responses api/i }),
		).not.toBeInTheDocument();
		await expect(canvas.getByLabelText(/temperature/i)).toBeInTheDocument();
	},
};
