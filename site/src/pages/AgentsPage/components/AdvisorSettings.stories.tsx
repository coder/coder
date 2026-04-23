import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { AdvisorSettings } from "./AdvisorSettings";

const nilUUID = "00000000-0000-0000-0000-000000000000";

const mockModelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: "model-1",
		provider: "openai",
		model: "gpt-5",
		display_name: "GPT-5",
		enabled: true,
		is_default: true,
		context_limit: 200000,
		compression_threshold: 80,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
	{
		id: "model-2",
		provider: "anthropic",
		model: "claude-sonnet-4",
		display_name: "Claude Sonnet 4",
		enabled: true,
		is_default: false,
		context_limit: 200000,
		compression_threshold: 80,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
	{
		id: "model-3",
		provider: "openai",
		model: "gpt-3.5",
		display_name: "GPT-3.5 (Disabled)",
		enabled: false,
		is_default: false,
		context_limit: 16000,
		compression_threshold: 60,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
];

const defaultAdvisorConfig: TypesGen.AdvisorConfig = {
	enabled: false,
	max_uses_per_run: 0,
	max_output_tokens: 0,
	reasoning_effort: "",
	model_config_id: "",
};

const meta = {
	title: "pages/AgentsPage/AdvisorSettings",
	component: AdvisorSettings,
	args: {
		advisorConfigData: defaultAdvisorConfig,
		isAdvisorConfigLoading: false,
		isAdvisorConfigFetching: false,
		isAdvisorConfigLoadError: false,
		modelConfigs: mockModelConfigs,
		modelConfigsError: undefined,
		isLoadingModelConfigs: false,
		onSaveAdvisorConfig: fn((_req, options) => {
			options?.onSuccess?.();
		}),
		isSavingAdvisorConfig: false,
		isSaveAdvisorConfigError: false,
		saveAdvisorConfigError: undefined,
	},
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof AdvisorSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const enableAdvisorSwitch = canvas.getByRole("switch", {
			name: /Enable advisor/i,
		});

		expect(
			canvas.queryByRole("spinbutton", { name: /Max uses per run/i }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("combobox", { name: /Advisor model/i }),
		).not.toBeInTheDocument();

		await userEvent.click(enableAdvisorSwitch);

		await waitFor(() => {
			expect(
				canvas.getByRole("spinbutton", { name: /Max uses per run/i }),
			).toBeVisible();
			expect(
				canvas.getByRole("combobox", { name: /Advisor model/i }),
			).toBeVisible();
		});
	},
};

export const Enabled: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const maxUsesInput = await canvas.findByRole("spinbutton", {
			name: /Max uses per run/i,
		});
		const maxOutputTokensInput = canvas.getByRole("spinbutton", {
			name: /Max output tokens/i,
		});
		const reasoningEffortSelect = canvas.getByRole("combobox", {
			name: /Reasoning effort/i,
		});
		const advisorModelSelect = canvas.getByRole("combobox", {
			name: /Advisor model/i,
		});
		const saveButton = canvas.getByRole("button", { name: /Save/i });

		expect(saveButton).toBeDisabled();

		await userEvent.clear(maxUsesInput);
		await userEvent.type(maxUsesInput, "5");
		await userEvent.clear(maxOutputTokensInput);
		await userEvent.type(maxOutputTokensInput, "2048");

		await userEvent.click(reasoningEffortSelect);
		await userEvent.click(await body.findByRole("option", { name: /^High$/i }));

		await userEvent.click(advisorModelSelect);
		expect(
			body.queryByRole("option", { name: /GPT-3.5 \(Disabled\)/i }),
		).not.toBeInTheDocument();
		await userEvent.click(
			await body.findByRole("option", { name: /Claude Sonnet 4/i }),
		);

		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});

		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalled();
		});

		const [request, options] = args.onSaveAdvisorConfig.mock.calls[0];
		expect(request).toEqual({
			enabled: true,
			max_uses_per_run: 5,
			max_output_tokens: 2048,
			reasoning_effort: "high",
			model_config_id: "model-2",
		});
		expect(typeof options?.onSuccess).toBe("function");

		await waitFor(() => {
			expect(saveButton).toBeDisabled();
		});
	},
};

export const SaveWithUseChatModel: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const maxUsesInput = await canvas.findByRole("spinbutton", {
			name: /Max uses per run/i,
		});

		expect(
			canvas.getByRole("combobox", { name: /Advisor model/i }),
		).toHaveTextContent(/Use chat model/i);

		await userEvent.clear(maxUsesInput);
		await userEvent.type(maxUsesInput, "3");

		const saveButton = canvas.getByRole("button", { name: /Save/i });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalled();
		});
		const [request] = args.onSaveAdvisorConfig.mock.calls[0];
		expect(request.model_config_id).toBe(nilUUID);
	},
};

export const NilUUIDInitialRoundTrip: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
			model_config_id: nilUUID,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const advisorModelSelect = await canvas.findByRole("combobox", {
			name: /Advisor model/i,
		});

		expect(advisorModelSelect).toHaveTextContent(/Use chat model/i);
	},
};

