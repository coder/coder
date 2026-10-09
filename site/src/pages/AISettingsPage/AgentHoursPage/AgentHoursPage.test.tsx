import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import {
	MockAgentHoursGroupAllotment,
	MockAgentHoursOrganizationAllotment,
	MockEntitlements,
	MockGroup,
	MockNoPermissions,
	MockOrganization,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import AgentHoursPage from "./AgentHoursPage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: { ...MockNoPermissions, editDeploymentConfig: true },
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockOrganization],
		entitlements: {
			...MockEntitlements,
			features: withDefaultFeatures({
				agent_runtime_hours: {
					enabled: true,
					entitlement: "entitled",
					limit: 1000,
				},
			}),
		},
	}),
}));

afterEach(() => {
	vi.restoreAllMocks();
});

const renderPage = () => {
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
	});
	vi.spyOn(API, "getGroupsByOrganization").mockResolvedValue([MockGroup]);
	const getOrganizationAllotments = vi
		.spyOn(API, "getAgentHoursOrganizationAllotments")
		.mockResolvedValue([
			{ ...MockAgentHoursOrganizationAllotment, allotment_bps: 6000 },
		]);
	const getGroupAllotments = vi
		.spyOn(API, "getAgentHoursGroupAllotments")
		.mockResolvedValue({
			organization_allotment_bps: 6000,
			groups: [MockAgentHoursGroupAllotment],
		});
	const router = createMemoryRouter(
		[{ path: "/ai/settings/agent-hours", element: <AgentHoursPage /> }],
		{ initialEntries: ["/ai/settings/agent-hours"] },
	);
	renderWithRouter(router);
	return { getOrganizationAllotments, getGroupAllotments };
};

const saveAllotment = async (
	user: ReturnType<typeof userEvent.setup>,
	region: HTMLElement,
	name: string,
	percent: string,
) => {
	await user.click(
		await within(region).findByRole("button", {
			name: `Edit allotment for ${name}`,
		}),
	);
	const input = screen.getByRole("textbox", { name: "Allotment" });
	await user.clear(input);
	await user.type(input, percent);
	await user.click(screen.getByRole("button", { name: "Save" }));
};

it("saves and removes organization allotments", async () => {
	const user = userEvent.setup();
	const upsert = vi
		.spyOn(API, "upsertAgentHoursOrganizationAllotment")
		.mockResolvedValue(MockAgentHoursOrganizationAllotment);
	const remove = vi
		.spyOn(API, "deleteAgentHoursOrganizationAllotment")
		.mockResolvedValue();
	const { getGroupAllotments } = renderPage();
	const region = screen.getByRole("region", {
		name: "Organization allotments",
	});
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(1));

	// 80% fits only once the organization's own 60% is excluded.
	await saveAllotment(user, region, MockOrganization.display_name, "80");
	await waitFor(() =>
		expect(upsert).toHaveBeenCalledWith(MockOrganization.id, {
			allotment_bps: 8000,
		}),
	);
	// The group view carries the organization's share, so it must refetch.
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(2));

	await user.click(
		within(region).getByRole("button", {
			name: `Remove allotment for ${MockOrganization.display_name}`,
		}),
	);
	expect(remove).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Remove" }));
	await waitFor(() => expect(remove).toHaveBeenCalledWith(MockOrganization.id));
});

it("saves and removes group allotments", async () => {
	const user = userEvent.setup();
	const upsert = vi
		.spyOn(API, "upsertAgentHoursGroupAllotment")
		.mockResolvedValue(MockAgentHoursGroupAllotment);
	const remove = vi
		.spyOn(API, "deleteAgentHoursGroupAllotment")
		.mockResolvedValue();
	renderPage();
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});

	// 70% fits only once the group's own 50% is excluded.
	await saveAllotment(user, region, MockGroup.display_name, "70");
	await waitFor(() =>
		expect(upsert).toHaveBeenCalledWith(MockGroup.id, { allotment_bps: 7000 }),
	);

	await user.click(
		within(region).getByRole("button", {
			name: `Remove allotment for ${MockGroup.display_name}`,
		}),
	);
	await user.click(screen.getByRole("button", { name: "Remove" }));
	await waitFor(() => expect(remove).toHaveBeenCalledWith(MockGroup.id));
});

it("refreshes the allotments after a rejected save", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "upsertAgentHoursOrganizationAllotment").mockRejectedValue(
		mockApiError({
			message: "Agent Hours allotments cannot exceed 100% in total.",
		}),
	);
	const { getOrganizationAllotments } = renderPage();
	const region = screen.getByRole("region", {
		name: "Organization allotments",
	});
	await waitFor(() =>
		expect(getOrganizationAllotments).toHaveBeenCalledTimes(1),
	);

	// A conflict means another change landed, so the totals are stale.
	await saveAllotment(user, region, MockOrganization.display_name, "80");
	await waitFor(() =>
		expect(getOrganizationAllotments).toHaveBeenCalledTimes(2),
	);
});
