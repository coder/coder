import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
import { saveAs } from "file-saver";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import { paginatedOrganizationAISpend } from "#/api/queries/aiBridge";
import type {
	OrganizationAISpendReport,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import type { Permissions } from "#/modules/permissions";
import {
	MockAIProviders,
	MockEntitlements,
	MockGroup,
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderHookWithAuth } from "#/testHelpers/hooks";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import SpendPage from "./SpendPage";

const sessionViewerPermissions: Permissions = {
	...MockNoPermissions,
	viewAnyAIBridgeInterception: true,
};
const auth = { permissions: sessionViewerPermissions };
vi.mock("file-saver", () => ({ saveAs: vi.fn() }));
vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: auth.permissions,
	}),
}));
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
	auth.permissions = sessionViewerPermissions;
});

const fixedNow = dayjs("2026-03-12T12:00:00Z");
const period = {
	period_start: "2026-02-10T00:00:00.000Z",
	period_end: "2026-03-12T00:00:00.000Z",
};
const initialSearch = new URLSearchParams({
	startDate: period.period_start,
	endDate: period.period_end,
}).toString();

function mockSpendApi(
	report: Partial<OrganizationAISpendReport> = {},
	canManageModelPrices = false,
) {
	const users: OrganizationAISpendUser[] = Array.from(
		{ length: 12 },
		(_, i) => ({
			...MockOrganizationAISpendUser,
			user_id: `user-${i + 1}`,
			username: `user${String(i + 1).padStart(2, "0")}`,
			name: `User ${i + 1}`,
		}),
	);
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockOrganization,
		MockOrganization2,
	]);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async (req) =>
		"readModelPrices" in req.checks
			? {
					readModelPrices: canManageModelPrices,
					updateModelPrices: canManageModelPrices,
				}
			: { [MockOrganization.id]: true, [MockOrganization2.id]: true },
	);
	const buildReport = (
		params: Parameters<typeof API.getOrganizationAISpendUsers>[1],
	): OrganizationAISpendReport => ({
		...MockOrganizationAISpendReport,
		...period,
		count: users.length,
		totals: { cost_micros: 30_000_000, unpriced_usage_count: 0 },
		users: users.slice(
			params.offset ?? 0,
			(params.offset ?? 0) + (params.limit ?? 10),
		),
		...report,
	});
	const spendSpy = vi
		.spyOn(API, "getOrganizationAISpendUsers")
		.mockImplementation(async (_organizationId, params) => buildReport(params));
	return { spendSpy, buildReport, users };
}

function renderSpend(
	search = initialSearch,
	report: Partial<OrganizationAISpendReport> = {},
	canManageModelPrices = false,
) {
	const { spendSpy, buildReport, users } = mockSpendApi(
		report,
		canManageModelPrices,
	);
	vi.spyOn(API, "getAIBridgeProviders").mockResolvedValue(MockAIProviders);
	vi.spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
	vi.spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
	vi.spyOn(API, "getUsers").mockImplementation(async ({ q = "" }) => {
		const normalizedQuery = q.toLowerCase();
		const matched = users
			.map((spendUser) => ({
				...MockUserMember,
				id: spendUser.user_id,
				username: spendUser.username,
				name: spendUser.name,
			}))
			.filter((candidate) =>
				candidate.username.toLowerCase().includes(normalizedQuery),
			);
		return { users: matched, count: matched.length };
	});
	const router = createMemoryRouter(
		[
			{
				path: "/ai/settings/spend",
				element: <SpendPage now={fixedNow.toDate()} />,
			},
		],
		{ initialEntries: [`/ai/settings/spend?${search}`] },
	);
	renderWithRouter(router);
	return { router, spendSpy, buildReport };
}

/** Text of each body row in the spend table. */
const spendRows = () =>
	within(screen.getByRole("table", { name: "Spend by user" }))
		.getAllByRole("row")
		.slice(1)
		.map((row) => row.textContent ?? "");

const searchParam = (
	router: ReturnType<typeof createMemoryRouter>,
	key: string,
) => new URLSearchParams(router.state.location.search).get(key);

it("requests the default organization and switches organizations from the first page", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	expect(spendSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining({ ...period, offset: 10, limit: 10 }),
	);

	await user.click(
		screen.getByRole("combobox", { name: "Search and filter users…" }),
	);
	await user.click(await screen.findByRole("option", { name: "Organization" }));
	await user.click(
		await screen.findByRole("button", { name: MockOrganization2.display_name }),
	);
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization2.id,
			expect.objectContaining({ ...period, offset: 0 }),
		),
	);
	expect(searchParam(router, "filter")).toBe(`org:${MockOrganization2.name}`);
	expect(searchParam(router, "page")).toBeNull();
});

