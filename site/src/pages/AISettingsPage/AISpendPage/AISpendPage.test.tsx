import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { saveAs } from "file-saver";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import * as authModule from "#/hooks/useAuthenticated";
import {
	MockEntitlements,
	MockGroup,
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendOverview,
	MockOrganizationAISpendReport,
	MockUnpricedAIModels,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import AISpendPage from "./AISpendPage";
import { TOP_SPENDERS_COUNT } from "./topSpenders";
import { UNPRICED_MODELS_COUNT } from "./unpricedModels";

vi.mock("file-saver", () => ({ saveAs: vi.fn() }));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: {
			...MockEntitlements,
			features: withDefaultFeatures({
				aibridge: { enabled: true, entitlement: "entitled" },
			}),
		},
	}),
}));

afterEach(() => {
	vi.restoreAllMocks();
	vi.mocked(saveAs).mockClear();
});

const now = new Date("2026-03-12T15:30:00Z");
const path = "/ai/settings/spend/experimental";
const topSpenderIds = MockOrganizationAISpendOverview.users.map(
	(user) => user.user_id,
);

function renderAISpendPage(
	search = "",
	{ canViewSessions = false, retentionMs = 0 } = {},
) {
	vi.spyOn(authModule, "useAuthenticated").mockReturnValue({
		user: MockUserMember,
		permissions: {
			...MockNoPermissions,
			viewAnyAIBridgeInterception: canViewSessions,
		},
		isLoading: false,
		isSignedOut: false,
		isSigningOut: false,
		isConfiguringTheFirstUser: false,
		isSignedIn: true,
		isSigningIn: false,
		isUpdatingProfile: false,
		signInError: undefined,
		updateProfileError: undefined,
		signOut: vi.fn(),
		signIn: vi.fn(),
		updateProfile: vi.fn(),
	});
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockOrganization,
		MockOrganization2,
	]);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API.experimental, "getAIGatewayRetention").mockResolvedValue({
		retention_ms: retentionMs,
	});
	const overviewSpy = vi
		.spyOn(API.experimental, "getOrganizationAISpendOverview")
		.mockResolvedValue(MockOrganizationAISpendOverview);
	const unpricedModelsSpy = vi
		.spyOn(API.experimental, "getOrganizationUnpricedAIModels")
		.mockResolvedValue(MockUnpricedAIModels);
	const usersSpy = vi
		.spyOn(API.experimental, "getOrganizationAISpendUsers")
		.mockResolvedValue(MockOrganizationAISpendReport);
	const router = createMemoryRouter(
		[{ path, element: <AISpendPage now={now} /> }],
		{ initialEntries: [`${path}?${search}`] },
	);
	renderWithRouter(router);
	const searchParam = (key: string) =>
		new URLSearchParams(router.state.location.search).get(key);
	return { overviewSpy, unpricedModelsSpy, usersSpy, searchParam };
}

it("requests the last 7 days as UTC days when the URL has no dates", async () => {
	const { overviewSpy, unpricedModelsSpy, usersSpy, searchParam } =
		renderAISpendPage();
	const period = {
		period_start: "2026-03-06T00:00:00.000Z",
		period_end: "2026-03-13T00:00:00.000Z",
	};
	await waitFor(() => {
		expect(overviewSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			period,
			TOP_SPENDERS_COUNT,
		);
		expect(unpricedModelsSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			period,
			UNPRICED_MODELS_COUNT,
		);
		expect(usersSpy).toHaveBeenCalledWith(MockOrganization.id, {
			...period,
			user_ids: topSpenderIds,
			limit: topSpenderIds.length,
		});
	});
	expect(searchParam("startDate")).toBeNull();
	expect(searchParam("endDate")).toBeNull();
});

it("ignores URL days outside the retention", async () => {
	const { overviewSpy } = renderAISpendPage(
		"startDate=2026-03-01&endDate=2026-03-03",
		{ retentionMs: 3 * 24 * 60 * 60 * 1000 },
	);
	await waitFor(() =>
		expect(overviewSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			{
				period_start: "2026-03-10T00:00:00.000Z",
				period_end: "2026-03-13T00:00:00.000Z",
			},
			TOP_SPENDERS_COUNT,
		),
	);
	expect(overviewSpy).toHaveBeenCalledTimes(1);
});

