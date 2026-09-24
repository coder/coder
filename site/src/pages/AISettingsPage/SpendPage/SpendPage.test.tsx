import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
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

function mockSpendApi(report: Partial<OrganizationAISpendReport> = {}) {
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
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
		[MockOrganization2.id]: true,
	});
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
	return { spendSpy, buildReport };
}

function renderSpend(
	search = initialSearch,
	report: Partial<OrganizationAISpendReport> = {},
) {
	const { spendSpy, buildReport } = mockSpendApi(report);
	vi.spyOn(API, "getAIBridgeProviders").mockResolvedValue(MockAIProviders);
	vi.spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
	vi.spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
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

const searchParam = (
	router: ReturnType<typeof createMemoryRouter>,
	key: string,
) => new URLSearchParams(router.state.location.search).get(key);

it("requests the default organization and switches organizations from the first page", async () => {
	const user = userEvent.setup();
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	expect(spendSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining({ ...period, offset: 10, limit: 10 }),
	);

	await user.click(
		screen.getByRole("button", {
			name: `Organization ${MockOrganization.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization2.id,
			expect.objectContaining({ ...period, offset: 0 }),
		),
	);
	expect(searchParam(router, "org")).toBe(MockOrganization2.name);
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
	const user = userEvent.setup();
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
	expect(searchParam(router, "org")).toBe(MockOrganization2.name);
});

it("applies a date preset and resets pagination", async () => {
	const user = userEvent.setup();
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(screen.getByRole("button", { name: /Feb 10.*Mar 12/ }));
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
	const user = userEvent.setup();
	const retentionStart = fixedNow.subtract(10, "day");
	const { spendSpy } = renderSpend(initialSearch, {
		retention_start: retentionStart.toISOString(),
	});
	await screen.findByRole("table", { name: "Spend by user" });

	// Leave the refiltered report pending so its retention bound never arrives.
	spendSpy.mockImplementationOnce(() => new Promise(() => {}));
	await user.click(screen.getByRole("button", { name: "Select provider" }));
	await user.click(await screen.findByRole("option", { name: /OpenAI/ }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ provider_name: "openai" }),
		),
	);

	// The picker stays usable with the bound the first report delivered, so
	// every preset it offers requests a period that starts within retention.
	const requestsBeforePicking = spendSpy.mock.calls.length;
	await user.click(screen.getByRole("button", { name: /Feb 10.*Mar 12/ }));
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
	const user = userEvent.setup();
	const { spendSpy } = renderSpend();
	await screen.findByRole("table", { name: "Spend by user" });

	await user.click(screen.getByRole("button", { name: /Feb 10.*Mar 12/ }));
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
	const { spendSpy } = renderSpend(`${initialSearch}&provider_name=openai`);
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

it("applies the provider filter and resets pagination", async () => {
	const user = userEvent.setup();
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(screen.getByRole("button", { name: "Select provider" }));
	await user.click(await screen.findByRole("option", { name: /OpenAI/ }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ provider_name: "openai", offset: 0 }),
		),
	);
	expect(searchParam(router, "provider_name")).toBe("openai");
	expect(searchParam(router, "page")).toBeNull();
});

it("requests the next page offset", async () => {
	const user = userEvent.setup();
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
