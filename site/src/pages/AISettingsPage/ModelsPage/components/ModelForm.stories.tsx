import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockDisabledProviderState,
	MockOpenAIProviderState,
	mockGPT5,
	mockProviderDisabledModel,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

const meta: Meta<typeof ModelForm> = {
	title: "pages/AISettingsPage/ModelsPage/ModelForm",
	component: ModelForm,
	decorators: [withToaster],
	args: {
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		onProviderChange: fn(),
		isSaving: false,
		isDeleting: false,
		onCreateModel: fn(async () => undefined),
		onUpdateModel: fn(async () => undefined),
	},
	parameters: {
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

export const CostTrackingExpanded: Story = {
	args: {
		editingModel: mockGPT5,
		onDeleteModel: fn(async () => undefined),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: /cost tracking/i });
		await userEvent.click(toggle);
	},
};
