import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { rest } from "msw";
import {
  MockUser,
  MockUser2,
  SuspendedMockUser,
  MockAuditorRole,
} from "testHelpers/entities";
import * as API from "api/api";
import { Role } from "api/typesGenerated";
import { Language as ResetPasswordDialogLanguage } from "./ResetPasswordDialog";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { UsersPage } from "./UsersPage";

const renderPage = () => {
  return renderWithAuth(<UsersPage />);
};

const suspendUser = async () => {
  const user = userEvent.setup();
  // Get the first user in the table
  const moreButtons = await screen.findAllByLabelText("More options");
  const firstMoreButton = moreButtons[0];
  await user.click(firstMoreButton);

  const suspendButton = screen.getByTestId("suspend-button");
  await user.click(suspendButton);

  // Check if the confirm message is displayed
  const confirmDialog = await screen.findByRole("dialog");
  const confirmButton = await within(confirmDialog).findByRole("button", {
    name: "Suspend",
  });
  await user.click(confirmButton);
};

const deleteUser = async () => {
  const user = userEvent.setup();
  // Click on the "More options" button to display the "Delete" option
  // Needs to await fetching users and fetching permissions, because they're needed to see the more button
  const moreButtons = await screen.findAllByLabelText("More options");
  // get MockUser2
  const selectedMoreButton = moreButtons[1];

  await user.click(selectedMoreButton);

  const deleteButton = screen.getByText(/Delete/);
  await user.click(deleteButton);

  // Check if the confirm message is displayed
  const confirmDialog = await screen.findByRole("dialog");
  expect(confirmDialog).toHaveTextContent(`Are you sure you want to proceed?`);

  // Confirm with text input
  const textField = screen.getByLabelText("Name of the user to delete");
  const dialog = screen.getByRole("dialog");
  await user.type(textField, MockUser2.username);

  // Click on the "Confirm" button
  const confirmButton = within(dialog).getByRole("button", { name: "Delete" });
  await user.click(confirmButton);
};

const activateUser = async () => {
  const moreButtons = await screen.findAllByLabelText("More options");
  const suspendedMoreButton = moreButtons[2];
  fireEvent.click(suspendedMoreButton);

  const activateButton = screen.getByText(/Activate/);
  fireEvent.click(activateButton);

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog");

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByRole("button", {
    name: "Activate",
  });
  fireEvent.click(confirmButton);
};

const resetUserPassword = async (setupActionSpies: () => void) => {
  const moreButtons = await screen.findAllByLabelText("More options");
  const firstMoreButton = moreButtons[0];
  fireEvent.click(firstMoreButton);

  const resetPasswordButton = screen.getByText(/Reset password/);
  fireEvent.click(resetPasswordButton);

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog");
  expect(confirmDialog).toHaveTextContent(
    `You will need to send ${MockUser.username} the following password:`,
  );

  // Setup spies to check the actions after
  setupActionSpies();

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByRole("button", {
    name: ResetPasswordDialogLanguage.confirmText,
  });

  fireEvent.click(confirmButton);
};

const updateUserRole = async (role: Role) => {
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
  const auditorOption = within(fieldset).getByText(role.display_name);
  fireEvent.click(auditorOption);

  return {
    userRow,
  };
};

jest.spyOn(console, "error").mockImplementation(() => {});

