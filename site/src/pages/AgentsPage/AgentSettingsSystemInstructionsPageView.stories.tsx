import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	AgentSettingsSystemInstructionsPageView,
	type AgentSettingsSystemInstructionsPageViewProps,
} from "./AgentSettingsSystemInstructionsPageView";

const baseArgs: AgentSettingsSystemInstructionsPageViewProps = {
	systemPromptData: {
		system_prompt: "Always explain tradeoffs before proposing a change.",
		include_default_system_prompt: true,
		default_system_prompt: "You are Coder, an AI coding assistant.",
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
	title: "pages/AgentsPage/AgentSettingsSystemInstructionsPageView",
	component: AgentSettingsSystemInstructionsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsSystemInstructionsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsSystemInstructionsPageView>;

export const Default: Story = {};
