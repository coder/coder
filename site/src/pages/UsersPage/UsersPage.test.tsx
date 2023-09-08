import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { rest } from "msw";
import {
  MockUser,
  MockUser2,
  SuspendedMockUser,
  MockAuditorRole,
  MockOwnerRole,
} from "testHelpers/entities";
import { Language as usersXServiceLanguage } from "xServices/users/usersXService";
import * as API from "../../api/api";
import { Role } from "../../api/typesGenerated";
import { Language as ResetPasswordDialogLanguage } from "./ResetPasswordDialog";
import { renderWithAuth } from "../../testHelpers/renderHelpers";
import { server } from "../../testHelpers/server";
import { Language as UsersPageLanguage, UsersPage } from "./UsersPage";

const renderPage = () => {
  return renderWithAuth(<UsersPage />);
};

const suspendUser = async (setupActionSpies: () => void) => {
  const user = userEvent.setup();
  // Get the first user in the table
  const moreButtons = await screen.findAllByLabelText("more");
  const firstMoreButton = moreButtons[0];

  await user.click(firstMoreButton);

  const menu = await screen.findByRole("menu");
  const suspendButton = within(menu).getByText("Suspend");

  await user.click(suspendButton);

  // Check if the confirm message is displayed
  const confirmDialog = await screen.findByRole("dialog");
  expect(confirmDialog).toHaveTextContent(
    `${UsersPageLanguage.suspendDialogMessagePrefix} ${MockUser.username}?`,
  );

  // Setup spies to check the actions after
  setupActionSpies();

  // Click on the "Confirm" button
  const confirmButton = await within(confirmDialog).findByText(
    UsersPageLanguage.suspendDialogAction,
  );
  await user.click(confirmButton);
};

const deleteUser = async (setupActionSpies: () => void) => {
  const user = userEvent.setup();
  // Click on the "more" button to display the "Delete" option
  // Needs to await fetching users and fetching permissions, because they're needed to see the more button
  const moreButtons = await screen.findAllByLabelText("more");
  // get MockUser2
  const selectedMoreButton = moreButtons[1];

  await user.click(selectedMoreButton);

  const menu = await screen.findByRole("menu");
  const deleteButton = within(menu).getByText("Delete");

  await user.click(deleteButton);

  // Check if the confirm message is displayed
  const confirmDialog = await screen.findByRole("dialog");
  expect(confirmDialog).toHaveTextContent(
    `Type ${MockUser2.username} below to confirm.`,
  );

  // Confirm with text input
  const textField = screen.getByLabelText("Name of the user to delete");
  const dialog = screen.getByRole("dialog");
  await user.type(textField, MockUser2.username);

  // Setup spies to check the actions after
  setupActionSpies();

  // Click on the "Confirm" button
  const confirmButton = within(dialog).getByRole("button", { name: "Delete" });
  await user.click(confirmButton);
};

const activateUser = async (setupActionSpies: () => void) => {
  const moreButtons = await screen.findAllByLabelText("more");
  const suspendedMoreButton = moreButtons[2];
  fireEvent.click(suspendedMoreButton);

  const menu = screen.getByRole("menu");
  const activateButton = within(menu).getByText("Activate");
  fireEvent.click(activateButton);

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog");
  expect(confirmDialog).toHaveTextContent(
    `${UsersPageLanguage.activateDialogMessagePrefix} ${SuspendedMockUser.username}?`,
  );

  // Setup spies to check the actions after
  setupActionSpies();

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByText(
    UsersPageLanguage.activateDialogAction,
  );
  fireEvent.click(confirmButton);
};

