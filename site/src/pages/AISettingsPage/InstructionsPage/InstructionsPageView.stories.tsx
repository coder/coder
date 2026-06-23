import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	InstructionsPageView,
	type InstructionsPageViewProps,
} from "./InstructionsPageView";

const mockDefaultSystemPrompt = "You are Coder, an AI coding assistant.";

const baseArgs: InstructionsPageViewProps = {
	systemPromptData: {
		system_prompt: "Always explain tradeoffs before proposing a change.",
		include_default_system_prompt: true,
		default_system_prompt: mockDefaultSystemPrompt,
	},
	planModeInstructionsData: {
		plan_mode_instructions:
			"Use a numbered checklist for implementation plans.",
	},
	onSaveSystemPrompt: fn(async () => undefined),
	onSavePlanModeInstructions: fn(async () => undefined),
	onResetSystemPromptSave: fn(),
	onResetPlanModeInstructionsSave: fn(),
	isSaving: false,
	isSaveSystemPromptError: false,
	isSavePlanModeInstructionsError: false,
};

const meta = {
	title: "pages/AISettingsPage/InstructionsPage/InstructionsPageView",
	component: InstructionsPageView,
	args: baseArgs,
} satisfies Meta<typeof InstructionsPageView>;

export default meta;
type Story = StoryObj<typeof InstructionsPageView>;

export const Default: Story = {};

export const AdminWithDefaultToggleOn: Story = {
	args: {
		systemPromptData: {
			system_prompt: "Always use TypeScript for code examples.",
			include_default_system_prompt: true,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const toggle = await canvas.findByRole("switch", {
			name: "Include Coder Agents default system prompt",
		});
		expect(toggle).toBeChecked();
		const promptInput = await canvas.findByDisplayValue(
			"Always use TypeScript for code examples.",
		);
		expect(promptInput).toBeInTheDocument();
		expect(
			canvas.getByText("Additional system instructions"),
		).toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "View prompt" }));
		expect(await body.findByText("Default System Prompt")).toBeInTheDocument();
		expect(body.getByText(mockDefaultSystemPrompt)).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(body.queryByText("Default System Prompt")).not.toBeInTheDocument();
		});

		await userEvent.click(toggle);
		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
	},
};

export const AdminWithDefaultToggleOff: Story = {
	args: {
		systemPromptData: {
			system_prompt: "You are a custom assistant.",
			include_default_system_prompt: false,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Include Coder Agents default system prompt",
		});
		expect(toggle).not.toBeChecked();
		expect(
			await canvas.findByDisplayValue("You are a custom assistant."),
		).toBeInTheDocument();
		expect(
			canvas.getByText("Additional system instructions"),
		).toBeInTheDocument();
	},
};

export const InvisibleUnicodeWarningSystemPrompt: Story = {
	args: {
		systemPromptData: {
			system_prompt:
				"Normal prompt text\u200b\u200b\u200b\u200bhidden instruction",
			include_default_system_prompt: true,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Additional system instructions");
		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("4");
	},
};

export const NoWarningForCleanPrompt: Story = {
	args: {
		systemPromptData: {
			system_prompt: "You are a helpful coding assistant.",
			include_default_system_prompt: true,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Additional system instructions");
		await canvas.findByDisplayValue("You are a helpful coding assistant.");
		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();
	},
};

export const SavesSystemPrompt: Story = {
	args: {
		systemPromptData: {
			system_prompt: "",
			include_default_system_prompt: false,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const textarea = await canvas.findByLabelText(
			"Additional system instructions",
		);

		await userEvent.type(textarea, "Always explain tradeoffs first.");
		await userEvent.click(
			canvas.getByRole("switch", {
				name: "Include Coder Agents default system prompt",
			}),
		);

		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveSystemPrompt).toHaveBeenCalledWith({
				system_prompt: "Always explain tradeoffs first.",
				include_default_system_prompt: true,
			});
		});
		expect(args.onSavePlanModeInstructions).not.toHaveBeenCalled();
	},
};

export const SavesPlanModeInstructions: Story = {
	args: {
		planModeInstructionsData: { plan_mode_instructions: "" },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const textarea = await canvas.findByLabelText(
			"Additional Plan mode instructions",
		);

		await userEvent.clear(textarea);
		await userEvent.type(textarea, "Always produce a concise plan first.");

		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSavePlanModeInstructions).toHaveBeenCalledWith({
				plan_mode_instructions: "Always produce a concise plan first.",
			});
		});
		expect(args.onSaveSystemPrompt).not.toHaveBeenCalled();
	},
};
