import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";

describe("RequireAuth", () => {
  it("redirects to /login if user is not authenticated", async () => {
    // appear logged out
    server.use(
      http.get("/api/v2/users/me", () => {
        return HttpResponse.json({ message: "no user here" }, { status: 401 });
      }),
    );

    renderWithAuth(<h1>Test</h1>, {
      nonAuthenticatedRoutes: [
        {
          path: "login",
          element: <h1>Login</h1>,
        },
      ],
    });

    await screen.findByText("Login");
  });
});
