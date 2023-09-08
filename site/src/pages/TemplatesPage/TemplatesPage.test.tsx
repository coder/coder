import { screen } from "@testing-library/react";
import { rest } from "msw";
import * as CreateDayString from "utils/createDayString";
import { MockTemplate } from "../../testHelpers/entities";
import { renderWithAuth } from "../../testHelpers/renderHelpers";
import { server } from "../../testHelpers/server";
import { TemplatesPage } from "./TemplatesPage";
import i18next from "i18next";

const { t } = i18next;

describe("TemplatesPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString");
    mock.mockImplementation(() => "a minute ago");
  });

  it("renders an empty templates page", async () => {
    // Given
    server.use(
      rest.get(
        "/api/v2/organizations/:organizationId/templates",
        (req, res, ctx) => {
          return res(ctx.status(200), ctx.json([]));
        },
      ),
      rest.post("/api/v2/authcheck", (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            createTemplates: true,
          }),
        );
      }),
    );

    // When
    renderWithAuth(<TemplatesPage />, {
      route: `/templates`,
      path: "/templates",
    });

    // Then
    const emptyMessage = t("empty.message", {
      ns: "templatesPage",
    });
    await screen.findByText(emptyMessage);
  });

  it("renders a filled templates page", async () => {
    // When
    renderWithAuth(<TemplatesPage />, {
      route: `/templates`,
      path: "/templates",
    });

    // Then
    await screen.findByText(MockTemplate.display_name);
  });

  it("shows empty view without permissions to create", async () => {
    server.use(
      rest.get(
        "/api/v2/organizations/:organizationId/templates",
        (req, res, ctx) => {
          return res(ctx.status(200), ctx.json([]));
        },
      ),
      rest.post("/api/v2/authcheck", (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            createTemplates: false,
          }),
        );
      }),
    );

    // When
    renderWithAuth(<TemplatesPage />, {
      route: `/templates`,
      path: "/templates",
    });
    // Then
    const emptyMessage = t("empty.descriptionWithoutPermissions", {
      ns: "templatesPage",
    });
    await screen.findByText(emptyMessage);
  });
});
