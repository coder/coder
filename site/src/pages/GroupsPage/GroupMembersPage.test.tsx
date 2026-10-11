import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import {
	MockEntitlements,
	MockGroupWithoutMembers,
	MockOrganization,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import GroupMembersPage from "./GroupMembersPage";
import GroupPage from "./GroupPage";

const renderGroupMembersPage = (agentHoursEnabled: boolean) => {
	server.use(
		http.get("/api/v2/entitlements", () =>
			HttpResponse.json({
				...MockEntitlements,
				has_license: true,
				features: withDefaultFeatures({
					agent_runtime_hours: {
						enabled: agentHoursEnabled,
						entitlement: agentHoursEnabled ? "entitled" : "not_entitled",
					},
				}),
			}),
		),
		http.get("/api/v2/organizations/:organization/groups/:groupName", () =>
			HttpResponse.json(MockGroupWithoutMembers),
		),
		http.get(
			"/api/v2/organizations/:organization/groups/:groupName/members",
			() => HttpResponse.json({ users: [MockUserMember], count: 1 }),
		),
	);
	const getAgentHours = vi
		.spyOn(API, "getGroupMembersAgentHours")
		.mockResolvedValue({
			usage_period: {
				issued_at: "2026-10-01T00:00:00Z",
				start: "2026-10-01T00:00:00Z",
				end: "2026-11-01T00:00:00Z",
			},
			members: [],
		});
	renderWithAuth(<GroupPage />, {
		route: `/organizations/${MockOrganization.name}/groups/${MockGroupWithoutMembers.name}`,
		path: "/organizations/:organization/groups/:groupName",
		children: [{ index: true, element: <GroupMembersPage /> }],
	});
	return getAgentHours;
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("GroupMembersPage Agent Hours", () => {
	it("loads members' Agent Hours when Agent Hours is enabled", async () => {
		const getAgentHours = renderGroupMembersPage(true);
		await screen.findByRole("table", { name: "Group members" });
		await vi.waitFor(() =>
			expect(getAgentHours).toHaveBeenCalledWith(MockGroupWithoutMembers.id, [
				MockUserMember.id,
			]),
		);
	});

	it("skips members' Agent Hours when Agent Hours is disabled", async () => {
		const getAgentHours = renderGroupMembersPage(false);
		await screen.findByText(MockUserMember.username);
		expect(getAgentHours).not.toHaveBeenCalled();
	});
});