const resetUserPassword = async (setupActionSpies: () => void) => {
  const moreButtons = await screen.findAllByLabelText("more");
  const firstMoreButton = moreButtons[0];

  fireEvent.click(firstMoreButton);

  const menu = screen.getByRole("menu");
  const resetPasswordButton = within(menu).getByText("Reset password");

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

const updateUserRole = async (setupActionSpies: () => void, role: Role) => {
  // Get the first user in the table
  const users = await screen.findAllByText(/.*@coder.com/);
  const userRow = users[0].closest("tr");
  if (!userRow) {
    throw new Error("Error on get the first user row");
  }

  // Click on the "edit icon" to display the role options
  const editButton = within(userRow).getByTitle("Edit user roles");
  fireEvent.click(editButton);

  // Setup spies to check the actions after
  setupActionSpies();

  // Click on the role option
  const fieldset = await screen.findByTitle("Available roles");
  const auditorOption = within(fieldset).getByText(role.display_name);
  fireEvent.click(auditorOption);

  return {
    userRow,
  };
};

describe("UsersPage", () => {
  describe("suspend user", () => {
    describe("when it is success", () => {
      it("shows a success message and refresh the page", async () => {
        renderPage();

        await suspendUser(() => {
          jest.spyOn(API, "suspendUser").mockResolvedValueOnce(MockUser);
          jest.spyOn(API, "getUsers").mockResolvedValueOnce({
            users: [SuspendedMockUser, MockUser2],
            count: 2,
          });
        });

        // Check if the success message is displayed
        await screen.findByText(usersXServiceLanguage.suspendUserSuccess);

        // Check if the API was called correctly
        expect(API.suspendUser).toBeCalledTimes(1);
        expect(API.suspendUser).toBeCalledWith(MockUser.id);

        // Check if the users list was reload
        await waitFor(() => expect(API.getUsers).toBeCalledTimes(1));
      });
    });
    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        await suspendUser(() => {
          jest.spyOn(API, "suspendUser").mockRejectedValueOnce({});
        });

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.suspendUserError);

        // Check if the API was called correctly
        expect(API.suspendUser).toBeCalledTimes(1);
        expect(API.suspendUser).toBeCalledWith(MockUser.id);
      });
    });
  });

  describe("pagination", () => {
    it("goes to next and previous page", async () => {
      const { container } = renderPage();
      const user = userEvent.setup();

      const mock = jest
        .spyOn(API, "getUsers")
        .mockResolvedValueOnce({ users: [MockUser, MockUser2], count: 26 });

      const nextButton = await screen.findByLabelText("Next page");
      expect(nextButton).toBeEnabled();
      const previousButton = await screen.findByLabelText("Previous page");
      expect(previousButton).toBeDisabled();
      await user.click(nextButton);

      await waitFor(() =>
        expect(API.getUsers).toBeCalledWith({ offset: 25, limit: 25, q: "" }),
      );

      mock.mockClear();
      await user.click(previousButton);

      await waitFor(() =>
        expect(API.getUsers).toBeCalledWith({ offset: 0, limit: 25, q: "" }),
      );

      const pageButtons = container.querySelectorAll(
        `button[name="Page button"]`,
      );
      // count handler says there are 2 pages of results
      expect(pageButtons.length).toBe(2);
    });
  });

  describe("delete user", () => {
    describe("when it is success", () => {
      it("shows a success message and refresh the page", async () => {
        renderPage();

        await deleteUser(() => {
          jest.spyOn(API, "deleteUser").mockResolvedValueOnce(undefined);
          jest.spyOn(API, "getUsers").mockResolvedValueOnce({
            users: [MockUser, SuspendedMockUser],
            count: 2,
          });
        });

        // Check if the success message is displayed
        await screen.findByText(usersXServiceLanguage.deleteUserSuccess);

        // Check if the API was called correctly
        expect(API.deleteUser).toBeCalledTimes(1);
        expect(API.deleteUser).toBeCalledWith(MockUser2.id);

        // Check if the users list was reloaded
        await waitFor(() => {
          const users = screen.getAllByLabelText("more");
          expect(users.length).toEqual(2);
        });
      });
    });
    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        await deleteUser(() => {
          jest.spyOn(API, "deleteUser").mockRejectedValueOnce({});
        });

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.deleteUserError);

        // Check if the API was called correctly
        expect(API.deleteUser).toBeCalledTimes(1);
        expect(API.deleteUser).toBeCalledWith(MockUser2.id);
      });
    });
  });

  describe("activate user", () => {
    describe("when user is successfully activated", () => {
      it("shows a success message and refreshes the page", async () => {
        renderPage();

        await activateUser(() => {
          jest
            .spyOn(API, "activateUser")
            .mockResolvedValueOnce(SuspendedMockUser);
          jest.spyOn(API, "getUsers").mockImplementationOnce(() =>
            Promise.resolve({
              users: [MockUser, MockUser2, SuspendedMockUser],
              count: 3,
            }),
          );
        });

        // Check if the success message is displayed
        await screen.findByText(usersXServiceLanguage.activateUserSuccess);

        // Check if the API was called correctly
        expect(API.activateUser).toBeCalledTimes(1);
        expect(API.activateUser).toBeCalledWith(SuspendedMockUser.id);
      });
    });
    describe("when activation fails", () => {
      it("shows an error message", async () => {
        renderPage();

        await activateUser(() => {
          jest.spyOn(API, "activateUser").mockRejectedValueOnce({});
        });

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.activateUserError);

        // Check if the API was called correctly
        expect(API.activateUser).toBeCalledTimes(1);
        expect(API.activateUser).toBeCalledWith(SuspendedMockUser.id);
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
        await screen.findByText(usersXServiceLanguage.resetUserPasswordSuccess);

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
        await screen.findByText(usersXServiceLanguage.resetUserPasswordError);

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

        const { userRow } = await updateUserRole(() => {
          jest.spyOn(API, "updateUserRoles").mockResolvedValueOnce({
            ...MockUser,
            roles: [...MockUser.roles, MockAuditorRole],
          });
        }, MockAuditorRole);

        // Check if the select text was updated with the Auditor role
        await waitFor(() => {
          expect(userRow).toHaveTextContent(MockOwnerRole.display_name);
        });
        expect(userRow).toHaveTextContent(MockAuditorRole.display_name);

        // Check if the API was called correctly
        const currentRoles = MockUser.roles.map((r) => r.name);
        expect(API.updateUserRoles).toBeCalledTimes(1);
        expect(API.updateUserRoles).toBeCalledWith(
          [...currentRoles, MockAuditorRole.name],
          MockUser.id,
        );
      });
    });

    describe("when it fails", () => {
      it("shows an error message", async () => {
        renderPage();

        await updateUserRole(() => {
          jest.spyOn(API, "updateUserRoles").mockRejectedValueOnce({});
        }, MockAuditorRole);

        // Check if the error message is displayed
        const errorMessage = await screen.findByText(
          usersXServiceLanguage.updateUserRolesError,
        );
        await waitFor(() => expect(errorMessage).toBeDefined());

        // Check if the API was called correctly
        const currentRoles = MockUser.roles.map((r) => r.name);

        expect(API.updateUserRoles).toBeCalledTimes(1);
        expect(API.updateUserRoles).toBeCalledWith(
          [...currentRoles, MockAuditorRole.name],
          MockUser.id,
        );
      });
      it("shows an error from the backend", async () => {
        renderPage();

        server.use(
          rest.put(`/api/v2/users/${MockUser.id}/roles`, (req, res, ctx) => {
            return res(
              ctx.status(400),
              ctx.json({ message: "message from the backend" }),
            );
          }),
        );

        await updateUserRole(() => null, MockAuditorRole);

        // Check if the error message is displayed
        const errorMessage = await screen.findByText(
          "message from the backend",
        );
        expect(errorMessage).toBeDefined();
      });
    });
  });
});
