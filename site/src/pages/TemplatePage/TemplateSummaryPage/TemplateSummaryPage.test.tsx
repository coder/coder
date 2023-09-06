import { screen } from "@testing-library/react";
import { TemplateLayout } from "components/TemplateLayout/TemplateLayout";
import { rest } from "msw";
import { ResizeObserver } from "resize-observer";
import {
  MockTemplate,
  MockTemplateVersion,
  MockMemberPermissions,
} from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import * as CreateDayString from "utils/createDayString";
import { TemplateSummaryPage } from "./TemplateSummaryPage";

jest.mock("remark-gfm", () => jest.fn());

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserver,
});

const renderPage = () =>
  renderWithAuth(
    <TemplateLayout>
      <TemplateSummaryPage />
    </TemplateLayout>,
    {
      route: `/templates/${MockTemplate.id}`,
      path: "/templates/:template",
    },
  );

describe("TemplateSummaryPage", () => {
  it("shows the template name and resources", async () => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString");
    mock.mockImplementation(() => "a minute ago");

    renderPage();
    await screen.findByText(MockTemplate.display_name);
    screen.queryAllByText(`${MockTemplateVersion.name}`).length;
  });
  it("does not allow a member to delete a template", () => {
    // get member-level permissions
    server.use(
      rest.post("/api/v2/authcheck", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockMemberPermissions));
      }),
    );
    renderPage();
    const dropdownButton = screen.queryByLabelText("open-dropdown");
    expect(dropdownButton).toBe(null);
  });
});
