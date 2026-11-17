import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import {
	MockEntitlements,
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendOverview,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import AISpendPage from "./AISpendPage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: MockNoPermissions,
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

const now = new Date("2026-03-12T15:30:00Z");
const path = "/ai/settings/spend/experimental";

function renderAISpendPage(search = "") {
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockOrganization,
		MockOrganization2,
	]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
		[MockOrganization2.id]: true,
	});
	const overviewSpy = vi
		.spyOn(API.experimental, "getOrganizationAISpendOverview")
		.mockResolvedValue(MockOrganizationAISpendOverview);
	const router = createMemoryRouter(
		[{ path, element: <AISpendPage now={now} /> }],
		{ initialEntries: [`${path}?${search}`] },
	);
	renderWithRouter(router);
	const searchParam = (key: string) =>
		new URLSearchParams(router.state.location.search).get(key);
	return { overviewSpy, searchParam };
}

it("requests the last 7 days as UTC days when the URL has no dates", async () => {
	const { overviewSpy, searchParam } = renderAISpendPage();
	await waitFor(() =>
		expect(overviewSpy).toHaveBeenCalledWith(MockOrganization.id, {
			period_start: "2026-03-06T00:00:00.000Z",
			period_end: "2026-03-13T00:00:00.000Z",
		}),
	);
	expect(searchParam("startDate")).toBeNull();
	expect(searchParam("endDate")).toBeNull();
});

it("writes a picked range to the URL as days and requests it", async () => {
	const user = userEvent.setup();
	const { overviewSpy, searchParam } = renderAISpendPage(
		"startDate=2026-03-01&endDate=2026-03-03",
	);
	await waitFor(() =>
		expect(overviewSpy).toHaveBeenCalledWith(MockOrganization.id, {
			period_start: "2026-03-01T00:00:00.000Z",
			period_end: "2026-03-04T00:00:00.000Z",
		}),
	);

	await user.click(screen.getByRole("button", { name: /Mar 1, 2026/ }));
	await user.click(screen.getByRole("button", { name: "Last 14 days" }));
	await waitFor(() =>
		expect(overviewSpy).toHaveBeenCalledWith(MockOrganization.id, {
			period_start: "2026-02-27T00:00:00.000Z",
			period_end: "2026-03-13T00:00:00.000Z",
		}),
	);
	expect(searchParam("startDate")).toBe("2026-02-27");
	expect(searchParam("endDate")).toBe("2026-03-12");
});

it("requests the selected organization's spend", async () => {
	const user = userEvent.setup();
	const { overviewSpy, searchParam } = renderAISpendPage();
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
		),
	);
	expect(searchParam("org")).toBe(MockOrganization2.name);
});

it("requests no spend for an organization the viewer cannot see", async () => {
	const { overviewSpy } = renderAISpendPage("org=missing");
	await screen.findByRole("combobox", { name: "Organization" });
	expect(overviewSpy).not.toHaveBeenCalled();
});