describe("UsersPage", () => {
  describe("suspend user", () => {
    describe("when it is success", () => {
      it("shows a success message", async () => {
        renderPage();

        server.use(
          rest.put(
            `/api/v2/users/${MockUser.id}/status/suspend`,
            async (req, res, ctx) => {
              return res(ctx.status(200), ctx.json(SuspendedMockUser));
            },
          ),
        );

        await suspendUser();

        // Check if the success message is displayed
        await screen.findByText("Successfully suspended the user.");
      });
    });

    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        server.use(
          rest.put(
            `/api/v2/users/${MockUser.id}/status/suspend`,
            async (req, res, ctx) => {
              return res(
                ctx.status(400),
                ctx.json({
                  message: "Error suspending user.",
                }),
              );
            },
          ),
        );

        await suspendUser();

        // Check if the error message is displayed
        await screen.findByText("Error suspending user.");
      });
    });
  });

  describe("delete user", () => {
    describe("when it is success", () => {
      it("shows a success message", async () => {
        renderPage();

        server.use(
          rest.delete(
            `/api/v2/users/${MockUser2.id}`,
            async (req, res, ctx) => {
              return res(ctx.status(200), ctx.json(MockUser2));
            },
          ),
        );

        await deleteUser();

        // Check if the success message is displayed
        await screen.findByText("Successfully deleted the user.");
      });
    });
    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        server.use(
          rest.delete(
            `/api/v2/users/${MockUser2.id}`,
            async (req, res, ctx) => {
              return res(
                ctx.status(400),
                ctx.json({
                  message: "Error deleting user.",
                }),
              );
            },
          ),
        );

        await deleteUser();

        // Check if the error message is displayed
        await screen.findByText("Error deleting user.");
      });
    });
  });

  describe("activate user", () => {
    describe("when user is successfully activated", () => {
      it("shows a success message and refreshes the page", async () => {
        renderPage();

        server.use(
          rest.put(
            `/api/v2/users/${SuspendedMockUser.id}/status/activate`,
            async (req, res, ctx) => {
              return res(ctx.status(200), ctx.json(MockUser));
            },
          ),
        );

        await activateUser();

        // Check if the success message is displayed
        await screen.findByText("Successfully activated the user.");
      });
    });
    describe("when activation fails", () => {
      it("shows an error message", async () => {
        renderPage();

        server.use(
          rest.put(
            `/api/v2/users/${SuspendedMockUser.id}/status/activate`,
            async (req, res, ctx) => {
              return res(
                ctx.status(400),
                ctx.json({
                  message: "Error activating user.",
                }),
              );
            },
          ),
        );

        await activateUser();

        // Check if the error message is displayed
        await screen.findByText("Error activating user.");
      });
    });
  });

  describe("reset user password", () => {
    describe("when it is success", () => {
      it("shows a success message", async () => {
        renderPage();

        await resetUserPassword(() => {
          jest
            .spyOn(API, "updateUserPassword")
            .mockResolvedValueOnce(undefined);
        });

        // Check if the success message is displayed
        await screen.findByText("Successfully updated the user password.");

        // Check if the API was called correctly
        expect(API.updateUserPassword).toBeCalledTimes(1);
        expect(API.updateUserPassword).toBeCalledWith(MockUser.id, {
          password: expect.any(String),
          old_password: "",
        });
      });
    });
    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        await resetUserPassword(() => {
          jest.spyOn(API, "updateUserPassword").mockRejectedValueOnce({});
        });

        // Check if the error message is displayed
        await screen.findByText("Error on resetting the user password.");

        // Check if the API was called correctly
        expect(API.updateUserPassword).toBeCalledTimes(1);
        expect(API.updateUserPassword).toBeCalledWith(MockUser.id, {
          password: expect.any(String),
          old_password: "",
        });
      });
    });
  });

  describe("Update user role", () => {
    describe("when it is success", () => {
      it("updates the roles", async () => {
        renderPage();

        server.use(
          rest.put(
            `/api/v2/users/${MockUser.id}/roles`,
            async (req, res, ctx) => {
              return res(
                ctx.status(200),
                ctx.json({
                  ...MockUser,
                  roles: [...MockUser.roles, MockAuditorRole],
                }),
              );
            },
          ),
        );

        await updateUserRole(MockAuditorRole);

        await screen.findByText("Successfully updated the user roles.");
      });
    });

    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        server.use(
          rest.put(`/api/v2/users/${MockUser.id}/roles`, (req, res, ctx) => {
            return res(
              ctx.status(400),
              ctx.json({ message: "Error on updating the user roles." }),
            );
          }),
        );

        await updateUserRole(MockAuditorRole);

        // Check if the error message is displayed
        await screen.findByText("Error on updating the user roles.");
      });
    });
  });
});
