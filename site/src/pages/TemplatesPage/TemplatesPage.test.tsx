import { render, screen } from "@testing-library/react";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import TemplatesPage from "./TemplatesPage";
import userEvent from "@testing-library/user-event";

test("create template from scratch", async () => {
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
          {
            path: "/templates/new",
            element: <div data-testid="new-template-page" />,
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
  const fromScratchMenuItem = await screen.findByText("From scratch");
  await user.click(fromScratchMenuItem);
  await screen.findByTestId("new-template-page");
  expect(router.state.location.pathname).toBe("/templates/new");
  expect(router.state.location.search).toBe("?exampleId=scratch");
});
