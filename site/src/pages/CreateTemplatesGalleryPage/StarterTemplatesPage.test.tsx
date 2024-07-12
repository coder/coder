import { render, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import {
  MockTemplateExample,
  MockTemplateExample2,
} from "testHelpers/entities";
import { server } from "testHelpers/server";
import StarterTemplatesPage from "./CreateTemplatesGalleryPage";

test("does not display the scratch template", async () => {
  server.use(
    http.get("api/v2/organizations/:organizationId/templates/examples", () => {
      return HttpResponse.json([
        MockTemplateExample,
        MockTemplateExample2,
        {
          ...MockTemplateExample,
          id: "scratch",
          name: "Scratch",
          description: "Create a template from scratch",
        },
      ]);
    }),
  );

  render(
    <AppProviders>
      <RouterProvider
        router={createMemoryRouter(
          [
            {
              element: <RequireAuth />,
              children: [
                {
                  path: "/create-templates",
                  element: <StarterTemplatesPage />,
                },
              ],
            },
          ],
          { initialEntries: ["/create-templatess"] },
        )}
      />
    </AppProviders>,
  );

  await screen.findByText(MockTemplateExample.name);
  screen.getByText(MockTemplateExample2.name);
  expect(screen.queryByText("Scratch")).not.toBeInTheDocument();
});
