import { screen } from "@testing-library/react";
import { rest } from "msw";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { DashboardLayout } from "./DashboardLayout";

test("Show the new Coder version notification", async () => {
  server.use(
    rest.get("/api/v2/updatecheck", (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json({
          current: false,
          version: "v0.12.9",
          url: "https://github.com/coder/coder/releases/tag/v0.12.9",
        }),
      );
    }),
  );
  renderWithAuth(<DashboardLayout />, {
    children: [{ element: <h1>Test page</h1> }],
  });
  await screen.findByTestId("update-check-snackbar");
});
