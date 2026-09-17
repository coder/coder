import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockAIProviders,
	MockGroup,
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendDetails,
	MockOrganizationAISpendRow,
	MockOrganizationMember,
	MockOrganizationMember2,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import SpendDetailsPage from "./SpendDetailsPage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		permissions: { viewAnyAIBridgeInterception: true },
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: {
			features: {
				aibridge: { enabled: true, entitlement: "entitled" },
			},
		},
	}),
}));

const response = {
	...MockOrganizationAISpendDetails,
	count: 2,
	rows: [MockOrganizationAISpendRow],
};

const renderPage = (
	options: {
		organizations?: readonly (typeof MockOrganization)[];
		search?: string;
		report?: typeof response;
	} = {},
) => {
	const organizations = options.organizations ?? [MockOrganization];
	vi.spyOn(API, "getOrganizations").mockResolvedValue([...organizations]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue(
		Object.fromEntries(
			organizations.map((organization) => [organization.id, true]),
		),
	);
	const details = vi
		.spyOn(API, "getOrganizationAISpendDetails")
		.mockResolvedValue(options.report ?? response);
	const exportCSV = vi
		.spyOn(API, "exportOrganizationAISpend")
		.mockResolvedValue(new Blob(["csv"]));
	vi.spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
		members: [MockOrganizationMember, MockOrganizationMember2],
		count: 2,
	});
	vi.spyOn(API, "getOrganizationPaginatedGroups").mockResolvedValue({
		groups: [MockGroup],
		count: 1,
	});
	vi.spyOn(API.experimental, "listAIProviders").mockResolvedValue(
		MockAIProviders,
	);
	vi.spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o-mini"]);
	const router = createMemoryRouter(
		[
			{
				path: "/ai/settings/spend-details",
				element: <SpendDetailsPage now={new Date("2026-09-16T12:00:00Z")} />,
			},
		],
		{ initialEntries: [`/ai/settings/spend-details${options.search ?? ""}`] },
	);
	renderWithRouter(router);
	return { details, exportCSV, router };
};

describe("SpendDetailsPage", () => {
	it("requests the server budget window without explicit dates and exports the applied filter", async () => {
		const user = userEvent.setup();
		const { details, exportCSV } = renderPage();
		await screen.findByRole("table", { name: "AI spend details" });
		expect(details).toHaveBeenCalledWith(
			MockOrganization.id,
			expect.objectContaining({ limit: 25, offset: 0 }),
		);
		expect(details.mock.calls[0][1]).not.toHaveProperty("period_start");

		await user.click(screen.getByRole("button", { name: "Export CSV" }));
		await waitFor(() =>
			expect(exportCSV).toHaveBeenCalledWith(
				MockOrganization.id,
				expect.objectContaining({
					period_start: response.period_start,
					period_end: response.period_end,
				}),
			),
		);
	});

	it("uses the server default window for export when retention narrows it", async () => {
		const user = userEvent.setup();
		const { exportCSV } = renderPage({
			report: {
				...response,
				retention_start: response.period_start,
			},
		});
		await screen.findByRole("table", { name: "AI spend details" });
		await user.click(screen.getByRole("button", { name: "Export CSV" }));
		await waitFor(() =>
			expect(exportCSV).toHaveBeenCalledWith(MockOrganization.id, {}),
		);
	});

	it("surfaces an export API error", async () => {
		const user = userEvent.setup();
		const { exportCSV } = renderPage();
		exportCSV.mockRejectedValueOnce(new Error("Export failed"));
		await screen.findByRole("table", { name: "AI spend details" });
		await user.click(screen.getByRole("button", { name: "Export CSV" }));
		await waitFor(() => expect(exportCSV).toHaveBeenCalledTimes(1));
		await screen.findByText("Export failed");
	});

	it("applies the selected organization member and resets the request to the first page", async () => {
		const user = userEvent.setup();
		const { details } = renderPage({
			search: "?page=2&model=gpt-4o-mini",
		});
		await screen.findByRole("table", { name: "AI spend details" });
		await user.click(screen.getByRole("button", { name: "Select user" }));
		await user.click(
			await screen.findByRole("option", { name: MockOrganizationMember2.name }),
		);
		await waitFor(() =>
			expect(details).toHaveBeenLastCalledWith(
				MockOrganization.id,
				expect.objectContaining({
					user_id: MockOrganizationMember2.user_id,
					model: "gpt-4o-mini",
					offset: 0,
				}),
			),
		);
	});

	it("preserves filters when navigating between pages", async () => {
		const user = userEvent.setup();
		const { details, router } = renderPage({
			search: "?model=gpt-4o-mini",
			report: { ...response, count: 60 },
		});
		await screen.findByRole("table", { name: "AI spend details" });
		await user.click(screen.getByRole("button", { name: "Next page" }));
		await waitFor(() =>
			expect(details).toHaveBeenLastCalledWith(
				MockOrganization.id,
				expect.objectContaining({ model: "gpt-4o-mini", offset: 25 }),
			),
		);
		expect(new URLSearchParams(router.state.location.search).get("model")).toBe(
			"gpt-4o-mini",
		);
	});

	it("clears organization-scoped filters when selecting another organization", async () => {
		const user = userEvent.setup();
		const { details } = renderPage({
			organizations: [MockOrganization, MockOrganization2],
			search: `?user_id=${MockOrganizationMember.user_id}&group_id=${MockGroup.id}`,
		});
		await screen.findByRole("table", { name: "AI spend details" });
		await user.click(
			screen.getByRole("button", {
				name: `Organization ${MockOrganization.display_name}`,
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: /My Organization 2/,
			}),
		);
		await waitFor(() =>
			expect(details).toHaveBeenLastCalledWith(
				MockOrganization2.id,
				expect.objectContaining({
					user_id: undefined,
					group_id: undefined,
					offset: 0,
				}),
			),
		);
	});
});
