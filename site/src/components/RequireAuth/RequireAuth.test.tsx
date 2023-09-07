import { screen } from "@testing-library/react";
import { rest } from "msw";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";

describe("RequireAuth", () => {
  it("redirects to /setup if there is no first user", async () => {
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }));
      }),
    );
    // No first user
    server.use(
      rest.get("/api/v2/users/first", async (req, res, ctx) => {
        return res(ctx.status(404));
      }),
    );

    renderWithAuth(<h1>Test</h1>, {
      nonAuthenticatedRoutes: [
        {
          path: "setup",
          element: <h1>Setup</h1>,
        },
      ],
    });

    await screen.findByText("Setup");
  });
});
