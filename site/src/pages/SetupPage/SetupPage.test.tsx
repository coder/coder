import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { rest } from "msw";
import { createMemoryRouter } from "react-router-dom";
import {
  renderWithRouter,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { SetupPage } from "./SetupPage";
import { Language as PageViewLanguage } from "./SetupPageView";
import { MockUser } from "testHelpers/entities";

const fillForm = async ({
  username = "someuser",
  email = "someone@coder.com",
  password = "password",
}: {
  username?: string;
  email?: string;
  password?: string;
} = {}) => {
  const usernameField = screen.getByLabelText(PageViewLanguage.usernameLabel);
  const emailField = screen.getByLabelText(PageViewLanguage.emailLabel);
  const passwordField = screen.getByLabelText(PageViewLanguage.passwordLabel);
  await userEvent.type(usernameField, username);
  await userEvent.type(emailField, email);
  await userEvent.type(passwordField, password);
  const submitButton = screen.getByRole("button", {
    name: PageViewLanguage.create,
  });
  fireEvent.click(submitButton);
};

describe("Setup Page", () => {
  beforeEach(() => {
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }));
      }),
      rest.get("/api/v2/users/first", (req, res, ctx) => {
        return res(
          ctx.status(404),
          ctx.json({ message: "no first user has been created" }),
        );
      }),
    );
  });

  it("redirects to the app when setup is successful", async () => {
    let userHasBeenCreated = false;

    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        if (!userHasBeenCreated) {
          return res(ctx.status(401), ctx.json({ message: "no user here" }));
        }
        return res(ctx.status(200), ctx.json(MockUser));
      }),
      rest.get("/api/v2/users/first", (req, res, ctx) => {
        if (!userHasBeenCreated) {
          return res(
            ctx.status(404),
            ctx.json({ message: "no first user has been created" }),
          );
        }
        return res(
          ctx.status(200),
          ctx.json({ message: "hooray, someone exists!" }),
        );
      }),
      rest.post("/api/v2/users/first", (req, res, ctx) => {
        userHasBeenCreated = true;
        return res(
          ctx.status(200),
          ctx.json({ data: "user setup was successful!" }),
        );
      }),
    );

    renderWithRouter(
      createMemoryRouter(
        [
          {
            path: "/setup",
            element: <SetupPage />,
          },
          {
            path: "/",
            element: <h1>Workspaces</h1>,
          },
        ],
        { initialEntries: ["/setup"] },
      ),
    );
    await waitForLoaderToBeRemoved();
    await fillForm();
    await waitFor(() => screen.findByText("Workspaces"));
  });
});
