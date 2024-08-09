import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { SlimRole } from "api/typesGenerated";
import {
  MockEntitlementsWithMultiOrg,
  MockUser,
  MockOrganization,
  MockOrganizationAuditorRole,
} from "testHelpers/entities";
import {
  renderWithManagementSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import OrganizationMembersPage from "./OrganizationMembersPage";

jest.spyOn(console, "error").mockImplementation(() => {});

beforeEach(() => {
  server.use(
    http.get("/api/v2/experiments", () => {
      return HttpResponse.json(["multi-organization"]);
    }),
    http.get("/api/v2/entitlements", () => {
      return HttpResponse.json(MockEntitlementsWithMultiOrg);
    }),
    http.post("/api/v2/authcheck", async () => {
      return HttpResponse.json({
        editMembers: true,
        viewDeploymentValues: true,
      });
    }),
  );
});

const renderPage = async () => {
  renderWithManagementSettingsLayout(<OrganizationMembersPage />, {
    route: `/organizations/${MockOrganization.name}/members`,
    path: `/organizations/:organization/members`,
  });
  await waitForLoaderToBeRemoved();
};

const removeMember = async () => {
  const user = userEvent.setup();
  // Click on the "More options" button to display the "Remove" option
  const moreButtons = await screen.findAllByLabelText("More options");
  // get MockUser2
  const selectedMoreButton = moreButtons[0];

  await user.click(selectedMoreButton);

  const removeButton = screen.getByText(/Remove/);
  await user.click(removeButton);
};

const updateUserRole = async (role: SlimRole) => {
  // Get the first user in the table
  const users = await screen.findAllByText(/.*@coder.com/);
  const userRow = users[0].closest("tr");
  if (!userRow) {
    throw new Error("Error on get the first user row");
  }

  // Click on the "edit icon" to display the role options
  const editButton = within(userRow).getByTitle("Edit user roles");
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
        await screen.findByText("Member removed successfully.");
      });
    });
  });

  describe("Update user role", () => {
    describe("when it is success", () => {
      it("updates the roles", async () => {
        server.use(
          http.put(
            `/api/v2/organizations/:organizationId/members/${MockUser.id}/roles`,
            async () => {
              return HttpResponse.json({
                ...MockUser,
                roles: [...MockUser.roles, MockOrganizationAuditorRole],
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
            `/api/v2/organizations/:organizationId/members/${MockUser.id}/roles`,
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
