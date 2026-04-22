import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	AgentSettingsInstructionsPageView,
	type AgentSettingsInstructionsPageViewProps,
} from "./AgentSettingsInstructionsPageView";

const mockDefaultSystemPrompt = "You are Coder, an AI coding assistant.";

const baseArgs: AgentSettingsInstructionsPageViewProps = {
	systemPromptData: {
		system_prompt: "Always explain tradeoffs before proposing a change.",
		include_default_system_prompt: true,
		default_system_prompt: mockDefaultSystemPrompt,
	},
	planModeInstructionsData: {
		plan_mode_instructions:
			"Use a numbered checklist for implementation plans.",
	},
	onSaveSystemPrompt: fn(),
	isSavingSystemPrompt: false,
	isSaveSystemPromptError: false,
	onSavePlanModeInstructions: fn(),
	isSavingPlanModeInstructions: false,
	isSavePlanModeInstructionsError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsInstructionsPageView",
	component: AgentSettingsInstructionsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsInstructionsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsInstructionsPageView>;

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
			canvas.getByText(/built-in Coder Agents prompt is prepended/i),
		).toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "Preview" }));
		expect(await body.findByText("Default System Prompt")).toBeInTheDocument();
		expect(body.getByText(mockDefaultSystemPrompt)).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(body.queryByText("Default System Prompt")).not.toBeInTheDocument();
		});

		await userEvent.click(toggle);
		const promptForm = promptInput.closest("form");
		if (!(promptForm instanceof HTMLFormElement)) {
			throw new Error("Expected system prompt textarea to live inside a form.");
		}
		const saveButton = within(promptForm).getByRole("button", {
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
			canvas.getByText(/only the additional instructions below are used/i),
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

		await canvas.findByText("System Instructions");
		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("4");
	},
};

// The deleted combined story covered both prompt editors on one page. After
// the split, this story covers the system prompt half and the General page
// stories cover the personal instructions half.
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

		await canvas.findByText("System Instructions");
		await canvas.findByDisplayValue("You are a helpful coding assistant.");
		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();
	},
};

export const SavesPlanModeInstructions: Story = {
	args: {
		planModeInstructionsData: { plan_mode_instructions: "" },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const textarea = await canvas.findByPlaceholderText(
			"Additional instructions for planning mode",
		);

		await userEvent.clear(textarea);
		await userEvent.type(textarea, "Always produce a concise plan first.");

		const planModeForm = textarea.closest("form");
		if (!(planModeForm instanceof HTMLFormElement)) {
			throw new Error(
				"Expected plan mode instructions textarea to live inside a form.",
			);
		}
		const saveButton = within(planModeForm).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSavePlanModeInstructions).toHaveBeenCalledWith(
				{ plan_mode_instructions: "Always produce a concise plan first." },
				expect.anything(),
			);
		});
	},
};
