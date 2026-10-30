import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockChatModel } from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import {
	AgentSettingsCompactionPageView,
	type AgentSettingsCompactionPageViewProps,
} from "./AgentSettingsCompactionPageView";

const baseArgs: AgentSettingsCompactionPageViewProps = {
	models: [
		{
			...MockChatModel,
			id: "model-config-1",
			organization_id: MockDefaultOrganization.id,
			ai_provider_id: "prov-openai",
			model: "gpt-4.1-mini",
			display_name: "GPT 4.1 Mini",
			is_default: false,
			context_limit: 1_000_000,
		},
	],
	providerInfoByID: new Map([
		["prov-openai", { provider: "openai", displayName: "OpenAI", icon: "" }],
	]),
	organizations: [MockDefaultOrganization],
	compactionTriggersByOrganizationID: new Map(),
	compactionTriggerLoadErrors: [],
	modelsError: undefined,
	isLoadingModels: false,
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

export const Default: Story = {};

export const UnsavedThreshold: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const thresholdInput = await canvas.findByLabelText(
			`GPT 4.1 Mini compaction threshold for ${MockDefaultOrganization.display_name}`,
		);

		await userEvent.clear(thresholdInput);
		await userEvent.type(thresholdInput, "80");
	},
};

export const InvalidThreshold: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const thresholdInput = await canvas.findByLabelText(
			`GPT 4.1 Mini compaction threshold for ${MockDefaultOrganization.display_name}`,
		);

		await userEvent.clear(thresholdInput);
		await userEvent.type(thresholdInput, "150");
	},
};
