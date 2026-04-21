import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentSettingsCompactionPageView,
	type AgentSettingsCompactionPageViewProps,
} from "./AgentSettingsCompactionPageView";

const baseArgs: AgentSettingsCompactionPageViewProps = {
	modelConfigsData: [
		{
			id: "model-config-1",
			provider: "openai",
			model: "gpt-4.1-mini",
			display_name: "GPT 4.1 Mini",
			enabled: true,
			is_default: false,
			context_limit: 1_000_000,
			compression_threshold: 70,
			created_at: "2026-03-12T12:00:00.000Z",
			updated_at: "2026-03-12T12:00:00.000Z",
		},
	] as TypesGen.ChatModelConfig[],
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	thresholds: [
		{
			model_config_id: "model-config-1",
			threshold_percent: 60,
		},
	],
	isThresholdsLoading: false,
	thresholdsError: undefined,
	onSaveThreshold: fn(async () => undefined),
	onResetThreshold: fn(async () => undefined),
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsCompactionPageView",
	component: AgentSettingsCompactionPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsCompactionPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsCompactionPageView>;

// Interaction coverage for threshold save and reset lives in
// UserCompactionThresholdSettings.stories.tsx because this page view only wraps
// that component with a section header.
export const Default: Story = {};