it("requests the last 7 days when the URL has no dates", async () => {
	const { spendSpy } = renderSpend("");
	await screen.findByRole("table", { name: "Spend by user" });
	expect(spendSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining({
			period_start: fixedNow.subtract(7, "day").toISOString(),
			period_end: fixedNow.toISOString(),
			offset: 0,
			limit: 10,
		}),
	);
});

it("requests no spend for a denied organization until another one is picked", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend(`${initialSearch}&org=missing`);
	await screen.findByRole("alert");
	expect(spendSpy).not.toHaveBeenCalled();

	await user.click(
		screen.getByRole("button", { name: /Select an organization/ }),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization2.id,
			expect.objectContaining(period),
		),
	);
	expect(spendSpy).not.toHaveBeenCalledWith(
		MockOrganization.id,
		expect.anything(),
	);
	expect(searchParam(router, "filter")).toBe(`org:${MockOrganization2.name}`);
});

it("applies a date preset and resets pagination", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(
		screen.getByRole("button", { name: /February 10.*March 12/ }),
	);
	await user.click(await screen.findByRole("radio", { name: "Last 7 days" }));
	const window = {
		period_start: fixedNow.subtract(7, "day").toISOString(),
		period_end: fixedNow.toISOString(),
	};
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ ...window, offset: 0 }),
		),
	);
	expect(searchParam(router, "startDate")).toBe(window.period_start);
	expect(searchParam(router, "endDate")).toBe(window.period_end);
	expect(searchParam(router, "page")).toBeNull();
});

it("keeps the retention bound while a filtered report is pending", async () => {
	const user = userEvent.setup({ skipHover: true });
	const retentionStart = fixedNow.subtract(10, "day");
	const { spendSpy } = renderSpend(initialSearch, {
		retention_start: retentionStart.toISOString(),
	});
	await screen.findByRole("table", { name: "Spend by user" });

	// Leave the refiltered report pending so its retention bound never arrives.
	spendSpy.mockImplementationOnce(() => new Promise(() => {}));
	await user.click(
		screen.getByRole("combobox", { name: "Search and filter users…" }),
	);
	await user.click(await screen.findByRole("option", { name: "Provider" }));
	await user.click(await screen.findByRole("button", { name: /OpenAI/ }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ provider_name: "openai" }),
		),
	);

	// The picker stays usable with the bound the first report delivered, so
	// every preset it offers requests a period that starts within retention.
	const requestsBeforePicking = spendSpy.mock.calls.length;
	await user.click(
		screen.getByRole("button", { name: /February 10.*March 12/ }),
	);
	const offered = screen
		.getAllByRole("radio", { name: /^Last \d+ (days|hours)$/ })
		.map((preset) => preset.textContent ?? "");
	for (const label of offered) {
		const requestsBeforePreset = spendSpy.mock.calls.length;
		await user.click(screen.getByRole("radio", { name: label }));
		await waitFor(() =>
			expect(spendSpy.mock.calls.length).toBeGreaterThan(requestsBeforePreset),
		);
		// The trigger shows the applied preset once the URL updates; reopen it
		// for the next one.
		await user.click(await screen.findByRole("button", { name: label }));
	}
	const presetRequests = spendSpy.mock.calls.slice(requestsBeforePicking);
	expect(presetRequests).toHaveLength(offered.length);
	expect(presetRequests.length).toBeGreaterThan(0);
	for (const [, params] of presetRequests) {
		expect(params.provider_name).toBe("openai");
		expect(dayjs(params.period_start).isBefore(retentionStart)).toBe(false);
	}
	expect(spendSpy).not.toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining({
			period_start: fixedNow.subtract(30, "day").toISOString(),
		}),
	);
});

it("applies a second range from the keyboard after the first one resolves", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { spendSpy } = renderSpend();
	await screen.findByRole("table", { name: "Spend by user" });

	await user.click(
		screen.getByRole("button", { name: /February 10.*March 12/ }),
	);
	await user.click(await screen.findByRole("radio", { name: "Last 7 days" }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({
				period_start: fixedNow.subtract(7, "day").toISOString(),
			}),
		),
	);
	await screen.findByRole("button", { name: "Last 7 days" });

	// Focus returned to the picker when its popover closed, so Enter reopens it.
	await user.keyboard("{Enter}");
	await user.click(await screen.findByRole("radio", { name: "Last 24 hours" }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({
				period_start: fixedNow.subtract(24, "hour").toISOString(),
			}),
		),
	);
});

