import React from "react"
import { screen } from "@testing-library/react"

import { render } from "../../test_helpers"
import { ProjectIcon } from "./ProjectIcon"

describe("ProjectIcon", () => {
  it("renders content", async () => {
    // When
    render(
      <ProjectIcon
        title="Test Title"
        onClick={() => {
          return
        }}
      />,
    )

    // Then
    await screen.findByText("Test Title", { exact: false })
  })
})