it("writes a picked range to the URL as days and requests it", async () => {
	const user = userEvent.setup();
	const { overviewSpy, unpricedModelsSpy, usersSpy, searchParam } =
		renderAISpendPage("startDate=2026-03-01&endDate=2026-03-03");
	await waitFor(() =>
		expect(overviewSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			{
				period_start: "2026-03-01T00:00:00.000Z",
				period_end: "2026-03-04T00:00:00.000Z",
			},
			TOP_SPENDERS_COUNT,
		),
	);

	await user.click(screen.getByRole("button", { name: /Mar 1, 2026/ }));
	await user.click(screen.getByRole("button", { name: "Last 14 days" }));
	const period = {
		period_start: "2026-02-27T00:00:00.000Z",
		period_end: "2026-03-13T00:00:00.000Z",
	};
	await waitFor(() => {
		expect(overviewSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			period,
			TOP_SPENDERS_COUNT,
		);
		expect(unpricedModelsSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			period,
			UNPRICED_MODELS_COUNT,
		);
		expect(usersSpy).toHaveBeenCalledWith(MockOrganization.id, {
			...period,
			user_ids: topSpenderIds,
			limit: topSpenderIds.length,
		});
	});
	expect(searchParam("startDate")).toBe("2026-02-27");
	expect(searchParam("endDate")).toBe("2026-03-12");
});