export const CustomConfig: Story = {
	args: {
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 7,
			max_output_tokens: 8192,
			reasoning_effort: "medium",
			model_config_id: "model-2",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const maxUsesInput = await canvas.findByRole("spinbutton", {
			name: /Max uses per run/i,
		});
		const maxOutputTokensInput = canvas.getByRole("spinbutton", {
			name: /Max output tokens/i,
		});

		expect(maxUsesInput).toHaveValue(7);
		expect(maxOutputTokensInput).toHaveValue(8192);
		expect(
			canvas.getByRole("combobox", { name: /Reasoning effort/i }),
		).toHaveTextContent(/Medium/i);
		expect(
			canvas.getByRole("combobox", { name: /Advisor model/i }),
		).toHaveTextContent(/Claude Sonnet 4/i);
	},
};

export const UnavailableSelectedModel: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
			model_config_id: "22222222-2222-2222-2222-222222222222",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const advisorModelSelect = await canvas.findByRole("combobox", {
			name: /Advisor model/i,
		});

		expect(advisorModelSelect).toHaveTextContent(
			/Unavailable model \(22222222-2222-2222-2222-222222222222\)/i,
		);
	},
};

export const Loading: Story = {
	args: {
		advisorConfigData: undefined,
		isAdvisorConfigLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByRole("switch", { name: /Enable advisor/i }),
		).toBeDisabled();
		expect(canvas.getByRole("button", { name: /Save/i })).toBeDisabled();
	},
};

export const Refetching: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
		isAdvisorConfigFetching: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByRole("switch", { name: /Enable advisor/i }),
		).toBeDisabled();
		expect(
			canvas.getByRole("spinbutton", { name: /Max uses per run/i }),
		).toBeDisabled();
		expect(
			canvas.getByRole("combobox", { name: /Reasoning effort/i }),
		).toBeDisabled();
		expect(canvas.getByRole("button", { name: /Save/i })).toBeDisabled();
	},
};

export const LoadingModelConfigs: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
			model_config_id: "model-2",
		},
		modelConfigs: [],
		isLoadingModelConfigs: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const advisorModelSelect = await canvas.findByRole("combobox", {
			name: /Advisor model/i,
		});

		expect(advisorModelSelect).toBeDisabled();
		expect(advisorModelSelect).toHaveTextContent(/Loading/i);
		expect(
			canvas.getByText(/Loading chat model overrides\./i),
		).toBeInTheDocument();
	},
};

export const ModelConfigsError: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
			model_config_id: "model-2",
		},
		modelConfigsError: new Error("fail"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const advisorModelSelect = await canvas.findByRole("combobox", {
			name: /Advisor model/i,
		});

		expect(advisorModelSelect).toBeDisabled();
		expect(
			canvas.getByText(
				/Model overrides are unavailable\. The current selection will be sent unchanged\./i,
			),
		).toBeInTheDocument();
	},
};

export const ModelConfigsErrorWithUnsetSelection: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
		modelConfigsError: new Error("fail"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText(
				/Model overrides are unavailable\. Saving will keep using the chat model\./i,
			),
		).toBeInTheDocument();
	},
};

export const LoadError: Story = {
	args: {
		advisorConfigData: undefined,
		isAdvisorConfigLoadError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText(/Failed to load advisor settings\./i),
		).toBeInTheDocument();
	},
};

export const Saving: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
		isSavingAdvisorConfig: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByRole("switch", { name: /Enable advisor/i }),
		).toBeDisabled();
		expect(
			canvas.getByRole("spinbutton", { name: /Max uses per run/i }),
		).toBeDisabled();
		expect(
			canvas.getByRole("combobox", { name: /Reasoning effort/i }),
		).toBeDisabled();
		expect(canvas.getByRole("button", { name: /Save/i })).toBeDisabled();
	},
};

export const SaveError: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
		isSaveAdvisorConfigError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText(/Failed to save advisor settings\./i),
		).toBeInTheDocument();
	},
};

export const SaveErrorWithDetail: Story = {
	args: {
		advisorConfigData: {
			...defaultAdvisorConfig,
			enabled: true,
		},
		isSaveAdvisorConfigError: true,
		saveAdvisorConfigError: new Error(
			"reasoning_effort must be one of: low, medium, high.",
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText(/reasoning_effort must be one of: low, medium, high\./i),
		).toBeInTheDocument();
	},
};

export const DisableThenSave: Story = {
	args: {
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 5,
			max_output_tokens: 2048,
			reasoning_effort: "high",
			model_config_id: "model-2",
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const enableAdvisorSwitch = await canvas.findByRole("switch", {
			name: /Enable advisor/i,
		});

		expect(
			canvas.getByRole("spinbutton", { name: /Max uses per run/i }),
		).toBeVisible();

		// Clear a numeric field to an invalid value before disabling. After
		// disabling the field is hidden, and saving must not silently overwrite
		// the stored limit with a coerced value.
		const maxUsesInput = canvas.getByRole("spinbutton", {
			name: /Max uses per run/i,
		});
		await userEvent.clear(maxUsesInput);

		await userEvent.click(enableAdvisorSwitch);

		await waitFor(() => {
			expect(
				canvas.queryByRole("spinbutton", { name: /Max uses per run/i }),
			).not.toBeInTheDocument();
		});

		const saveButton = canvas.getByRole("button", { name: /Save/i });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});

		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalled();
		});

		const [request] = args.onSaveAdvisorConfig.mock.calls[0];
		expect(request).toEqual({
			enabled: false,
			max_uses_per_run: 5,
			max_output_tokens: 2048,
			reasoning_effort: "high",
			model_config_id: "model-2",
		});
	},
};
