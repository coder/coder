import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	chatPersonalMemorySettingsKey,
	chatUserMemoriesKey,
} from "#/api/queries/chatUserMemories";
import {
	MockChatUserMemory,
	MockChatUserMemory2,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	AgentSettingsMemoryPageView,
	type AgentSettingsMemoryPageViewProps,
} from "./AgentSettingsMemoryPageView";

const baseArgs: AgentSettingsMemoryPageViewProps = {
	organizations: [MockDefaultOrganization],
	selectedOrganization: MockDefaultOrganization,
	settings: { enabled: true },
	settingsError: null,
	isLoadingSettings: false,
	isSavingSettings: false,
	isSaveSettingsError: false,
	onSelectOrganization: fn(),
	onSaveSettings: fn(),
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsMemoryPageView",
	component: AgentSettingsMemoryPageView,
	args: baseArgs,
	parameters: {
		queries: [
			{
				key: chatUserMemoriesKey(MockDefaultOrganization.id),
				data: [MockChatUserMemory, MockChatUserMemory2],
			},
		],
	},
} satisfies Meta<typeof AgentSettingsMemoryPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const EnabledWithMemories: Story = {};

export const Disabled: Story = {
	args: { settings: { enabled: false } },
};

export const Empty: Story = {
	parameters: {
		queries: [
			{ key: chatPersonalMemorySettingsKey, data: { enabled: true } },
			{ key: chatUserMemoriesKey(MockDefaultOrganization.id), data: [] },
		],
	},
};

export const MultipleOrganizations: Story = {
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
};
