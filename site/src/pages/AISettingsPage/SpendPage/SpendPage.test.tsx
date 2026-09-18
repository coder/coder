import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import type {
	OrganizationAISpendReport,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
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

function renderSpend(
	search = initialSearch,
	report: Partial<OrganizationAISpendReport> = {},
) {
	const users: OrganizationAISpendUser[] = Array.from(
		{ length: 12 },
		(_, i) => ({
			...MockOrganizationAISpendUser,
			user_id: `user-${i + 1}`,
			username: `user${String(i + 1).padStart(2, "0")}`,
			name: `User ${i + 1}`,
			providers: i === 0 ? ["openai"] : MockOrganizationAISpendUser.providers,
			models: i === 0 ? ["gpt-5.4"] : MockOrganizationAISpendUser.models,
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
	expect(searchParam(router, "organization")).toBe(MockOrganization2.name);
	expect(searchParam(router, "page")).toBeNull();
});

it("requests the server's budget period when the URL has no dates", async () => {
	const { spendSpy } = renderSpend("");
	await screen.findByRole("table", { name: "Spend by user" });
	expect(spendSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining({ offset: 0, limit: 10 }),
	);
	expect(spendSpy.mock.calls[0][1]).not.toHaveProperty("period_start");
	expect(spendSpy.mock.calls[0][1]).not.toHaveProperty("period_end");
});

it("requests no spend for a denied organization until another one is picked", async () => {
	const user = userEvent.setup();
	const { router, spendSpy } = renderSpend(
		`${initialSearch}&organization=missing`,
	);
	await screen.findByRole("alert");
	expect(spendSpy).not.toHaveBeenCalled();

	await user.click(screen.getByRole("button", { name: "Organization" }));
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
	expect(searchParam(router, "organization")).toBe(MockOrganization2.name);
});

it("applies a date preset and resets pagination", async () => {
	const user = userEvent.setup();
	const { router, spendSpy } = renderSpend(`${initialSearch}&page=2`);
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(
		screen.getByRole("button", { name: /Feb 10, 2026.*Mar 11, 2026/ }),
	);
	await user.click(await screen.findByRole("button", { name: "Last 7 days" }));
	const window = {
		period_start: fixedNow.subtract(6, "day").startOf("day").toISOString(),
		period_end: fixedNow.add(1, "hour").startOf("hour").toISOString(),
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

it("holds the date picker until the filtered report brings its retention bound", async () => {
	const user = userEvent.setup();
	const { router, spendSpy, buildReport } = renderSpend(initialSearch, {
		retention_start: fixedNow.subtract(10, "day").toISOString(),
	});
	await screen.findByRole("table", { name: "Spend by user" });
	const picker = screen.getByRole("button", {
		name: /Feb 10, 2026.*Mar 11, 2026/,
	});

	let deliverReport = () => {};
	spendSpy.mockImplementationOnce(
		(_organizationId, params) =>
			new Promise((resolve) => {
				deliverReport = () =>
					resolve({
						...buildReport(params),
						count: 0,
						totals: { cost_micros: 0, unpriced_usage_count: 0 },
						users: [],
					});
			}),
	);
	await user.click(screen.getByRole("button", { name: "Select provider" }));
	await user.click(await screen.findByRole("option", { name: /OpenAI/ }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ provider_name: "openai" }),
		),
	);

	// Without the report's retention bound an open picker would offer this
	// preset, which starts before retention.
	const requestsBeforePicking = spendSpy.mock.calls.length;
	await user.click(picker);
	for (const preset of screen.queryAllByRole("button", {
		name: "Last 30 days",
	})) {
		await user.click(preset);
	}
	expect(spendSpy).toHaveBeenCalledTimes(requestsBeforePicking);
	expect(searchParam(router, "startDate")).toBe(period.period_start);

	deliverReport();
	await screen.findByText("No AI Gateway spend found");
	await user.click(picker);
	await user.click(await screen.findByRole("button", { name: "Last 7 days" }));
	await waitFor(() =>
		expect(spendSpy).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({
				provider_name: "openai",
				period_start: fixedNow.subtract(6, "day").startOf("day").toISOString(),
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

it("keeps the loaded rows while paging but not across organizations", async () => {
	const user = userEvent.setup();
	const { spendSpy, buildReport } = renderSpend();
	await screen.findByRole("table", { name: "Spend by user" });

	let deliverPage = () => {};
	spendSpy.mockImplementationOnce(
		(_organizationId, params) =>
			new Promise((resolve) => {
				deliverPage = () => resolve(buildReport(params));
			}),
	);
	await user.click(screen.getByRole("button", { name: "Next page" }));
	await screen.findByRole("status", { name: "Refreshing spend" });
	screen.getByText("@user01");
	deliverPage();
	await screen.findByText("@user11");

	spendSpy.mockImplementation(() => new Promise(() => {}));
	await user.click(
		screen.getByRole("button", {
			name: `Organization ${MockOrganization.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);
	await screen.findByRole("status", { name: "Loading spend" });
	expect(screen.queryByText("@user11")).toBeNull();
});
