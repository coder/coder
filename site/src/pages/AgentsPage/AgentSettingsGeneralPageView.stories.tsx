import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	AgentSettingsGeneralPageView,
	type AgentSettingsGeneralPageViewProps,
} from "./AgentSettingsGeneralPageView";

const baseArgs: AgentSettingsGeneralPageViewProps = {
	userPromptData: {
		custom_prompt: "Prefer concise answers with clear next steps.",
	},
	onSaveUserPrompt: fn(),
	isSavingUserPrompt: false,
	isSaveUserPromptError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsGeneralPageView",
	component: AgentSettingsGeneralPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsGeneralPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsGeneralPageView>;

export const Default: Story = {};
