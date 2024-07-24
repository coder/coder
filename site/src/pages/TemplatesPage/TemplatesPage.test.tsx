import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { server } from "testHelpers/server";
import TemplatesPage from "./TemplatesPage";

beforeAll(() => {
  server.use(
    http.get("/api/v2/experiments", () => {
      return HttpResponse.json(["multi-organization"]);
    }),
  );
});

test("navigate to starter templates page", async () => {
  const user = userEvent.setup();
  const router = createMemoryRouter(
    [
      {
        element: <RequireAuth />,
        children: [
          {
            path: "/templates",
            element: <TemplatesPage />,
          },
        ],
      },
    ],
    { initialEntries: ["/templates"] },
  );
  render(
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>,
  );
  const createTemplateButton = await screen.findByRole("button", {
    name: "Create Template",
  });
  await user.click(createTemplateButton);
  expect(router.state.location.pathname).toBe("/starter-templates");
});
