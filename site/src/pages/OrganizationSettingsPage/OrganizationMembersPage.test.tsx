import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { SlimRole } from "api/typesGenerated";
import { http, HttpResponse } from "msw";
import {
	MockEntitlementsWithMultiOrg,
	MockOrganization,
	MockOrganizationAuditorRole,
	MockOrganizationPermissions,
	MockUserOwner,
} from "testHelpers/entities";
import {
	renderWithOrganizationSettingsLayout,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import OrganizationMembersPage from "./OrganizationMembersPage";

jest.spyOn(console, "error").mockImplementation(() => {});

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

	const removeOption = await screen.findByRole("menuitem", { name: "Remove" });
	await user.click(removeOption);

	const dialog = await within(document.body).findByRole("dialog");
	await user.click(within(dialog).getByRole("button", { name: "Remove" }));
};

const updateUserRole = async (role: SlimRole) => {
	// Get the first user in the table
	const users = await screen.findAllByText(/.*@coder.com/);
	const userRow = users[0].closest("tr");
	if (!userRow) {
		throw new Error("Error on get the first user row");
	}

	// Click on the "edit icon" to display the role options
	const editButton = within(userRow).getByLabelText("Edit user roles");
	fireEvent.click(editButton);

	// Click on the role option
	const fieldset = await screen.findByTitle("Available roles");
	const roleOption = within(fieldset).getByText(role.display_name);
	fireEvent.click(roleOption);

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
				await screen.findByText("User removed from organization successfully!");
			});
		});
	});

	describe("Update user role", () => {
		describe("when it is success", () => {
			it("updates the roles", async () => {
				server.use(
					http.put(
						`/api/v2/organizations/:organizationId/members/${MockUserOwner.id}/roles`,
						async () => {
							return HttpResponse.json({
								...MockUserOwner,
								roles: [...MockUserOwner.roles, MockOrganizationAuditorRole],
							});
						},
					),
				);

				await renderPage();
				await updateUserRole(MockOrganizationAuditorRole);
				await screen.findByText("Roles updated successfully.");
			});
		});

		describe("when it fails", () => {
			it("shows an error message", async () => {
				server.use(
					http.put(
						`/api/v2/organizations/:organizationId/members/${MockUserOwner.id}/roles`,
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
