import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	InstructionsPageView,
	type InstructionsPageViewProps,
} from "./InstructionsPageView";

const mockDefaultSystemPrompt = "You are Coder, an AI coding assistant.";
const saveOrder: string[] = [];

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
			"Additional plan mode instructions",
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

export const SystemPromptSaveErrorThenCancel: Story = {
	args: {
		systemPromptData: {
			system_prompt: "Baseline system prompt.",
			include_default_system_prompt: false,
			default_system_prompt: mockDefaultSystemPrompt,
		},
		onResetSystemPromptSave: fn(),
		onResetPlanModeInstructionsSave: fn(),
		isSaveSystemPromptError: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText("Failed to save system prompt."),
		).toBeInTheDocument();

		const textarea = await canvas.findByLabelText(
			"Additional system instructions",
		);
		await userEvent.type(textarea, " edited");

		const cancelButton = canvas.getByRole("button", {
			name: "Cancel",
		});
		await waitFor(() => {
			expect(cancelButton).toBeEnabled();
		});
		await userEvent.click(cancelButton);

		expect(args.onResetSystemPromptSave).toHaveBeenCalledTimes(1);
		expect(args.onResetPlanModeInstructionsSave).toHaveBeenCalledTimes(1);
		await waitFor(() => {
			expect(
				canvas.getByDisplayValue("Baseline system prompt."),
			).toBeInTheDocument();
		});
	},
};

export const CleanSystemPromptErrorCancelDismisses: Story = {
	args: {
		onResetSystemPromptSave: fn(),
		onResetPlanModeInstructionsSave: fn(),
		isSaveSystemPromptError: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText("Failed to save system prompt."),
		).toBeInTheDocument();

		const cancelButton = canvas.getByRole("button", {
			name: "Cancel",
		});
		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});

		await waitFor(() => {
			expect(cancelButton).toBeEnabled();
		});
		expect(saveButton).toBeDisabled();

		await userEvent.click(cancelButton);

		expect(args.onResetSystemPromptSave).toHaveBeenCalledTimes(1);
		expect(args.onResetPlanModeInstructionsSave).toHaveBeenCalledTimes(1);
	},
};

export const CleanPlanModeInstructionsErrorCancelDismisses: Story = {
	args: {
		onResetSystemPromptSave: fn(),
		onResetPlanModeInstructionsSave: fn(),
		isSavePlanModeInstructionsError: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText("Failed to save plan mode instructions."),
		).toBeInTheDocument();

		const cancelButton = canvas.getByRole("button", {
			name: "Cancel",
		});
		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});

		await waitFor(() => {
			expect(cancelButton).toBeEnabled();
		});
		expect(saveButton).toBeDisabled();

		await userEvent.click(cancelButton);

		expect(args.onResetSystemPromptSave).toHaveBeenCalledTimes(1);
		expect(args.onResetPlanModeInstructionsSave).toHaveBeenCalledTimes(1);
	},
};

export const CleanBothErrorsCancelDismisses: Story = {
	args: {
		onResetSystemPromptSave: fn(),
		onResetPlanModeInstructionsSave: fn(),
		isSaveSystemPromptError: true,
		isSavePlanModeInstructionsError: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText("Failed to save system prompt."),
		).toBeInTheDocument();
		expect(
			canvas.getByText("Failed to save plan mode instructions."),
		).toBeInTheDocument();

		const cancelButton = canvas.getByRole("button", {
			name: "Cancel",
		});
		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});

		await waitFor(() => {
			expect(cancelButton).toBeEnabled();
		});
		expect(saveButton).toBeDisabled();

		await userEvent.click(cancelButton);

		expect(args.onResetSystemPromptSave).toHaveBeenCalledTimes(1);
		expect(args.onResetPlanModeInstructionsSave).toHaveBeenCalledTimes(1);
	},
};

export const PlanModeInstructionsSaveError: Story = {
	args: {
		isSavePlanModeInstructionsError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText("Failed to save plan mode instructions."),
		).toBeInTheDocument();
	},
};

export const SavesBothSections: Story = {
	args: {
		systemPromptData: {
			system_prompt: "",
			include_default_system_prompt: false,
			default_system_prompt: mockDefaultSystemPrompt,
		},
		planModeInstructionsData: { plan_mode_instructions: "" },
		onSaveSystemPrompt: fn(async () => {
			saveOrder.push("system");
		}),
		onSavePlanModeInstructions: fn(async () => {
			saveOrder.push("plan");
		}),
	},
	play: async ({ canvasElement, args }) => {
		saveOrder.length = 0;
		const canvas = within(canvasElement);

		const systemPromptTextarea = await canvas.findByLabelText(
			"Additional system instructions",
		);
		const planModeTextarea = await canvas.findByLabelText(
			"Additional plan mode instructions",
		);

		await userEvent.type(systemPromptTextarea, "New system instruction.");
		await userEvent.type(planModeTextarea, "New plan mode guidance.");

		const saveButton = canvas.getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveSystemPrompt).toHaveBeenCalledWith({
				system_prompt: "New system instruction.",
				include_default_system_prompt: false,
			});
		});
		await waitFor(() => {
			expect(args.onSavePlanModeInstructions).toHaveBeenCalledWith({
				plan_mode_instructions: "New plan mode guidance.",
			});
		});

		expect(saveOrder).toEqual(["system", "plan"]);
	},
};

const RefetchPromptWrapper: FC = () => {
	const [systemPromptValue, setSystemPromptValue] = useState("Old");

	return (
		<>
			<button
				type="button"
				onClick={() => setSystemPromptValue("New")}
				className="sr-only"
			>
				Simulate system prompt refetch
			</button>
			<InstructionsPageView
				systemPromptData={{
					system_prompt: systemPromptValue,
					include_default_system_prompt: false,
					default_system_prompt: mockDefaultSystemPrompt,
				}}
				planModeInstructionsData={{
					plan_mode_instructions: "Baseline plan mode guidance.",
				}}
				onSaveSystemPrompt={fn()}
				onSavePlanModeInstructions={fn()}
				onResetSystemPromptSave={fn()}
				onResetPlanModeInstructionsSave={fn()}
				isSaving={false}
				isSaveSystemPromptError={false}
				isSavePlanModeInstructionsError={true}
			/>
		</>
	);
};

export const PartialSaveFailureCancelResyncsToServer: Story = {
	render: () => <RefetchPromptWrapper />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByDisplayValue("Old");
		await canvas.findByDisplayValue("Baseline plan mode guidance.");
		expect(
			canvas.getByText("Failed to save plan mode instructions."),
		).toBeInTheDocument();

		const planTextarea = await canvas.findByLabelText(
			"Additional plan mode instructions",
		);
		await userEvent.type(planTextarea, " edited");

		await userEvent.click(
			canvas.getByRole("button", { name: "Simulate system prompt refetch" }),
		);

		const cancelButton = canvas.getByRole("button", {
			name: "Cancel",
		});
		await waitFor(() => {
			expect(cancelButton).toBeEnabled();
		});
		await userEvent.click(cancelButton);

		await waitFor(() => {
			expect(canvas.getByDisplayValue("New")).toBeInTheDocument();
			expect(canvas.queryByDisplayValue("Old")).toBeNull();
		});
	},
};