it("shows spend without dimension filters to viewers who cannot read AI sessions", async () => {
	auth.permissions = MockNoPermissions;
	const { spendSpy } = renderSpend(`${initialSearch}&filter=provider%3Aopenai`);
	await screen.findByRole("table", { name: "Spend by user" });
	expect(API.getAIBridgeProviders).not.toHaveBeenCalled();
	expect(API.getAIBridgeModels).not.toHaveBeenCalled();
	expect(API.getAIBridgeClients).not.toHaveBeenCalled();
	expect(spendSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining(period),
	);
	expect(spendSpy.mock.calls[0][1]).not.toHaveProperty("provider_name");
});

it("applies user and unconfigured pricing filters", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });

	const filterInput = screen.getByRole("combobox", {
		name: "Search and filter users…",
	});
	await user.click(filterInput);
	await user.click(await screen.findByRole("option", { name: "User" }));
	await user.click(await screen.findByRole("button", { name: /user01/ }));
	await waitFor(() =>
		expect(searchParam(router, "filter")).toBe("user:user01"),
	);
	await waitFor(() =>
		expect(spendRows()).toEqual([expect.stringContaining("@user01")]),
	);
	expect(searchParam(router, "page")).toBeNull();

	// The endpoint does not accept these filters yet, so every user is loaded
	// and filtered in the browser.
	expect(spendSpy).toHaveBeenLastCalledWith(
		MockOrganization.id,
		expect.objectContaining({ limit: 100 }),
	);
	expect(spendSpy.mock.calls.at(-1)?.[1]).not.toHaveProperty("username");

	// No mocked user has unpriced usage.
	await user.click(filterInput);
	await user.click(
		await screen.findByRole("option", {
			name: "Uses models with unconfigured pricing",
		}),
	);
	await waitFor(() =>
		expect(searchParam(router, "filter")).toBe(
			"user:user01 pricing:unconfigured",
		),
	);
	await screen.findByText("No AI Gateway spend found");
	expect(spendSpy.mock.calls.at(-1)?.[1]).not.toHaveProperty("pricing");
});

it("lists unpriced models and links admins to set their pricing", async () => {
	const users = [
		{
			...MockOrganizationAISpendUser,
			user_id: "user-1",
			username: "user01",
			name: "User 1",
			providers: ["openai"],
			models: ["gpt-5.4", "gpt-custom"],
			unpriced_usage_count: 2,
		},
	];
	vi.spyOn(API.experimental, "getAIModelPrices").mockResolvedValue([
		{
			provider: "openai",
			model: "gpt-5.4",
			input_price: 1,
			output_price: 2,
			cache_read_price: null,
			cache_write_price: null,
			source: "default",
			created_at: "",
			updated_at: "",
		},
	]);
	const { spendSpy } = renderSpend(
		initialSearch,
		{
			count: 1,
			totals: { cost_micros: 2_500_000, unpriced_usage_count: 2 },
			users,
		},
		true,
	);
	const user = userEvent.setup();

	await screen.findByRole("table", { name: "Spend by user" });
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ limit: 100 }),
		),
	);
	await user.hover(
		screen.getByRole("button", { name: "Model pricing missing for User 1" }),
	);
	const tooltip = await screen.findByRole("tooltip");
	await waitFor(() =>
		expect(
			within(tooltip).getByRole("list", { name: "Models without pricing" }),
		).toHaveTextContent(/^gpt-custom$/),
	);
	expect(
		within(tooltip).getByRole("link", { name: "Set pricing for these models" }),
	).toHaveAttribute("href", `/ai/settings/models?org=${MockOrganization.name}`);
});

it("filters by group members and unconfigured pricing in the browser", async () => {
	const users = Array.from({ length: 4 }, (_, i) => ({
		...MockOrganizationAISpendUser,
		user_id: `user-${i + 1}`,
		username: `user${String(i + 1).padStart(2, "0")}`,
		name: `User ${i + 1}`,
		unpriced_usage_count: i % 2 === 0 ? 3 : 0,
	}));
	vi.spyOn(API, "getGroup").mockResolvedValue({
		...MockGroup,
		name: "devs",
		members: ["user-1", "user-2", "user-4"].map((id) => ({
			...MockUserMember,
			id,
		})),
	});
	const search = new URLSearchParams(initialSearch);
	search.set("filter", "group:devs pricing:unconfigured");
	renderSpend(search.toString(), { count: users.length, users });

	await waitFor(() =>
		expect(spendRows()).toEqual([expect.stringContaining("@user01")]),
	);
	expect(API.getGroup).toHaveBeenCalledWith(MockOrganization.id, "devs", {
		exclude_members: false,
	});
});

