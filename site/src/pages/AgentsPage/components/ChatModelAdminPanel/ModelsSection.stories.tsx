import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
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
		central_api_key_enabled: true,
		allow_user_api_key: false,
		allow_central_api_key_fallback: false,
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

const providerStateWithoutAPIKey: ProviderState = {
	...providerState,
	providerConfig: {
		...providerState.providerConfig!,
		has_api_key: false,
		central_api_key_enabled: false,
		allow_user_api_key: false,
	},
	hasManagedAPIKey: false,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: false,
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

const disabledModelConfig: TypesGen.ChatModelConfig = {
	...baseModelConfig,
	id: "disabled-model-config-id",
	model: "gpt-4.1-disabled",
	display_name: "GPT-4.1 Disabled",
	enabled: false,
};

const defaultModelConfig: TypesGen.ChatModelConfig = {
	...baseModelConfig,
	id: "default-model-config-id",
	model: "gpt-4o",
	display_name: "GPT-4o",
	is_default: true,
};

const duplicateSourceModel: TypesGen.ChatModelConfig = {
	...baseModelConfig,
	id: "duplicate-source-model-id",
	model: "gpt-4.1-default",
	display_name: "GPT-4.1 Default",
	is_default: true,
	context_limit: 200000,
	compression_threshold: 65,
	model_config: {
		max_output_tokens: 4096,
		provider_options: {
			openai: {
				max_tool_calls: 4,
				reasoning_effort: "high",
			},
		},
	},
};

const meta: Meta<typeof ModelsSection> = {
	title: "pages/AgentsPage/ChatModelAdminPanel/ModelsSection",
	component: ModelsSection,
	args: {
		sectionLabel: "Models",
		providerStates: [providerState],
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

export const LinksToProvidersFromEmptyState: Story = {
	args: {
		providerStates: [providerStateWithoutAPIKey],
		modelConfigs: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const providerLink = canvas.getByRole("link", { name: /provider/i });

		await expect(canvas.getByText("No models configured yet.")).toBeVisible();
		await expect(providerLink).toBeVisible();
		expect(providerLink).toHaveAttribute("href", "/agents/settings/providers");
	},
};

export const ShowsExplicitRowActions: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const rowButton = canvas.getByRole("button", {
			name: "Open model: GPT-4.1",
		});
		const starButton = canvas.getByRole("button", {
			name: "Set as default model: GPT-4.1",
		});
		const editButton = canvas.getByRole("button", {
			name: "Edit model: GPT-4.1",
		});
		const copyButton = canvas.getByRole("button", {
			name: "Duplicate model: GPT-4.1",
		});

		await expect(starButton).toBeVisible();
		await expect(editButton).toBeVisible();
		await expect(copyButton).toBeVisible();
		rowButton.focus();
		await expect(rowButton).toHaveFocus();
		await user.tab();
		await expect(starButton).toHaveFocus();
		await user.tab();
		await expect(editButton).toHaveFocus();
		await user.tab();
		await expect(copyButton).toHaveFocus();
	},
};

export const OpensDuplicateFormWithoutCreating: Story = {
	args: {
		modelConfigs: [duplicateSourceModel],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: "Duplicate model: GPT-4.1 Default",
			}),
		);

		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();
		expect(args.onCreateModel).not.toHaveBeenCalled();
		expect(args.onUpdateModel).not.toHaveBeenCalled();
		expect(canvas.getByDisplayValue("GPT-4.1 Default")).toBeVisible();
		expect(canvas.getByLabelText(/Model Identifier/)).toHaveValue(
			"gpt-4.1-default",
		);
		expect(canvas.getByLabelText(/Context Limit/)).toHaveValue("200000");
		const enabledSwitch = canvas.getByRole("switch", { name: "Enabled" });
		expect(enabledSwitch).toBeChecked();
		expect(enabledSwitch).toBeEnabled();

		await userEvent.click(canvas.getByRole("button", { name: /Advanced/ }));
		expect(canvas.getByLabelText(/Compression Threshold/)).toHaveValue("65");

		await userEvent.click(
			canvas.getByRole("button", { name: /Provider Configuration/ }),
		);
		expect(canvas.getByLabelText("Max Tool Calls")).toHaveValue("4");
	},
};

export const AbandonsDuplicateWithoutSaving: Story = {
	args: {
		modelConfigs: [duplicateSourceModel],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const copyButtonName = "Duplicate model: GPT-4.1 Default";

		await userEvent.click(canvas.getByRole("button", { name: copyButtonName }));
		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Cancel" }));
		await expect(
			canvas.findByRole("button", { name: copyButtonName }),
		).resolves.toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: copyButtonName }));
		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Back" }));
		await expect(
			canvas.findByRole("button", { name: copyButtonName }),
		).resolves.toBeVisible();
		expect(args.onCreateModel).not.toHaveBeenCalled();
		expect(args.onUpdateModel).not.toHaveBeenCalled();
	},
};

