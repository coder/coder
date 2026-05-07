import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { SlimRole } from "#/api/typesGenerated";
import {
	MockEntitlementsWithMultiOrg,
	MockOrganization,
	MockOrganizationAuditorRole,
	MockOrganizationPermissions,
	MockUserMember,
} from "#/testHelpers/entities";
import {
	renderWithOrganizationSettingsLayout,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import OrganizationMembersPage from "./OrganizationMembersPage";

vi.spyOn(console, "error").mockImplementation(() => {});

beforeEach(() => {
	server.use(
		http.get("/api/v2/entitlements", () => {
			return HttpResponse.json(MockEntitlementsWithMultiOrg);
		}),
		http.post("/api/v2/authcheck", async () => {
			return HttpResponse.json(
				Object.fromEntries(
					Object.entries(MockOrganizationPermissions).map(([key, value]) => [
						`${MockOrganization.id}.${key}`,
						value,
					]),
				),
			);
		}),
	);
});

const renderPage = async () => {
	renderWithOrganizationSettingsLayout(<OrganizationMembersPage />, {
		route: `/organizations/${MockOrganization.name}/paginated-members`,
		path: "/organizations/:organization/paginated-members",
	});
	await waitForLoaderToBeRemoved();
};

const removeMember = async () => {
	const user = userEvent.setup();

	const users = await screen.findAllByText(/.*@coder.com/);
	const userRow = users[1].closest("tr");
	if (!userRow) {
		throw new Error("Error on get the first user row");
	}
	const menuButton = await within(userRow).findByRole("button", {
		name: "Open menu",
	});
	await user.click(menuButton);

	const removeOption = await screen.findByRole("menuitem", { name: "Remove…" });
	await user.click(removeOption);

	const dialog = await within(document.body).findByRole("dialog");
	await user.click(within(dialog).getByRole("button", { name: "Remove" }));
};

const updateUserRole = async (role: SlimRole) => {
	const user = userEvent.setup();

	// Get the second user in the table (the first user is "me" and has
	// no action menu).
	const users = await screen.findAllByText(/.*@coder.com/);
	const userRow = users[1].closest("tr");
	if (!userRow) {
		throw new Error("Error on get the first user row");
	}

	// Open the Edit roles dialog
	const editButton = within(userRow).getByLabelText("Open menu");
	await user.click(editButton);
	await user.click(await screen.findByText("Edit roles"));

	// Click on the role option
	const dialog = await screen.findByRole("dialog");
	const roleOption = within(dialog).getByText(role.display_name);
	await user.click(roleOption);
	await user.click(await screen.findByText("Confirm"));

	return {
		userRow,
	};
};

describe("OrganizationMembersPage", () => {
	describe("remove member", () => {
		describe("when it is success", () => {
			it("shows a success message", async () => {
				await renderPage();
				await removeMember();
				await screen.findByText(
					/"TestUser2" has been removed from "My Organization"\./,
				);
			});
		});
	});

	describe("Update user role", () => {
		describe("when it is success", () => {
			it("updates the roles", async () => {
				server.use(
					http.put(
						`/api/v2/organizations/:organizationId/members/${MockUserMember.id}/roles`,
						async () => {
							return HttpResponse.json({
								...MockUserMember,
								roles: [...MockUserMember.roles, MockOrganizationAuditorRole],
							});
						},
					),
				);

				await renderPage();
				await updateUserRole(MockOrganizationAuditorRole);
				await screen.findByText(/TestUser2's roles have been updated\./);
			});
		});

		describe("when it fails", () => {
			it("shows an error message", async () => {
				server.use(
					http.put(
						`/api/v2/organizations/:organizationId/members/${MockUserMember.id}/roles`,
						() => {
							return HttpResponse.json(
								{ message: "Error on updating the user roles." },
								{ status: 400 },
							);
						},
					),
				);

				await renderPage();
				await updateUserRole(MockOrganizationAuditorRole);
				await screen.findByText("Error on updating the user roles.");
			});
		});
	});
});