it("applies the organization filter", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });

	await user.click(
		screen.getByRole("combobox", { name: "Search and filter users…" }),
	);
	await user.click(await screen.findByRole("option", { name: "Organization" }));
	await user.click(
		await screen.findByRole("button", { name: MockOrganization2.display_name }),
	);

	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization2.id,
			expect.objectContaining({ offset: 0 }),
		),
	);
	expect(searchParam(router, "filter")).toBe(`org:${MockOrganization2.name}`);
	expect(searchParam(router, "page")).toBeNull();
});

it("applies the provider filter and resets pagination", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(
		screen.getByRole("combobox", { name: "Search and filter users…" }),
	);
	await user.click(await screen.findByRole("option", { name: "Provider" }));
	await user.click(await screen.findByRole("button", { name: /OpenAI/ }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ provider_name: "openai", offset: 0 }),
		),
	);
	expect(searchParam(router, "filter")).toBe("provider:openai");
	expect(searchParam(router, "page")).toBeNull();
});

it("exports the filtered period as CSV", async () => {
	const user = userEvent.setup({ skipHover: true });
	const csv = new Blob(["user_id,username\n"], { type: "text/csv" });
	const exportSpy = vi
		.spyOn(API, "exportOrganizationAISpend")
		.mockResolvedValue(csv);
	vi.spyOn(API, "getUser").mockResolvedValue({
		...MockUserMember,
		id: "user-1",
		username: "user01",
	});
	vi.spyOn(API, "getGroup").mockResolvedValue({
		...MockGroup,
		name: "devs",
		members: [{ ...MockUserMember, id: "user-1" }],
	});
	const search = new URLSearchParams(initialSearch);
	search.set("filter", "provider:openai user:user01 group:devs");
	renderSpend(search.toString());
	await screen.findByRole("table", { name: "Spend by user" });

	await user.click(screen.getByRole("button", { name: "Export CSV" }));

	await waitFor(() =>
		expect(saveAs).toHaveBeenCalledWith(
			csv,
			`ai-spend-export-${MockOrganization.name}-2026-02-10-to-2026-03-12.csv`,
		),
	);
	expect(exportSpy).toHaveBeenCalledWith(MockOrganization.id, {
		...period,
		provider_name: "openai",
		model: undefined,
		user_id: "user-1",
		group_id: MockGroup.id,
	});
	expect(API.getUser).toHaveBeenCalledWith("user01");
});

it("requests the next page offset", async () => {
	const user = userEvent.setup({ skipHover: true });
	const { router, spendSpy } = renderSpend();
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(screen.getByRole("button", { name: "Next page" }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ offset: 10, limit: 10 }),
		),
	);
	expect(searchParam(router, "page")).toBe("2");
});

it("keeps the loaded page while paging but not across organizations", async () => {
	const { spendSpy, buildReport } = mockSpendApi();
	const { result, rerender } = await renderHookWithAuth(
		({ organizationId }) =>
			usePaginatedQuery({
				...paginatedOrganizationAISpend(organizationId, period),
				recordsPerPage: 10,
			}),
		{
			renderOptions: { initialProps: { organizationId: MockOrganization.id } },
		},
	);
	const firstPage = buildReport({ offset: 0, limit: 10 });
	await waitFor(() => expect(result.current.data).toEqual(firstPage));

	let deliverPage = () => {};
	spendSpy.mockImplementationOnce(
		(_organizationId, params) =>
			new Promise((resolve) => {
				deliverPage = () => resolve(buildReport(params));
			}),
	);
	act(() => result.current.goToNextPage());
	await waitFor(() => expect(result.current.isPlaceholderData).toBe(true));
	expect(result.current.data).toEqual(firstPage);
	deliverPage();
	await waitFor(() =>
		expect(result.current.data).toEqual(buildReport({ offset: 10, limit: 10 })),
	);

	spendSpy.mockImplementation(() => new Promise(() => {}));
	await rerender({ organizationId: MockOrganization2.id });
	expect(result.current.isLoading).toBe(true);
	expect(result.current.data).toBeUndefined();
});
