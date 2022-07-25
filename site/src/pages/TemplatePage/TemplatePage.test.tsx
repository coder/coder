import { screen } from "@testing-library/react"
import * as CreateDayString from "util/createDayString"
import {
  MockTemplate,
  MockTemplateVersion,
  MockWorkspaceResource,
  renderWithAuth,
} from "../../testHelpers/renderHelpers"
import { TemplatePage } from "./TemplatePage"

describe("TemplatePage", () => {
  it("shows the template name, readme and resources", async () => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")

    renderWithAuth(<TemplatePage />, {
      route: `/templates/${MockTemplate.id}`,
      path: "/templates/:template",
    })
    await screen.findByText(MockTemplate.name)
    screen.getByTestId("markdown")
    screen.getByText(MockWorkspaceResource.name)
    screen.getByTestId(`version-${MockTemplateVersion.id}`)
  })
})