it("requests the selected organization's spend and clears the filter", async () => {
	const user = userEvent.setup();
	const { overviewSpy, searchParam } = renderAISpendPage(
		"filter=provider:openai",
	);
	await user.click(
		await screen.findByRole("combobox", {
			name: `Organization ${MockOrganization.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);
	await waitFor(() =>
		expect(overviewSpy).toHaveBeenCalledWith(
			MockOrganization2.id,
			expect.anything(),
			TOP_SPENDERS_COUNT,
		),
	);
	expect(searchParam("org")).toBe(MockOrganization2.name);
	expect(searchParam("filter")).toBeNull();
});

it("keeps the filter when the same organization is selected", async () => {
	const user = userEvent.setup();
	const { searchParam } = renderAISpendPage("filter=provider:openai");
	await user.click(
		await screen.findByRole("combobox", {
			name: `Organization ${MockOrganization.display_name}`,
		}),
	);
	await user.click(await screen.findByRole("option", { selected: true }));
	expect(searchParam("filter")).toBe("provider:openai");
});

it("requests no spend for an organization the viewer cannot see", async () => {
	const { overviewSpy, unpricedModelsSpy } = renderAISpendPage("org=missing");
	await screen.findByRole("combobox", { name: "Organization" });
	expect(overviewSpy).not.toHaveBeenCalled();
	expect(unpricedModelsSpy).not.toHaveBeenCalled();
});

it("keeps the selected tab in the URL", async () => {
	const user = userEvent.setup();
	const { searchParam } = renderAISpendPage();
	await user.click(await screen.findByRole("tab", { name: "Detailed report" }));
	expect(
		await screen.findByRole("heading", { name: "Generate a detailed report" }),
	).toBeInTheDocument();
	expect(searchParam("tab")).toBe("detailed-report");
});

it("shares the picked days between the tabs", async () => {
	const user = userEvent.setup();
	renderAISpendPage("startDate=2026-03-01&endDate=2026-03-03");
	await user.click(await screen.findByRole("tab", { name: "Detailed report" }));
	expect(
		within(
			await screen.findByRole("tabpanel", { name: "Detailed report" }),
		).getByRole("button", { name: /Mar 1, 2026/ }),
	).toBeInTheDocument();
});

it("opens the Detailed report from the Top spenders card", async () => {
	const user = userEvent.setup();
	const { searchParam } = renderAISpendPage();
	await user.click(
		await screen.findByRole("button", { name: "Detailed report" }),
	);
	expect(searchParam("tab")).toBe("detailed-report");
});

it("opens the Detailed report for a top spender without running it", async () => {
	const user = userEvent.setup();
	const { usersSpy, searchParam } = renderAISpendPage();
	const [spender] = MockOrganizationAISpendOverview.users;
	await user.click(
		await screen.findByRole("button", {
			name: `Open menu for ${spender.name || spender.username}`,
		}),
	);
	await user.click(
		await screen.findByRole("menuitem", { name: "Cost details" }),
	);
	await user.click(
		await screen.findByRole("button", { name: "Run detailed report" }),
	);
	expect(searchParam("tab")).toBe("detailed-report");
	expect(searchParam("filter")).toBe(`user:${spender.username}`);
	expect(usersSpy).not.toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining({ offset: 0 }),
		expect.anything(),
	);
});

it("runs the report for the picked days and filter, resolving the user and group to IDs", async () => {
	const user = userEvent.setup();
	const getUserSpy = vi.spyOn(API, "getUser").mockResolvedValue(MockUserOwner);
	const getGroupSpy = vi.spyOn(API, "getGroup").mockResolvedValue(MockGroup);
	const filter =
		'user:admin group:Front-End provider:openai model:gpt-5.4 client:"Claude Code"';
	const { usersSpy } = renderAISpendPage(
		`tab=detailed-report&startDate=2026-03-01&endDate=2026-03-03&filter=${encodeURIComponent(filter)}`,
		{ canViewSessions: true },
	);
	await user.click(await screen.findByRole("button", { name: "Run report" }));
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			{
				period_start: "2026-03-01T00:00:00.000Z",
				period_end: "2026-03-04T00:00:00.000Z",
				provider_name: "openai",
				model: "gpt-5.4",
				client: "Claude Code",
				user_ids: [MockUserOwner.id],
				group_id: MockGroup.id,
				limit: 10,
				offset: 0,
			},
			expect.any(AbortSignal),
		),
	);
	expect(getUserSpy).toHaveBeenCalledWith("admin", expect.any(AbortSignal));
	expect(getGroupSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		"Front-End",
		{ exclude_members: true },
		expect.any(AbortSignal),
	);
});

it("ignores the URL filter when the viewer can't use it", async () => {
	const user = userEvent.setup();
	const getUserSpy = vi.spyOn(API, "getUser");
	const { usersSpy } = renderAISpendPage(
		`tab=detailed-report&startDate=2026-03-01&endDate=2026-03-03&filter=${encodeURIComponent("user:admin provider:openai")}`,
	);
	await user.click(await screen.findByRole("button", { name: "Run report" }));
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			{
				period_start: "2026-03-01T00:00:00.000Z",
				period_end: "2026-03-04T00:00:00.000Z",
				limit: 10,
				offset: 0,
			},
			expect.any(AbortSignal),
		),
	);
	expect(getUserSpy).not.toHaveBeenCalled();
});

// Like a real request, this stays pending until it is aborted.
function pendingUntilAborted(signal?: AbortSignal): Promise<never> {
	return new Promise((_, reject) => {
		signal?.addEventListener("abort", () => reject(signal.reason));
	});
}

it("cancels a report before it has results", async () => {
	const user = userEvent.setup();
	const { usersSpy } = renderAISpendPage("tab=detailed-report");
	let reportSignal: AbortSignal | undefined;
	usersSpy.mockImplementation((_organizationId, _params, signal) => {
		reportSignal = signal;
		return pendingUntilAborted(signal);
	});
	await user.click(await screen.findByRole("button", { name: "Run report" }));
	await user.click(await screen.findByRole("button", { name: "Cancel" }));
	expect(reportSignal?.aborted).toBe(true);
	expect(
		await screen.findByText("Create a detailed report"),
	).toBeInTheDocument();
});

it("cancels a report while it loads another page", async () => {
	const user = userEvent.setup();
	const { usersSpy, searchParam } = renderAISpendPage("tab=detailed-report");
	usersSpy.mockImplementation(async (_organizationId, params, signal) => {
		if ((params.offset ?? 0) > 0) {
			return pendingUntilAborted(signal);
		}
		return { ...MockOrganizationAISpendReport, count: 20 };
	});
	await user.click(await screen.findByRole("button", { name: "Run report" }));
	await user.click(await screen.findByRole("button", { name: "Next page" }));
	await waitFor(() => expect(searchParam("page")).toBe("2"));
	await user.click(await screen.findByRole("button", { name: "Cancel" }));
	await waitFor(() => expect(searchParam("page")).toBe("1"));
});

it("exports the last run report", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getUser").mockResolvedValue(MockUserOwner);
	const file = new Blob(["user_id"], { type: "text/csv" });
	const exportSpy = vi
		.spyOn(API, "getOrganizationAISpendExport")
		.mockResolvedValue(file);
	renderAISpendPage(
		`tab=detailed-report&startDate=2026-03-01&endDate=2026-03-03&filter=${encodeURIComponent("user:admin model:gpt-5.4")}`,
		{ canViewSessions: true },
	);
	await user.click(await screen.findByRole("button", { name: "Run report" }));
	const exportButton = screen.getByRole("button", { name: "Export CSV" });
	await waitFor(() => expect(exportButton).toBeEnabled());
	await user.click(exportButton);
	await waitFor(() =>
		expect(saveAs).toHaveBeenCalledWith(
			file,
			`ai-spend-export-${MockOrganization.name}-2026-03-01-to-2026-03-03.csv`,
		),
	);
	expect(exportSpy).toHaveBeenCalledWith(MockOrganization.id, {
		period_start: "2026-03-01T00:00:00.000Z",
		period_end: "2026-03-04T00:00:00.000Z",
		user_id: MockUserOwner.id,
		group_id: undefined,
		provider_name: undefined,
		model: "gpt-5.4",
	});
});
