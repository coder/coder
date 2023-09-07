import { fireEvent, screen, waitFor } from "@testing-library/react";
import * as API from "api/api";
import * as AccountForm from "./AccountForm";
import { renderWithAuth } from "testHelpers/renderHelpers";
import * as AuthXService from "xServices/auth/authXService";
import { AccountPage } from "./AccountPage";
import i18next from "i18next";
import { mockApiError } from "testHelpers/entities";

const { t } = i18next;

const renderPage = () => {
  return renderWithAuth(<AccountPage />);
};

const newData = {
  username: "user",
};

const fillAndSubmitForm = async () => {
  await waitFor(() => screen.findByLabelText("Username"));
  fireEvent.change(screen.getByLabelText("Username"), {
    target: { value: newData.username },
  });
  fireEvent.click(screen.getByText(AccountForm.Language.updateSettings));
};

describe("AccountPage", () => {
  describe("when it is a success", () => {
    it("shows the success message", async () => {
      jest.spyOn(API, "updateProfile").mockImplementationOnce((userId, data) =>
        Promise.resolve({
          id: userId,
          email: "user@coder.com",
          created_at: new Date().toString(),
          status: "active",
          organization_ids: ["123"],
          roles: [],
          avatar_url: "",
          last_seen_at: new Date().toString(),
          login_type: "password",
          ...data,
        }),
      );
      const { user } = renderPage();
      await fillAndSubmitForm();

      const successMessage = await screen.findByText(
        AuthXService.Language.successProfileUpdate,
      );
      expect(successMessage).toBeDefined();
      expect(API.updateProfile).toBeCalledTimes(1);
      expect(API.updateProfile).toBeCalledWith(user.id, newData);
    });
  });

  describe("when the username is already taken", () => {
    it("shows an error", async () => {
      jest.spyOn(API, "updateProfile").mockRejectedValueOnce(
        mockApiError({
          message: "Invalid profile",
          validations: [
            { detail: "Username is already in use", field: "username" },
          ],
        }),
      );

      const { user } = renderPage();
      await fillAndSubmitForm();

      const errorMessage = await screen.findByText(
        "Username is already in use",
      );
      expect(errorMessage).toBeDefined();
      expect(API.updateProfile).toBeCalledTimes(1);
      expect(API.updateProfile).toBeCalledWith(user.id, newData);
    });
  });

  describe("when it is an unknown error", () => {
    it("shows a generic error message", async () => {
      jest.spyOn(API, "updateProfile").mockRejectedValueOnce({
        data: "unknown error",
      });

      const { user } = renderPage();
      await fillAndSubmitForm();

      const errorText = t("warningsAndErrors.somethingWentWrong", {
        ns: "common",
      });
      const errorMessage = await screen.findByText(errorText);
      expect(errorMessage).toBeDefined();
      expect(API.updateProfile).toBeCalledTimes(1);
      expect(API.updateProfile).toBeCalledWith(user.id, newData);
    });
  });
});
