import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { rest } from "msw";
import { createMemoryRouter } from "react-router-dom";
import { Language } from "./SignInForm";
import {
  render,
  renderWithRouter,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  beforeEach(() => {
    server.use(
      // Appear logged out
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }));
      }),
    );
  });

  it("shows an error message if SignIn fails", async () => {
    // Given
    const apiErrorMessage = "Something wrong happened";
    server.use(
      // Make login fail
      rest.post("/api/v2/users/login", async (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({ message: apiErrorMessage }));
      }),
    );

    // When
    render(<LoginPage />);
    await waitForLoaderToBeRemoved();
    const email = screen.getByLabelText(Language.emailLabel);
    const password = screen.getByLabelText(Language.passwordLabel);
    await userEvent.type(email, "test@coder.com");
    await userEvent.type(password, "password");
    // Click sign-in
    const signInButton = await screen.findByText(Language.passwordSignIn);
    fireEvent.click(signInButton);

    // Then
    const errorMessage = await screen.findByText(apiErrorMessage);
    expect(errorMessage).toBeDefined();
  });

  it("redirects to the setup page if there is no first user", async () => {
    // Given
    server.use(
      // No first user
      rest.get("/api/v2/users/first", (req, res, ctx) => {
        return res(ctx.status(404));
      }),
    );

    // When
    renderWithRouter(
      createMemoryRouter(
        [
          {
            path: "/login",
            element: <LoginPage />,
          },
          {
            path: "/setup",
            element: <h1>Setup</h1>,
          },
        ],
        { initialEntries: ["/login"] },
      ),
    );

    // Then
    await screen.findByText("Setup");
  });
});
