import { render, screen } from "@testing-library/react";
import { rest } from "msw";
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
    rest.get(
      "/api/v2/organizations/:orgId/templates/:template/versions/:version/previous",
      (req, res, ctx) => {
        return res(ctx.status(404));
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
