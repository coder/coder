import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	AgentSettingsExperimentsPageView,
	type AgentSettingsExperimentsPageViewProps,
} from "./AgentSettingsExperimentsPageView";

const baseArgs: AgentSettingsExperimentsPageViewProps = {
	desktopEnabledData: { enable_desktop: false },
	onSaveDesktopEnabled: fn(),
	isSavingDesktopEnabled: false,
	isSaveDesktopEnabledError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsExperimentsPageView",
	component: AgentSettingsExperimentsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsExperimentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsExperimentsPageView>;

export const Default: Story = {};
