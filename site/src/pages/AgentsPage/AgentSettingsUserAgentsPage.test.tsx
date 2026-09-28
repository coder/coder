import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	chatModels,
	userChatPersonalModelOverrides,
} from "#/api/queries/chats";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import themes, { DEFAULT_THEME } from "#/theme";
import AgentSettingsUserAgentsPage from "./AgentSettingsUserAgentsPage";
import { buildOverridesResponse } from "./AgentSettingsUserAgentsPageView.stories";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockDefaultOrganization, MockOrganization2],
	}),
}));

const overrides = buildOverridesResponse();

const renderPage = (search = "") => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
		},
	});
	for (const organization of [MockDefaultOrganization, MockOrganization2]) {
		queryClient.setQueryData(
			userChatPersonalModelOverrides(organization.id).queryKey,
			overrides,
		);
		queryClient.setQueryData(chatModels(organization.id).queryKey, {
			models: [{ ...MockChatModel, organization_id: organization.id }],
			providers: [MockChatModelProviderDescriptor],
			unsupported_providers: [],
		});
	}
	const router = createMemoryRouter(
		[
			{
				path: "/agents/settings/agents",
				element: <AgentSettingsUserAgentsPage />,
			},
		],
		{ initialEntries: [`/agents/settings/agents${search}`] },
	);
	render(
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>
					<RouterProvider router={router} />
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>,
	);
	return { router, queryClient };
};

beforeEach(() => {
	vi.spyOn(
		API.experimental,
		"getUserChatPersonalModelOverrides",
	).mockResolvedValue(overrides);
	vi.spyOn(toast, "success").mockImplementation(() => "success");
	vi.spyOn(toast, "error").mockImplementation(() => "error");
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("AgentSettingsUserAgentsPage", () => {
	it.each(["success", "error"])(
		"prevents overlapping row saves until the pending save settles with %s",
		async (outcome) => {
			const user = userEvent.setup();
			const pendingSave = createDeferred<void>();
			const save = vi
				.spyOn(API.experimental, "updateUserChatPersonalModelOverride")
				.mockImplementationOnce(() => pendingSave.promise)
				.mockResolvedValue(undefined);
			renderPage();
			const root = within(
				screen.getByRole("region", { name: "Root agent model" }),
			);
			const general = within(
				screen.getByRole("region", { name: "General subagent model" }),
			);
			await user.click(root.getByRole("combobox"));
			await user.click(
				await screen.findByRole("option", {
					name: new RegExp(MockChatModel.display_name),
				}),
			);
			await user.click(general.getByRole("combobox"));
			await user.click(
				await screen.findByRole("option", { name: /^Chat default:/ }),
			);
			await user.click(root.getByRole("button", { name: "Save" }));
			await waitFor(() => expect(save).toHaveBeenCalledTimes(1));

			await user.click(general.getByRole("button", { name: "Save" }));
			await user.click(root.getByRole("button", { name: "Save" }));
			expect(save).toHaveBeenCalledTimes(1);
			expect(save).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				"me",
				"root",
				{
					mode: "model",
					model_config_id: MockChatModel.id,
				},
			);

			await act(async () => {
				if (outcome === "success") {
					pendingSave.resolve();
				} else {
					pendingSave.reject(new Error("Save failed"));
				}
			});
			await waitFor(() =>
				expect(
					outcome === "success" ? toast.success : toast.error,
				).toHaveBeenCalled(),
			);
			await user.click(general.getByRole("button", { name: "Save" }));
			await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
			expect(save).toHaveBeenLastCalledWith(
				MockDefaultOrganization.id,
				"me",
				"general",
				{
					mode: "chat_default",
					model_config_id: "",
				},
			);
		},
	);

	it("keeps saves blocked across organization switches and preserves other URL parameters", async () => {
		const user = userEvent.setup();
		const pendingSave = createDeferred<void>();
		const save = vi
			.spyOn(API.experimental, "updateUserChatPersonalModelOverride")
			.mockImplementationOnce(() => pendingSave.promise)
			.mockResolvedValue(undefined);
		const { router, queryClient } = renderPage(
			`?org=${MockOrganization2.name}&keep=value`,
		);
		const root = within(
			screen.getByRole("region", { name: "Root agent model" }),
		);
		await user.click(root.getByRole("combobox"));
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockChatModel.display_name),
			}),
		);
		await user.click(root.getByRole("button", { name: "Save" }));
		await waitFor(() =>
			expect(save).toHaveBeenCalledWith(MockOrganization2.id, "me", "root", {
				mode: "model",
				model_config_id: MockChatModel.id,
			}),
		);
		await user.click(
			screen.getByRole("button", {
				name: new RegExp(`Organization ${MockOrganization2.display_name}`),
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(`${MockDefaultOrganization.display_name}$`),
			}),
		);
		expect(new URLSearchParams(router.state.location.search).get("org")).toBe(
			MockDefaultOrganization.name,
		);
		expect(new URLSearchParams(router.state.location.search).get("keep")).toBe(
			"value",
		);
		await user.click(
			screen.getByRole("combobox", { name: /^Root agent model behavior/ }),
		);
		await user.click(
			within(
				screen.getByRole("region", { name: "Root agent model" }),
			).getByRole("button", { name: "Save" }),
		);
		expect(save).toHaveBeenCalledTimes(1);
		await act(async () => pendingSave.resolve());
		await waitFor(() => expect(toast.success).toHaveBeenCalled());
		expect(
			queryClient.getQueryState(
				userChatPersonalModelOverrides(MockOrganization2.id).queryKey,
			)?.isInvalidated,
		).toBe(true);
		await user.click(
			screen.getByRole("combobox", { name: /^Root agent model behavior/ }),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockChatModel.display_name),
			}),
		);
		await user.click(
			within(
				screen.getByRole("region", { name: "Root agent model" }),
			).getByRole("button", { name: "Save" }),
		);
		await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
		expect(save).toHaveBeenLastCalledWith(
			MockDefaultOrganization.id,
			"me",
			"root",
			{
				mode: "model",
				model_config_id: MockChatModel.id,
			},
		);
	});
});
