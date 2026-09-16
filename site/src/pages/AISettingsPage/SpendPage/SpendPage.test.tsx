import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import type {
	OrganizationAISpendReport,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
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

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: { ...MockNoPermissions, viewAnyAIBridgeInterception: true },
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
	const spendSpy = vi
		.spyOn(API, "getOrganizationAISpendUsers")
		.mockImplementation(async (_organizationId, params) => ({
			...MockOrganizationAISpendReport,
			...period,
			count: users.length,
			totals: { cost_micros: 30_000_000, unpriced_usage_count: 0 },
			users: users.slice(
				params.offset ?? 0,
				(params.offset ?? 0) + (params.limit ?? 10),
			),
			...report,
		}));
	vi.spyOn(API.experimental, "listAIProviders").mockResolvedValue(
		MockAIProviders,
	);
	vi.spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
	vi.spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
	const router = createMemoryRouter(
		[{ path: "/ai/settings/spend", element: <SpendPage now={fixedNow} /> }],
		{ initialEntries: [`/ai/settings/spend?${search}`] },
	);
	renderWithRouter(router);
	return { router, spendSpy };
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

it("falls back to the default organization when the requested one is not permitted", async () => {
	const { spendSpy } = renderSpend(`${initialSearch}&organization=missing`);
	await screen.findByRole("table", { name: "Spend by user" });
	expect(spendSpy).toHaveBeenCalledWith(
		MockOrganization.id,
		expect.objectContaining(period),
	);
	expect(spendSpy).not.toHaveBeenCalledWith(
		MockOrganization2.id,
		expect.anything(),
	);
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
	const { spendSpy } = renderSpend(initialSearch, {
		retention_start: fixedNow.subtract(10, "day").toISOString(),
	});
	await screen.findByRole("table", { name: "Spend by user" });
	const picker = screen.getByRole("button", {
		name: /Feb 10, 2026.*Mar 11, 2026/,
	});

	const loadedReport = await spendSpy.mock.results[0].value;
	let deliverReport = () => {};
	spendSpy.mockImplementationOnce(
		() =>
			new Promise((resolve) => {
				deliverReport = () => resolve(loadedReport);
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
	expect(picker).toBeDisabled();

	deliverReport();
	await waitFor(() => expect(picker).toBeEnabled());
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

it("opens the count badge lists from the pointer and the keyboard", async () => {
	const user = userEvent.setup();
	renderSpend();
	await screen.findByRole("table", { name: "Spend by user" });
	const [providerBadge] = screen.getAllByRole("button", {
		name: "2 providers",
	});
	const [clientBadge] = screen.getAllByRole("button", { name: "2 clients" });
	const [modelBadge] = screen.getAllByRole("button", { name: "2 models" });

	await user.hover(providerBadge);
	await screen.findByRole("tooltip");
	await user.unhover(providerBadge);

	act(() => clientBadge.focus());
	await screen.findByRole("tooltip");
	act(() => clientBadge.blur());

	act(() => modelBadge.focus());
	await screen.findByRole("tooltip");
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
