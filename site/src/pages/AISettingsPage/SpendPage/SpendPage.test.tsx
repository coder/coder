import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import {
	MockAIGatewaySpendUser,
	MockAIGatewaySpendUserSummary,
	MockAIProviders,
	MockEntitlements,
	MockNoPermissions,
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
	vi.useRealTimers();
	vi.restoreAllMocks();
});

const fixedNow = dayjs("2026-03-12T12:00:00Z");
const dateWindow = {
	start_date: "2026-02-10T00:00:00.000Z",
	end_date: "2026-03-12T00:00:00.000Z",
};
const initialSearch = new URLSearchParams({
	startDate: dateWindow.start_date,
	endDate: dateWindow.end_date,
}).toString();

function renderSpend(search = initialSearch) {
	const users = Array.from({ length: 12 }, (_, i) => ({
		...MockAIGatewaySpendUser,
		id: `user-${i + 1}`,
		username: `user${String(i + 1).padStart(2, "0")}`,
		name: `User ${i + 1}`,
	}));
	const usersSpy = vi
		.spyOn(API, "getAIGatewaySpendUsers")
		.mockImplementation(async (params) => ({
			...dateWindow,
			count: users.length,
			users: users.slice(
				params.offset ?? 0,
				(params.offset ?? 0) + (params.limit ?? 10),
			),
		}));
	const summarySpy = vi
		.spyOn(API, "getAIGatewaySpendSummary")
		.mockResolvedValue(MockAIGatewaySpendUserSummary);
	const userSummarySpy = vi
		.spyOn(API, "getAIGatewaySpendUserSummary")
		.mockResolvedValue(MockAIGatewaySpendUserSummary);
	vi.spyOn(API, "getUser").mockImplementation(async (id) => ({
		...MockUserMember,
		...users.find((user) => user.id === id),
	}));
	vi.spyOn(API.experimental, "listAIProviders").mockResolvedValue(
		MockAIProviders,
	);
	vi.spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
	vi.spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
	const router = createMemoryRouter(
		[
			{ path: "/before", element: <div /> },
			{ path: "/ai/settings/spend", element: <SpendPage now={fixedNow} /> },
		],
		{ initialEntries: ["/before", `/ai/settings/spend?${search}`] },
	);
	renderWithRouter(router);
	return { router, usersSpy, summarySpy, userSummarySpy };
}

// The workspaces-style combobox replaces the old dropdowns: open the menu,
// pick the Provider category, then choose an option.
async function filterByOpenAIProvider(
	user: ReturnType<typeof userEvent.setup>,
) {
	await user.click(
		await screen.findByRole("button", { name: "Toggle filters" }),
	);
	await user.click(await screen.findByRole("option", { name: "Provider" }));
	await user.click(await screen.findByRole("option", { name: /OpenAI/ }));
}

it("resets pagination when toggling or changing the server-side sort", async () => {
	const user = userEvent.setup();
	const { router, usersSpy } = renderSpend(`${initialSearch}&page=2`);
	const table = within(
		await screen.findByRole("table", { name: "Spend by user" }),
	);
	expect(usersSpy).toHaveBeenCalledWith(
		expect.objectContaining({
			sort_by: "total_cost_micros",
			sort_order: "desc",
			offset: 10,
			limit: 10,
		}),
	);
	await user.click(table.getByRole("button", { name: "Cost" }));
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				sort_by: "total_cost_micros",
				sort_order: "asc",
				offset: 0,
			}),
		),
	);
	expect(
		new URLSearchParams(router.state.location.search).get("page"),
	).toBeNull();
	expect(table.getByRole("columnheader", { name: "Cost" })).toHaveAttribute(
		"aria-sort",
		"ascending",
	);
	await user.click(table.getByRole("button", { name: "Requests" }));
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				sort_by: "request_count",
				sort_order: "desc",
				offset: 0,
			}),
		),
	);
	expect(table.getByRole("columnheader", { name: "Requests" })).toHaveAttribute(
		"aria-sort",
		"descending",
	);
	expect(table.getByRole("columnheader", { name: "Cost" })).toHaveAttribute(
		"aria-sort",
		"none",
	);
});

it("applies a date preset to users and deployment summary and resets pagination", async () => {
	const user = userEvent.setup();
	const { router, usersSpy, summarySpy } = renderSpend(
		`${initialSearch}&page=2`,
	);
	await screen.findByRole("table", { name: "Spend by user" });
	await user.click(
		screen.getByRole("button", { name: /Feb 10, 2026.*Mar 11, 2026/ }),
	);
	await user.click(await screen.findByRole("button", { name: "Last 7 days" }));
	const window = {
		start_date: fixedNow.subtract(6, "day").startOf("day").toISOString(),
		end_date: fixedNow.add(1, "hour").startOf("hour").toISOString(),
	};
	await waitFor(() =>
		expect(summarySpy).toHaveBeenLastCalledWith(
			expect.objectContaining(window),
		),
	);
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			expect.objectContaining({ ...window, offset: 0 }),
		),
	);
	const params = new URLSearchParams(router.state.location.search);
	expect(params.get("startDate")).toBe(window.start_date);
	expect(params.get("endDate")).toBe(window.end_date);
	expect(params.get("page")).toBeNull();
});

