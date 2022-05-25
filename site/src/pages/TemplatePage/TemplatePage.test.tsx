import { screen } from "@testing-library/react"
import React from "react"
import { MockTemplate, MockWorkspaceResource, renderWithAuth } from "../../testHelpers/renderHelpers"
import { TemplatePage } from "./TemplatePage"

describe("TemplatePage", () => {
  it("shows the template name, readme and resources", async () => {
    renderWithAuth(<TemplatePage />, { route: `/templates/${MockTemplate.id}`, path: "/templates/:template" })
    await screen.findByText(MockTemplate.name)
    screen.getByTestId("markdown")
    screen.getByText(MockWorkspaceResource.name)
  })
})
