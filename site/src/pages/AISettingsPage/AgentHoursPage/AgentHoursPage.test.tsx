import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { focusManager } from "react-query";
import { createMemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import { Toaster } from "#/components/Toaster/Toaster";
import {
	MockAgentHoursGroupAllotment,
	MockAgentHoursOrganizationAllotment,
	MockEntitlements,
	MockGroup,
	MockGroup2,
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentHoursPage from "./AgentHoursPage";

const access = vi.hoisted(() => ({ isOwner: true, isLicensed: true }));

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: {
			...MockNoPermissions,
			editDeploymentConfig: access.isOwner,
		},
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: {
			...MockEntitlements,
			features: withDefaultFeatures(
				access.isLicensed
					? {
							agent_runtime_hours: {
								enabled: true,
								entitlement: "entitled",
								limit: 1000,
							},
						}
					: {},
			),
		},
	}),
}));

afterEach(() => {
	vi.restoreAllMocks();
	focusManager.setFocused(undefined);
	access.isOwner = true;
	access.isLicensed = true;
});

const renderPage = ({
	canUpdateGroups = true,
	groups = [MockGroup],
	organizations = [MockOrganization],
} = {}) => {
	const getOrganizations = vi
		.spyOn(API, "getOrganizations")
		.mockResolvedValue(organizations);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: canUpdateGroups,
	});
	const getGroups = vi
		.spyOn(API, "getGroupsByOrganization")
		.mockResolvedValue(groups);
	const getOrganizationAllotments = vi
		.spyOn(API, "getAgentHoursOrganizationAllotments")
		.mockResolvedValue([
			{ ...MockAgentHoursOrganizationAllotment, allotment_bps: 6000 },
			{
				...MockAgentHoursOrganizationAllotment,
				organization_id: MockOrganization2.id,
				organization_name: MockOrganization2.name,
				organization_display_name: MockOrganization2.display_name,
				allotment_bps: 1000,
			},
		]);
	const getGroupAllotments = vi
		.spyOn(API, "getAgentHoursGroupAllotments")
		.mockResolvedValue({
			organization_allotment_bps: 6000,
			groups: [MockAgentHoursGroupAllotment],
		});
	const router = createMemoryRouter(
		[
			{
				path: "/ai/settings/agent-hours",
				element: (
					<>
						<AgentHoursPage />
						<Toaster />
					</>
				),
			},
		],
		{ initialEntries: ["/ai/settings/agent-hours"] },
	);
	renderWithRouter(router);
	return {
		getOrganizationAllotments,
		getGroupAllotments,
		getGroups,
		getOrganizations,
	};
};

const refocusWindow = () =>
	act(() => {
		focusManager.setFocused(false);
		focusManager.setFocused(true);
	});

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

it("refreshes every allotment view after a rejected save", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "upsertAgentHoursOrganizationAllotment").mockRejectedValue(
		mockApiError({
			message: "Agent Hours allotments cannot exceed 100% in total.",
		}),
	);
	const { getOrganizationAllotments, getGroupAllotments } = renderPage();
	const region = screen.getByRole("region", {
		name: "Organization allotments",
	});
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(1));

	// A conflict means another change landed, possibly to the organization
	// whose groups are shown, so every total is stale.
	await saveAllotment(user, region, MockOrganization2.display_name, "20");
	await waitFor(() =>
		expect(getOrganizationAllotments).toHaveBeenCalledTimes(2),
	);
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(2));
});

it("offers the edited group's current share after a rejected save", async () => {
	const user = userEvent.setup();
	const upsert = vi
		.spyOn(API, "upsertAgentHoursGroupAllotment")
		.mockRejectedValueOnce(
			mockApiError({
				message: "Agent Hours allotments cannot exceed 100% in total.",
			}),
		)
		.mockResolvedValue(MockAgentHoursGroupAllotment);
	const { getGroupAllotments } = renderPage();
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});
	await within(region).findByRole("button", {
		name: `Edit allotment for ${MockGroup.display_name}`,
	});
	// Another admin lowered this group to 10% and gave another group 60%.
	getGroupAllotments.mockResolvedValue({
		organization_allotment_bps: 6000,
		groups: [
			{ ...MockAgentHoursGroupAllotment, allotment_bps: 1000 },
			{
				...MockAgentHoursGroupAllotment,
				group_id: MockGroup2.id,
				group_name: MockGroup2.name,
				group_display_name: MockGroup2.display_name,
				allotment_bps: 6000,
			},
		],
	});

	await saveAllotment(user, region, MockGroup.display_name, "70");
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(2));

	// Only the current 10% plus the unallotted 30% is available.
	const input = screen.getByRole("textbox", { name: "Allotment" });
	for (const percent of ["45", "40"]) {
		await user.clear(input);
		await user.type(input, percent);
		await user.click(screen.getByRole("button", { name: "Save" }));
	}
	await waitFor(() =>
		expect(upsert.mock.calls).toEqual([
			[MockGroup.id, { allotment_bps: 7000 }],
			[MockGroup.id, { allotment_bps: 4000 }],
		]),
	);
});

it("reports a group deleted elsewhere by name on save", async () => {
	const user = userEvent.setup();
	server.use(
		http.put("/api/v2/groups/:groupId/agent-hours/allotment", () =>
			HttpResponse.json({ message: "Resource not found." }, { status: 404 }),
		),
	);
	const { getGroupAllotments, getGroups } = renderPage();
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});
	await user.click(
		await within(region).findByRole("button", {
			name: `Edit allotment for ${MockGroup.display_name}`,
		}),
	);

	// The refetch drops the deleted group before the save.
	getGroupAllotments.mockResolvedValue({
		organization_allotment_bps: 6000,
		groups: [],
	});
	getGroups.mockResolvedValue([]);
	refocusWindow();
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(2));

	const input = screen.getByRole("textbox", { name: "Allotment" });
	await user.clear(input);
	await user.type(input, "25");
	await user.click(screen.getByRole("button", { name: "Save" }));
	await screen.findByText(`${MockGroup.display_name} is no longer available.`);
});

