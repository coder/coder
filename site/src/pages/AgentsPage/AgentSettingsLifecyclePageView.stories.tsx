import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	AgentSettingsLifecyclePageView,
	type AgentSettingsLifecyclePageViewProps,
} from "./AgentSettingsLifecyclePageView";

const baseArgs: AgentSettingsLifecyclePageViewProps = {
	workspaceTTLData: { workspace_ttl_ms: 3_600_000 },
	isWorkspaceTTLLoading: false,
	isWorkspaceTTLLoadError: false,
	onSaveWorkspaceTTL: fn(),
	isSavingWorkspaceTTL: false,
	isSaveWorkspaceTTLError: false,
	retentionDaysData: { retention_days: 30 },
	isRetentionDaysLoading: false,
	isRetentionDaysLoadError: false,
	onSaveRetentionDays: fn(),
	isSavingRetentionDays: false,
	isSaveRetentionDaysError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsLifecyclePageView",
	component: AgentSettingsLifecyclePageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsLifecyclePageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsLifecyclePageView>;

export const Default: Story = {};
