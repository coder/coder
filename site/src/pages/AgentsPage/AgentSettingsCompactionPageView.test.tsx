import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { API } from "#/api/api";
import { userCompactionStrategyKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
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
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";

const renderView = (experiments: TypesGen.Experiment[]) => {
	const queryClient = createTestQueryClient();
	renderComponent(
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments,
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization],
					showOrganizations: false,
					canViewOrganizationSettings: false,
				}}
			>
				<AgentSettingsCompactionPageView
					models={[]}
					providerTypeByID={new Map()}
					organizations={[MockDefaultOrganization]}
					compactionTriggersByOrganizationID={new Map()}
					modelsError={undefined}
					compactionTriggerLoadErrors={[]}
					isLoadingModels={false}
					thresholds={[]}
					isThresholdsLoading={false}
					thresholdsError={undefined}
					onSaveThreshold={vi.fn()}
					onResetThreshold={vi.fn()}
					compactionStrategy="summarize"
				/>
			</DashboardContext.Provider>
		</QueryClientProvider>,
	);
	return queryClient;
};

const chooseTokenBudget = async () => {
	const trigger = await screen.findByRole("combobox", {
		name: "Compaction strategy",
	});
	await waitFor(() => expect(trigger).toBeEnabled());
	await userEvent.click(trigger);
	await userEvent.click(
		await screen.findByRole("option", { name: "Token budget" }),
	);
};

describe("AgentSettingsCompactionPageView compaction strategy", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("does not load the strategy when the experiment is off", async () => {
		const get = vi
			.spyOn(API.experimental, "getUserChatCompactionStrategy")
			.mockResolvedValue({ strategy: "summarize" });

		renderView([]);
		await screen.findByRole("table", { name: "Compaction thresholds" });

		expect(get).not.toHaveBeenCalled();
	});

	it("saves the chosen strategy and refetches it", async () => {
		let strategy: TypesGen.ChatCompactionStrategy = "summarize";
		vi.spyOn(
			API.experimental,
			"getUserChatCompactionStrategy",
		).mockImplementation(async () => ({ strategy }));
		const update = vi
			.spyOn(API.experimental, "updateUserChatCompactionStrategy")
			.mockImplementation(async (req) => {
				strategy = req.strategy;
				return req;
			});

		const queryClient = renderView(["chat-token-budget"]);
		await chooseTokenBudget();

		await waitFor(() =>
			expect(update).toHaveBeenCalledWith({ strategy: "token_budget" }),
		);
		await waitFor(() =>
			expect(queryClient.getQueryData(userCompactionStrategyKey)).toEqual({
				strategy: "token_budget",
			}),
		);
	});

	it("keeps the confirmed strategy when the refetch after a save fails", async () => {
		vi.spyOn(API.experimental, "getUserChatCompactionStrategy")
			.mockResolvedValueOnce({ strategy: "summarize" })
			.mockRejectedValue(new Error("strategy read failed"));
		vi.spyOn(
			API.experimental,
			"updateUserChatCompactionStrategy",
		).mockImplementation(async (req) => req);

		const queryClient = renderView(["chat-token-budget"]);
		await chooseTokenBudget();

		await waitFor(() =>
			expect(queryClient.getQueryState(userCompactionStrategyKey)?.status).toBe(
				"error",
			),
		);
		expect(queryClient.getQueryData(userCompactionStrategyKey)).toEqual({
			strategy: "token_budget",
		});
	});

	it("keeps the saved strategy when saving fails", async () => {
		vi.spyOn(
			API.experimental,
			"getUserChatCompactionStrategy",
		).mockResolvedValue({ strategy: "summarize" });
		vi.spyOn(
			API.experimental,
			"updateUserChatCompactionStrategy",
		).mockRejectedValue(new Error("strategy write failed"));

		const queryClient = renderView(["chat-token-budget"]);
		await chooseTokenBudget();

		await screen.findByRole("alert");
		expect(queryClient.getQueryData(userCompactionStrategyKey)).toEqual({
			strategy: "summarize",
		});
	});
});