it("does not add an allotment for a group deleted after selection", async () => {
	const user = userEvent.setup();
	const upsert = vi.spyOn(API, "upsertAgentHoursGroupAllotment");
	const { getGroups } = renderPage({ groups: [MockGroup, MockGroup2] });
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});
	const add = await within(region).findByRole("button", {
		name: "Add allotment",
	});
	await waitFor(() => expect(add).toBeEnabled());
	await user.click(add);
	await user.click(screen.getByRole("combobox", { name: "Group" }));
	await user.click(
		await screen.findByRole("option", { name: MockGroup2.name }),
	);
	await user.type(screen.getByRole("textbox", { name: "Allotment" }), "5");

	getGroups.mockResolvedValue([MockGroup]);
	refocusWindow();
	await waitFor(() => expect(getGroups).toHaveBeenCalledTimes(2));
	await user.click(screen.getByRole("button", { name: "Save" }));

	await screen.findByText("Select a group.");
	expect(upsert).not.toHaveBeenCalled();
});

it("does not add an allotment for an organization deleted after selection", async () => {
	const user = userEvent.setup();
	const upsert = vi.spyOn(API, "upsertAgentHoursOrganizationAllotment");
	const { getOrganizations } = renderPage({
		organizations: [MockOrganization, MockOrganization3],
	});
	const region = await screen.findByRole("region", {
		name: "Organization allotments",
	});
	const add = await within(region).findByRole("button", {
		name: "Add allotment",
	});
	await waitFor(() => expect(add).toBeEnabled());
	await user.click(add);
	await user.click(screen.getByRole("combobox", { name: "Organization" }));
	await user.click(
		await screen.findByRole("option", { name: MockOrganization3.display_name }),
	);
	await user.type(screen.getByRole("textbox", { name: "Allotment" }), "5");

	getOrganizations.mockResolvedValue([MockOrganization]);
	refocusWindow();
	await waitFor(() =>
		expect(
			screen.getByRole("combobox", { name: "Organization" }),
		).toHaveTextContent("Select an organization"),
	);
	await user.click(screen.getByRole("button", { name: "Save" }));

	await screen.findByText("Select an organization.");
	expect(upsert).not.toHaveBeenCalled();
});

it("reports a lost permission to change an allotment", async () => {
	const user = userEvent.setup();
	server.use(
		http.put("/api/v2/groups/:groupId/agent-hours/allotment", () =>
			HttpResponse.json({ message: "Forbidden." }, { status: 403 }),
		),
	);
	renderPage();
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});

	await saveAllotment(user, region, MockGroup.display_name, "70");
	await screen.findByText(
		"You no longer have access to change this allotment.",
	);
});

it("refreshes allotments and access when the window regains focus", async () => {
	const {
		getOrganizationAllotments,
		getGroupAllotments,
		getGroups,
		getOrganizations,
	} = renderPage();
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(getGroups).toHaveBeenCalledTimes(1));
	await waitFor(() => expect(getOrganizations).toHaveBeenCalledTimes(2));

	refocusWindow();
	await waitFor(() =>
		expect(getOrganizationAllotments).toHaveBeenCalledTimes(2),
	);
	await waitFor(() => expect(getGroupAllotments).toHaveBeenCalledTimes(2));
	await waitFor(() => expect(getGroups).toHaveBeenCalledTimes(2));
	await waitFor(() => expect(getOrganizations).toHaveBeenCalledTimes(4));
});

it("reports an allotment that is already gone as removed", async () => {
	const user = userEvent.setup();
	server.use(
		http.delete("/api/v2/groups/:groupId/agent-hours/allotment", () =>
			HttpResponse.json({ message: "Resource not found." }, { status: 404 }),
		),
	);
	renderPage();
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});

	await user.click(
		await within(region).findByRole("button", {
			name: `Remove allotment for ${MockGroup.display_name}`,
		}),
	);
	await user.click(screen.getByRole("button", { name: "Remove" }));
	await screen.findByText(
		`The allotment for ${MockGroup.display_name} was already removed.`,
	);
});

it("reports a lost permission to remove an allotment", async () => {
	const user = userEvent.setup();
	server.use(
		http.delete("/api/v2/groups/:groupId/agent-hours/allotment", () =>
			HttpResponse.json({ message: "Forbidden." }, { status: 403 }),
		),
	);
	renderPage();
	const region = await screen.findByRole("region", {
		name: "Group allotments",
	});

	await user.click(
		await within(region).findByRole("button", {
			name: `Remove allotment for ${MockGroup.display_name}`,
		}),
	);
	await user.click(screen.getByRole("button", { name: "Remove" }));
	await screen.findByText(
		"You no longer have access to remove this allotment.",
	);
});

it("shows group managers that the license lacks Agent Hours", async () => {
	access.isOwner = false;
	access.isLicensed = false;
	renderPage();

	await screen.findByText("Your license does not include Agent Hours");
});

it("denies users who cannot manage allotments", async () => {
	access.isOwner = false;
	access.isLicensed = false;
	renderPage({ canUpdateGroups: false });

	await screen.findByText("You don't have permission to view this page");
});
