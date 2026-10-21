import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import {
	organizationChatModelsKey,
	userChatProviderConfigsKey,
	userCompactionStrategyKey,
	userCompactionThresholdsKey,
} from "#/api/queries/chats";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import AgentSettingsCompactionPage from "./AgentSettingsCompactionPage";

const meta = {
	title: "pages/AgentsPage/AgentSettingsCompactionPage",
	component: AgentSettingsCompactionPage,
	decorators: [withDashboardProvider],
} satisfies Meta<typeof AgentSettingsCompactionPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TokenBudgetWhileOverridesLoad: Story = {
	parameters: {
		experiments: ["chat-token-budget"],
		queries: [
			{ key: userCompactionStrategyKey, data: { strategy: "token_budget" } },
			{
				key: organizationChatModelsKey(MockDefaultOrganization.id),
				data: {
					models: [MockChatModel],
					providers: [MockChatModelProviderDescriptor],
					unsupported_providers: [],
				},
			},
			{ key: userCompactionThresholdsKey, data: { thresholds: [] } },
			{ key: userChatProviderConfigsKey, data: [] },
		],
	},
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getOrganizationChatModelOverrides",
		).mockReturnValue(new Promise(() => undefined));
	},
};
