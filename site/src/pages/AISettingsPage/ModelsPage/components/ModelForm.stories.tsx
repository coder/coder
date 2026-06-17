import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import { withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
	mockGPT5,
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
			canvas.getByRole("checkbox", { name: /set as default model/i }),
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
			canvas.getByRole("checkbox", { name: /set as default model/i }),
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
		const dialog = await screen.findByRole("dialog");
		await expect(
			within(dialog).getByText(/unsaved changes/i),
		).toBeInTheDocument();
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
			canvas.getByRole("checkbox", { name: /set as default model/i }),
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
			canvas.getByRole("checkbox", { name: /set as default model/i }),
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
		await userEvent.click(save);
		await expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
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
