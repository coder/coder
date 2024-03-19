import { render, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { MockTemplate } from "testHelpers/entities";
import { server } from "testHelpers/server";
import { TemplateLayout } from "../TemplateLayout";
import TemplateFilesPage from "./TemplateFilesPage";

// Occasionally, Jest encounters HTML5 canvas errors. As the SyntaxHighlight is
// not required for these tests, we can safely mock it.
jest.mock("components/SyntaxHighlighter/SyntaxHighlighter", () => ({
  SyntaxHighlighter: () => <div data-testid="syntax-highlighter" />,
}));

test("displays the template files even when there is no previous version", async () => {
  server.use(
    http.get(
      "/api/v2/organizations/:organizationId/templates/:template/versions/:version/previous",
      () => {
        new HttpResponse(null, { status: 404 });
      },
    ),
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
                  element: <TemplateLayout />,
                  children: [
                    {
                      path: "/templates/:template/files",
                      element: <TemplateFilesPage />,
                    },
                  ],
                },
              ],
            },
          ],
          {
            initialEntries: [`/templates/${MockTemplate.name}/files`],
          },
        )}
      />
    </AppProviders>,
  );

  await screen.findByTestId("syntax-highlighter");
});
