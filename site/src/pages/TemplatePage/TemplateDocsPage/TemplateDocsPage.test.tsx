import { screen } from "@testing-library/react"
import { TemplateLayout } from "components/TemplateLayout/TemplateLayout"
import { ResizeObserver } from "resize-observer"
import {
  MockTemplate,
  renderWithAuth,
} from "testHelpers/renderHelpers"
import TemplateDocsPage from "./TemplateDocsPage"

jest.mock("remark-gfm", () => jest.fn())

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserver,
})

const renderPage = () =>
  renderWithAuth(
    <TemplateLayout>
      <TemplateDocsPage />
    </TemplateLayout>,
    {
      route: `/templates/${MockTemplate.id}/docs`,
      path: "/templates/:template/docs",
    },
  )

describe("TemplateSummaryPage", () => {
  it("shows the template readme", async () => {
    renderPage()
    await screen.findByText(MockTemplate.display_name)
    await screen.findByTestId("markdown")
  })
})
