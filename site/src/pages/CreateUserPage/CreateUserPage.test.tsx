import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Language as FormLanguage } from "./CreateUserForm";
import { Language as FooterLanguage } from "components/FormFooter/FormFooter";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { Language as CreateUserLanguage } from "xServices/users/createUserXService";
import { CreateUserPage } from "./CreateUserPage";

const renderCreateUserPage = async () => {
  renderWithAuth(<CreateUserPage />, {
    extraRoutes: [{ path: "/users", element: <div>Users Page</div> }],
  });
  await waitForLoaderToBeRemoved();
};

const fillForm = async ({
  username = "someuser",
  email = "someone@coder.com",
  password = "SomeSecurePassword!",
}: {
  username?: string;
  email?: string;
  password?: string;
}) => {
  const usernameField = screen.getByLabelText(FormLanguage.usernameLabel);
  const emailField = screen.getByLabelText(FormLanguage.emailLabel);
  const passwordField = screen
    .getByTestId("password-input")
    .querySelector("input");

  const loginTypeField = screen.getByTestId("login-type-input");
  await userEvent.type(usernameField, username);
  await userEvent.type(emailField, email);
  await userEvent.type(loginTypeField, "password");
  await userEvent.type(passwordField as HTMLElement, password);
  const submitButton = await screen.findByText(
    FooterLanguage.defaultSubmitLabel,
  );
  fireEvent.click(submitButton);
};

describe("Create User Page", () => {
  it("shows success notification and redirects to users page", async () => {
    await renderCreateUserPage();
    await fillForm({});
    const successMessage = await screen.findByText(
      CreateUserLanguage.createUserSuccess,
    );
    expect(successMessage).toBeDefined();
  });
});
