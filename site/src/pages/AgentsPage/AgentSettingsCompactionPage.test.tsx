import { screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { API } from "#/api/api";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import AgentSettingsCompactionPage from "./AgentSettingsCompactionPage";

describe("AgentSettingsCompactionPage", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("shows handoff targets under token budget while organization overrides load", async () => {
		vi.spyOn(
			API.experimental,
			"getUserChatCompactionStrategy",
		).mockResolvedValue({ strategy: "token_budget" });
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [MockChatModel],
			providers: [MockChatModelProviderDescriptor],
			unsupported_providers: [],
		});
		vi.spyOn(
			API.experimental,
			"getUserChatCompactionThresholds",
		).mockResolvedValue({ thresholds: [] });
		vi.spyOn(API.experimental, "getUserAIProviderKeyConfigs").mockResolvedValue(
			[],
		);
		const getOverrides = vi
			.spyOn(API.experimental, "getOrganizationChatModelOverrides")
			.mockReturnValue(new Promise(() => undefined));

		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<DashboardContext.Provider
					value={{
						entitlements: MockEntitlements,
						experiments: ["chat-token-budget"],
						appearance: MockAppearanceConfig,
						buildInfo: MockBuildInfo,
						organizations: [MockDefaultOrganization],
						showOrganizations: false,
						canViewOrganizationSettings: false,
					}}
				>
					<AgentSettingsCompactionPage />
				</DashboardContext.Provider>
			</QueryClientProvider>,
		);

		await screen.findByRole("textbox", { name: /handoff target for/ });
		expect(getOverrides).toHaveBeenCalled();
	});
});