it("debounces and trims user search without filtering the deployment summary", async () => {
	const { router, usersSpy, summarySpy } = renderSpend(
		`${initialSearch}&page=2`,
	);
	const search = await screen.findByRole("combobox", {
		name: "Search and filter spend\u2026",
	});
	await screen.findByRole("table", { name: "Spend by user" });
	usersSpy.mockClear();
	summarySpy.mockClear();
	vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
	const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
	// The combobox owns the debounce, so the URL and query stay put mid-type.
	await user.type(search, " user01 ");
	await act(() => vi.advanceTimersByTimeAsync(299));
	expect(
		new URLSearchParams(router.state.location.search).get("search"),
	).toBeNull();
	expect(
		usersSpy.mock.calls.every(([params]) => params.search === undefined),
	).toBe(true);
	await act(() => vi.advanceTimersByTimeAsync(1));
	// The leading/trailing whitespace is trimmed before it reaches the URL.
	expect(new URLSearchParams(router.state.location.search).get("search")).toBe(
		"user01",
	);
	expect(
		new URLSearchParams(router.state.location.search).get("page"),
	).toBeNull();
	// Real timers again so waitFor can poll the react-query refetch.
	vi.useRealTimers();
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			expect.objectContaining({ search: "user01", offset: 0 }),
		),
	);
	expect(summarySpy).not.toHaveBeenCalled();
});

it("preserves the provider filter through drill-in and pops Back to the sorted list", async () => {
	const user = userEvent.setup();
	const { router, usersSpy, summarySpy, userSummarySpy } = renderSpend(
		`${initialSearch}&page=2&sort_by=request_count&sort_order=asc`,
	);
	await screen.findByRole("table", { name: "Spend by user" });
	await filterByOpenAIProvider(user);
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			expect.objectContaining({ provider_name: "openai", offset: 0 }),
		),
	);
	await waitFor(() =>
		expect(summarySpy).toHaveBeenLastCalledWith(
			expect.objectContaining({ ...dateWindow, provider_name: "openai" }),
		),
	);
	expect(
		new URLSearchParams(router.state.location.search).get("page"),
	).toBeNull();
	const listLocation = router.state.location;
	await user.click(await screen.findByRole("link", { name: "User 1" }));
	await waitFor(() =>
		expect(userSummarySpy).toHaveBeenCalledWith(
			"user-1",
			expect.objectContaining({ ...dateWindow, provider_name: "openai" }),
		),
	);
	expect(router.state.historyAction).toBe("PUSH");
	expect(new URLSearchParams(router.state.location.search).get("user")).toBe(
		"user-1",
	);
	expect(
		new URLSearchParams(router.state.location.search).get("provider_name"),
	).toBe("openai");
	await user.click(screen.getByRole("button", { name: "Back" }));
	await waitFor(() => expect(router.state.location).toEqual(listLocation));
	expect(router.state.historyAction).toBe("POP");
	const table = within(
		await screen.findByRole("table", { name: "Spend by user" }),
	);
	expect(table.getByRole("columnheader", { name: "Requests" })).toHaveAttribute(
		"aria-sort",
		"ascending",
	);
	await act(() => router.navigate(-1));
	expect(router.state.location.pathname).toBe("/before");
});

it("replaces a direct drill-in with the list without adding history", async () => {
	const user = userEvent.setup();
	const { router } = renderSpend(
		`${initialSearch}&user=user-1&provider_name=openai`,
	);
	await user.click(await screen.findByRole("button", { name: "Back" }));
	expect(router.state.historyAction).toBe("REPLACE");
	expect(
		new URLSearchParams(router.state.location.search).get("user"),
	).toBeNull();
	expect(
		new URLSearchParams(router.state.location.search).get("provider_name"),
	).toBe("openai");
	await act(() => router.navigate(-1));
	expect(router.state.location.pathname).toBe("/before");
});

it("replaces a drill-in when its filters differ from the originating list", async () => {
	const user = userEvent.setup();
	const { router, usersSpy, userSummarySpy } = renderSpend();
	await user.click(await screen.findByRole("link", { name: "User 1" }));
	await screen.findByRole("link", { name: "View sessions" });
	await filterByOpenAIProvider(user);
	await waitFor(() =>
		expect(userSummarySpy).toHaveBeenCalledWith(
			"user-1",
			expect.objectContaining({ provider_name: "openai" }),
		),
	);
	await user.click(screen.getByRole("button", { name: "Back" }));
	expect(router.state.historyAction).toBe("REPLACE");
	expect(
		new URLSearchParams(router.state.location.search).get("user"),
	).toBeNull();
	expect(
		new URLSearchParams(router.state.location.search).get("provider_name"),
	).toBe("openai");
	await waitFor(() =>
		expect(usersSpy).toHaveBeenCalledWith(
			expect.objectContaining({ provider_name: "openai", offset: 0 }),
		),
	);
	await act(() => router.navigate(-1));
	expect(router.state.location.search).toBe(`?${initialSearch}`);
	await act(() => router.navigate(-1));
	expect(router.state.location.pathname).toBe("/before");
});