export const SavesDuplicateAsCreateRequest: Story = {
	args: {
		modelConfigs: [duplicateSourceModel],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: "Duplicate model: GPT-4.1 Default",
			}),
		);
		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();

		const modelInput = canvas.getByLabelText(/Model Identifier/);
		await userEvent.clear(modelInput);
		await userEvent.type(modelInput, "gpt-4.1-copy");
		const displayNameInput = canvas.getByDisplayValue("GPT-4.1 Default");
		await userEvent.clear(displayNameInput);
		await userEvent.type(displayNameInput, "GPT-4.1 Copy");
		await userEvent.click(
			canvas.getByRole("button", { name: "Create duplicate" }),
		);

		await waitFor(() => expect(args.onCreateModel).toHaveBeenCalledTimes(1));
		expect(args.onUpdateModel).not.toHaveBeenCalled();

		const createModelMock = args.onCreateModel as ReturnType<typeof fn>;
		const createReq = createModelMock.mock.calls[0]?.[0];
		if (!createReq) {
			throw new Error("Expected create request.");
		}
		expect(createReq).toEqual({
			provider: "openai",
			model: "gpt-4.1-copy",
			display_name: "GPT-4.1 Copy",
			enabled: true,
			is_default: false,
			context_limit: 200000,
			compression_threshold: 65,
			model_config: {
				max_output_tokens: 4096,
				provider_options: {
					openai: {
						max_tool_calls: 4,
						reasoning_effort: "high",
					},
				},
			},
		});
	},
};

export const SavesNonDefaultDuplicateWithEditableEnabled: Story = {
	args: {
		modelConfigs: [baseModelConfig],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Duplicate model: GPT-4.1" }),
		);
		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();

		const enabledSwitch = canvas.getByRole("switch", { name: "Enabled" });
		expect(enabledSwitch).toBeChecked();
		expect(enabledSwitch).toBeEnabled();
		await userEvent.click(enabledSwitch);

		const modelInput = canvas.getByLabelText(/Model Identifier/);
		await userEvent.clear(modelInput);
		await userEvent.type(modelInput, "gpt-4.1-copy");
		await userEvent.click(
			canvas.getByRole("button", { name: "Create duplicate" }),
		);

		await waitFor(() => expect(args.onCreateModel).toHaveBeenCalledTimes(1));
		expect(args.onUpdateModel).not.toHaveBeenCalled();

		const createModelMock = args.onCreateModel as ReturnType<typeof fn>;
		expect(createModelMock.mock.calls[0]?.[0]).toEqual({
			provider: "openai",
			model: "gpt-4.1-copy",
			display_name: "GPT-4.1",
			enabled: false,
			is_default: false,
			context_limit: 128000,
			compression_threshold: 80,
		});
	},
};

export const SavesDisabledDuplicateWithEditableEnabled: Story = {
	args: {
		modelConfigs: [disabledModelConfig],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: "Duplicate model: GPT-4.1 Disabled",
			}),
		);
		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();

		const enabledSwitch = canvas.getByRole("switch", { name: "Enabled" });
		expect(enabledSwitch).not.toBeChecked();
		expect(enabledSwitch).toBeEnabled();
		await userEvent.click(enabledSwitch);

		const modelInput = canvas.getByLabelText(/Model Identifier/);
		await userEvent.clear(modelInput);
		await userEvent.type(modelInput, "gpt-4.1-disabled-copy");
		await userEvent.click(
			canvas.getByRole("button", { name: "Create duplicate" }),
		);

		await waitFor(() => expect(args.onCreateModel).toHaveBeenCalledTimes(1));
		const createModelMock = args.onCreateModel as ReturnType<typeof fn>;
		expect(createModelMock.mock.calls[0]?.[0]).toEqual({
			provider: "openai",
			model: "gpt-4.1-disabled-copy",
			display_name: "GPT-4.1 Disabled",
			enabled: true,
			is_default: false,
			context_limit: 128000,
			compression_threshold: 80,
		});
	},
};

export const DisablesDuplicateWhenProviderCannotManageModels: Story = {
	args: {
		providerStates: [providerStateWithoutAPIKey],
		modelConfigs: [baseModelConfig],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const duplicateButton = canvas.getByRole("button", {
			name: "Duplicate model: GPT-4.1",
		});

		expect(duplicateButton).toHaveAttribute("aria-disabled", "true");
		await userEvent.click(duplicateButton);
		expect(canvas.queryByText("Duplicate Model")).not.toBeInTheDocument();
		expect(args.onCreateModel).not.toHaveBeenCalled();
	},
};

export const RowActionsDoNotOpenRowBody: Story = {
	args: {
		modelConfigs: [baseModelConfig, defaultModelConfig, disabledModelConfig],
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const updateModelMock = args.onUpdateModel as ReturnType<typeof fn>;

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Set as default model: GPT-4.1",
			}),
		);
		await waitFor(() => expect(args.onUpdateModel).toHaveBeenCalledTimes(1));
		expect(updateModelMock.mock.calls[0]).toEqual([
			"model-config-id",
			{ is_default: true },
		]);
		expect(canvas.queryByText("Edit Model")).not.toBeInTheDocument();

		await userEvent.click(
			canvas.getByRole("button", { name: "Default model: GPT-4o" }),
		);
		expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
		expect(canvas.queryByText("Edit Model")).not.toBeInTheDocument();

		const disabledStarButton = canvas.getByRole("button", {
			name: "Set as default model: GPT-4.1 Disabled",
		});
		expect(disabledStarButton).toHaveAttribute("aria-disabled", "true");
		await userEvent.click(disabledStarButton);
		expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
		expect(canvas.queryByText("Edit Model")).not.toBeInTheDocument();

		await userEvent.click(
			canvas.getByRole("button", { name: "Duplicate model: GPT-4.1" }),
		);
		await expect(canvas.findByText("Duplicate Model")).resolves.toBeVisible();
		expect(args.onCreateModel).not.toHaveBeenCalled();
		expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
		expect(canvas.queryByText("Edit Model")).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "Back" }));
		await userEvent.click(
			await canvas.findByRole("button", { name: "Edit model: GPT-4.1" }),
		);
		await expect(canvas.findByText("Edit Model")).resolves.toBeVisible();
	},
};
