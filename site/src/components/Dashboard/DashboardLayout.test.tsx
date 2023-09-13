import { renderWithAuth } from "testHelpers/renderHelpers";
import { DashboardLayout } from "./DashboardLayout";
import * as API from "api/api";
import { screen } from "@testing-library/react";

test("Show the new Coder version notification", async () => {
  jest.spyOn(API, "getUpdateCheck").mockResolvedValue({
    current: false,
    version: "v0.12.9",
    url: "https://github.com/coder/coder/releases/tag/v0.12.9",
  });
  renderWithAuth(<DashboardLayout />, {
    children: [{ element: <h1>Test page</h1> }],
  });
  await screen.findByTestId("update-check-snackbar");
});
