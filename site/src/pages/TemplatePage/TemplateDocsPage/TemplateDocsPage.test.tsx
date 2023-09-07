import { screen } from "@testing-library/react";
import { TemplateLayout } from "components/TemplateLayout/TemplateLayout";
import { ResizeObserver } from "resize-observer";
import { renderWithAuth } from "testHelpers/renderHelpers";
import TemplateDocsPage from "./TemplateDocsPage";

jest.mock("remark-gfm", () => jest.fn());

const TEMPLATE_NAME = "coder-ts";

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserver,
});

const renderPage = () =>
  renderWithAuth(
    <TemplateLayout>
      <TemplateDocsPage />
    </TemplateLayout>,
    {
      route: `/templates/${TEMPLATE_NAME}/docs`,
      path: "/templates/:template/docs",
    },
  );

describe("TemplateSummaryPage", () => {
  it("shows the template readme", async () => {
    renderPage();
    await screen.findByTestId("markdown");
  });
});
